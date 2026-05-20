// Cloudflare R2 implementation of the Storage interface (locked decision #61).
//
// Uses aws-sdk-go-v2 with R2's S3-compatible endpoint:
//
//	https://<account_id>.r2.cloudflarestorage.com
//
// Region is hardcoded to "auto" — R2 ignores it but the SDK requires a value.
// Path-style addressing is used because R2 does not support virtual-host
// buckets in all regions.
package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"
)

// R2Client implements Storage backed by Cloudflare R2.
type R2Client struct {
	s3       *s3.Client
	presign  *s3.PresignClient
	bucket   string
	defaultTTL time.Duration
}

// NewR2Client constructs an R2-backed Storage. cfg must satisfy
// IsConfigured() == true.
func NewR2Client(ctx context.Context, cfg R2Config) (*R2Client, error) {
	if !cfg.IsConfigured() {
		return nil, errors.New("storage: R2 config incomplete")
	}

	endpoint := fmt.Sprintf("https://%s.r2.cloudflarestorage.com", strings.TrimSpace(cfg.AccountID))

	awsCfg, err := awsconfig.LoadDefaultConfig(ctx,
		awsconfig.WithRegion("auto"),
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
			cfg.AccessKeyID, cfg.SecretAccessKey, "",
		)),
	)
	if err != nil {
		return nil, fmt.Errorf("storage: load aws config: %w", err)
	}

	cli := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(endpoint)
		o.UsePathStyle = true
	})

	ttlSec := cfg.PresignTTL
	if ttlSec <= 0 {
		ttlSec = 900
	}

	return &R2Client{
		s3:         cli,
		presign:    s3.NewPresignClient(cli),
		bucket:     cfg.Bucket,
		defaultTTL: time.Duration(ttlSec) * time.Second,
	}, nil
}

// Bucket returns the configured bucket name (used by readyz HeadBucket probe).
func (r *R2Client) Bucket() string { return r.bucket }

// HeadBucket pings the underlying bucket. Used by readyz probes.
func (r *R2Client) HeadBucket(ctx context.Context) error {
	_, err := r.s3.HeadBucket(ctx, &s3.HeadBucketInput{Bucket: aws.String(r.bucket)})
	return err
}

// PutObject implements Storage.
func (r *R2Client) PutObject(ctx context.Context, in PutObjectInput) error {
	if in.Key == "" {
		return errors.New("storage: empty key")
	}
	if in.Body == nil {
		return errors.New("storage: nil body")
	}
	input := &s3.PutObjectInput{
		Bucket: aws.String(r.bucket),
		Key:    aws.String(in.Key),
		Body:   in.Body,
	}
	if in.ContentType != "" {
		input.ContentType = aws.String(in.ContentType)
	}
	if in.Size > 0 {
		input.ContentLength = aws.Int64(in.Size)
	}
	if _, err := r.s3.PutObject(ctx, input); err != nil {
		return fmt.Errorf("storage: PutObject %s: %w", in.Key, err)
	}
	return nil
}

// GetObject implements Storage.
func (r *R2Client) GetObject(ctx context.Context, key string) (*Object, error) {
	out, err := r.s3.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(r.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		if isNotFound(err) {
			return nil, fmt.Errorf("%w: %s", ErrObjectNotFound, key)
		}
		return nil, fmt.Errorf("storage: GetObject %s: %w", key, err)
	}
	o := &Object{
		Key:  key,
		Body: out.Body,
	}
	if out.ContentLength != nil {
		o.Size = *out.ContentLength
	}
	if out.ContentType != nil {
		o.ContentType = *out.ContentType
	}
	return o, nil
}

// DeleteObject implements Storage. R2 returns 204 even when the object is
// missing, so this is naturally idempotent — we still swallow NotFound for
// belt-and-suspenders.
func (r *R2Client) DeleteObject(ctx context.Context, key string) error {
	_, err := r.s3.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(r.bucket),
		Key:    aws.String(key),
	})
	if err != nil && !isNotFound(err) {
		return fmt.Errorf("storage: DeleteObject %s: %w", key, err)
	}
	return nil
}

// ObjectExists implements Storage via HeadObject.
func (r *R2Client) ObjectExists(ctx context.Context, key string) (bool, error) {
	_, err := r.s3.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(r.bucket),
		Key:    aws.String(key),
	})
	if err == nil {
		return true, nil
	}
	if isNotFound(err) {
		return false, nil
	}
	return false, fmt.Errorf("storage: HeadObject %s: %w", key, err)
}

// PresignGet implements Storage. ttl is clamped to [60s, 24h].
func (r *R2Client) PresignGet(ctx context.Context, key string, ttl time.Duration) (string, error) {
	return r.presignGet(ctx, key, ttl, "")
}

// PresignGetDownload implements Storage with a forced attachment Content-
// Disposition. filename is RFC 5987-encoded into the disposition header so
// browsers display it correctly even when it contains UTF-8 / spaces.
func (r *R2Client) PresignGetDownload(ctx context.Context, key string, ttl time.Duration, filename string) (string, error) {
	disp := "attachment"
	if filename != "" {
		// path-safe printable ASCII subset for the unquoted form; UTF-8
		// goes into filename* via RFC 5987 percent-encoding.
		disp = fmt.Sprintf("attachment; filename=%q; filename*=UTF-8''%s",
			sanitizeASCII(filename), urlPathEscape(filename))
	}
	return r.presignGet(ctx, key, ttl, disp)
}

// presignGet is the shared implementation; contentDisposition is empty for
// PresignGet (no override) or a fully-formed header value for downloads.
func (r *R2Client) presignGet(ctx context.Context, key string, ttl time.Duration, contentDisposition string) (string, error) {
	if ttl <= 0 {
		ttl = r.defaultTTL
	}
	if ttl < 60*time.Second {
		ttl = 60 * time.Second
	}
	if ttl > 24*time.Hour {
		ttl = 24 * time.Hour
	}

	// Verify the object exists first so callers can distinguish 404 vs valid
	// presigned URL pointing at nothing.
	exists, err := r.ObjectExists(ctx, key)
	if err != nil {
		return "", err
	}
	if !exists {
		return "", fmt.Errorf("%w: %s", ErrObjectNotFound, key)
	}

	getInput := &s3.GetObjectInput{
		Bucket: aws.String(r.bucket),
		Key:    aws.String(key),
	}
	if contentDisposition != "" {
		getInput.ResponseContentDisposition = aws.String(contentDisposition)
	}
	req, err := r.presign.PresignGetObject(ctx, getInput,
		func(o *s3.PresignOptions) {
			o.Expires = ttl
		},
	)
	if err != nil {
		return "", fmt.Errorf("storage: presign %s: %w", key, err)
	}
	return req.URL, nil
}

// isNotFound reports whether err corresponds to an S3/R2 NoSuchKey or NotFound
// response. The SDK splits these between *types.NoSuchKey (GetObject) and
// *types.NotFound (HeadObject); we match on smithy-go API error codes too
// because R2 sometimes returns generic 404s without a typed error.
func isNotFound(err error) bool {
	if err == nil {
		return false
	}
	var nsk *types.NoSuchKey
	if errors.As(err, &nsk) {
		return true
	}
	var nf *types.NotFound
	if errors.As(err, &nf) {
		return true
	}
	var ae smithy.APIError
	if errors.As(err, &ae) {
		switch ae.ErrorCode() {
		case "NoSuchKey", "NotFound", "404":
			return true
		}
	}
	return false
}

// Compile-time check.
var _ Storage = (*R2Client)(nil)

// sanitizeASCII strips characters that are unsafe to put in a quoted
// filename header value. Replaces non-printable / control / quote chars
// with underscore. Output is intended for the legacy `filename="..."`
// segment of Content-Disposition, where most clients only consume ASCII.
func sanitizeASCII(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch {
		case r < 0x20 || r == 0x7f:
			b.WriteByte('_')
		case r == '"' || r == '\\':
			b.WriteByte('_')
		case r > 0x7e:
			b.WriteByte('_')
		default:
			b.WriteRune(r)
		}
	}
	out := b.String()
	if out == "" {
		return "download"
	}
	return out
}

// urlPathEscape percent-encodes a UTF-8 filename for use in the RFC 5987
// `filename*=UTF-8''…` segment of Content-Disposition. url.PathEscape is
// closer to the RFC 3986 spec than QueryEscape (which would encode space as +).
func urlPathEscape(s string) string {
	return url.PathEscape(s)
}

// Avoid unused import when SDK constants aren't referenced elsewhere.
var _ = io.EOF
