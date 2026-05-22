// Image upload + presigned download untuk BankSoal (Task 6.B.2).
//
// Per soal ada 6 inline slot gambar (mirror SoalBab Task 5.B.2, locked #78):
//   - pertanyaan (stem soal)
//   - opsi a..e (untuk soal "pilih gambar")
//
// Pipeline upload (locked decisions #46 + #62 + #78 + #84):
//   1. Auth + ownership guard via canManageSoal — guru hanya boleh
//      manage soal miliknya sendiri (per-guru pribadi, locked #84).
//   2. Mime sniff via http.DetectContentType — allowlist image/jpeg,
//      image/png, image/webp.
//   3. Size cap MaxBankSoalImageBytes = 5MB. Reject 413.
//   4. Decode + EXIF auto-orient + resize via disintegration/imaging — max
//      1920px sisi panjang. JPEG q85, PNG passthrough, WebP -> JPEG q85
//      (Go stdlib tidak punya WebP encoder).
//   5. Generate uuid -> object_key = "soal-bank/<uuid>.<ext>" (canonical
//      #58/#61, distinct dari "soal/" SoalBab supaya cleanup cron + audit
//      per-fitur tidak bercampur).
//   6. PutObject ke R2 dulu; kalau DB UPDATE gagal -> compensating delete
//      best-effort dgn context.Background() (locked pattern).
//   7. Atomic swap UpdateSoalImageSlot -> capture old key + write new.
//      Old key dihapus dari R2 best-effort post-commit.
//   8. Audit log banksoal_image_uploaded.
//
// Pipeline delete:
//   DELETE /bank-soal/:id/image?slot=<...> -> DB nullify slot column +
//   compensating R2 DeleteObject. Jika slot kosong -> 404
//   image_slot_empty.
//
// Pipeline presign:
//   GET /bank-soal/:id/image-url?slot=<...> -> presigned GET URL inline
//   disposition (locked #62) TTL 15m. Jika slot kosong -> 404.
//
// Authorization (locked #84):
//   Upload/Delete/Presign: hanya owner guru atau admin override. Guru
//   lain (siswa otomatis) -> 403 forbidden. Soal milik orang lain
//   tidak bisa diintip via image-url.
//
// Image swap is orthogonal to optimistic concurrency Version (#56) —
// repo.UpdateSoalImageSlot tidak bump version. Guru bisa swap gambar
// tanpa invalidate tab editor lain yang lagi edit teks.
package banksoal

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"net/http"
	"strings"
	"time"

	"github.com/disintegration/imaging"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/pikip/lms/backend/internal/storage"

	// Side-effect imports register WebP/GIF decoders for imaging.Decode.
	_ "golang.org/x/image/webp"
)

// ImageSlot enumerates the 6 inline image slots (mirror soalbab).
type ImageSlot string

const (
	// SlotPertanyaan adalah gambar untuk stem soal.
	SlotPertanyaan ImageSlot = "pertanyaan"
	SlotOpsiA      ImageSlot = "a"
	SlotOpsiB      ImageSlot = "b"
	SlotOpsiC      ImageSlot = "c"
	SlotOpsiD      ImageSlot = "d"
	SlotOpsiE      ImageSlot = "e"
)

// Valid reports whether s is a recognised image slot.
func (s ImageSlot) Valid() bool {
	switch s {
	case SlotPertanyaan, SlotOpsiA, SlotOpsiB, SlotOpsiC, SlotOpsiD, SlotOpsiE:
		return true
	}
	return false
}

// columnFor returns the underlying DB column name for the slot.
func (s ImageSlot) columnFor() string {
	switch s {
	case SlotPertanyaan:
		return "pertanyaan_object_key"
	case SlotOpsiA:
		return "opsi_a_object_key"
	case SlotOpsiB:
		return "opsi_b_object_key"
	case SlotOpsiC:
		return "opsi_c_object_key"
	case SlotOpsiD:
		return "opsi_d_object_key"
	case SlotOpsiE:
		return "opsi_e_object_key"
	}
	return ""
}

// MaxBankSoalImageBytes adalah batas raw upload sebelum resize (locked #78).
const MaxBankSoalImageBytes int64 = 5 * 1024 * 1024

// MaxBankSoalImageDimension adalah batas sisi panjang setelah resize (locked #78).
const MaxBankSoalImageDimension = 1920

// BankSoalImagePresignTTL adalah TTL presigned URL gambar inline (locked #62).
const BankSoalImagePresignTTL = 15 * time.Minute

// allowedBankSoalImageMimes daftar mime allowlist (locked #46/#78). Value
// adalah extension yang dipakai di object key + sumber routing encode
// (image/webp -> JPEG re-encode karena Go stdlib tidak encode WebP).
var allowedBankSoalImageMimes = map[string]string{
	"image/jpeg": "jpg",
	"image/png":  "png",
	"image/webp": "jpg",
}

// Sentinel errors mapped ke HTTP di handler.
var (
	ErrImageUnsupportedMime = errors.New("banksoal: image mime not allowed")
	ErrImageTooLarge        = errors.New("banksoal: image too large")
	ErrImageDecodeFailed    = errors.New("banksoal: image decode failed")
	ErrImageEncodeFailed    = errors.New("banksoal: image encode failed")
	ErrImageUploadFailed    = errors.New("banksoal: image upload failed")
	ErrImageSlotEmpty       = errors.New("banksoal: image slot is empty")
	ErrImageSlotInvalid     = errors.New("banksoal: invalid image slot")
	ErrR2Required           = errors.New("banksoal: object store not configured")
)

// UploadImageInput holds the multipart payload for image upload.
type UploadImageInput struct {
	Slot ImageSlot
	Body []byte
}

// ImageUploadResult is returned to the handler on success.
type ImageUploadResult struct {
	Soal      *BankSoal
	Slot      ImageSlot
	ObjectKey string
	MimeType  string
	SizeBytes int64
	Width     int
	Height    int
	Replaced  bool
}

// UploadImage validates + resizes + stores a soal image via R2 + updates
// the corresponding object_key column on the bank_soal row. Compensating
// R2 cleanup runs for the OLD key when a slot is replaced (locked #69).
func (s *Service) UploadImage(ctx context.Context, soalID, callerID uuid.UUID, callerRole string, in UploadImageInput, ip, userAgent string) (*ImageUploadResult, error) {
	if s.store == nil {
		return nil, ErrR2Required
	}
	if !in.Slot.Valid() {
		return nil, fmt.Errorf("%w: %q", ErrImageSlotInvalid, in.Slot)
	}
	if len(in.Body) == 0 {
		return nil, fmt.Errorf("%w: empty body", ErrInvalidInput)
	}
	if int64(len(in.Body)) > MaxBankSoalImageBytes {
		return nil, fmt.Errorf("%w: %d bytes exceeds %d", ErrImageTooLarge, len(in.Body), MaxBankSoalImageBytes)
	}

	// Mime sniff via stdlib (locked #46 + #78). Defense vs lying clients.
	probe := in.Body
	if len(probe) > 512 {
		probe = probe[:512]
	}
	sniffed := http.DetectContentType(probe)
	mimeStripped := strings.TrimSpace(strings.SplitN(sniffed, ";", 2)[0])
	ext, ok := allowedBankSoalImageMimes[mimeStripped]
	if !ok {
		return nil, fmt.Errorf("%w: detected %q", ErrImageUnsupportedMime, sniffed)
	}

	// Lookup soal + ownership guard (locked #84 per-guru pribadi).
	soal, err := s.repo.FindSoalByID(ctx, soalID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("banksoal image find: %w", err)
	}
	if !canManageSoal(soal, callerID, callerRole) {
		return nil, ErrForbidden
	}

	// Decode + resize + re-encode. JPEG q85, PNG lossless, WebP fallback
	// re-encode JPEG.
	resizedBody, finalMime, finalExt, w, h, err := resizeBankSoalImage(in.Body, mimeStripped, ext)
	if err != nil {
		return nil, err
	}

	// Build new object key: soal-bank/<uuid>.<ext> (locked #58/#61).
	objectID := uuid.New()
	objectKey, err := storage.BuildKey(storage.CategoryBankSoal, objectID.String()+"."+finalExt)
	if err != nil {
		return nil, fmt.Errorf("banksoal image build key: %w", err)
	}

	// PutObject sebelum DB UPDATE — kalau DB gagal, compensating delete
	// pakai context.Background() supaya cancel request tidak sabotase
	// cleanup.
	if perr := s.store.PutObject(ctx, storage.PutObjectInput{
		Key:         objectKey,
		Body:        bytes.NewReader(resizedBody),
		Size:        int64(len(resizedBody)),
		ContentType: finalMime,
	}); perr != nil {
		return nil, fmt.Errorf("%w: %v", ErrImageUploadFailed, perr)
	}

	// Atomic swap: capture old key, write new, return old for cleanup.
	oldKey, err := s.repo.UpdateSoalImageSlot(ctx, soalID, in.Slot.columnFor(), &objectKey)
	if err != nil {
		// DB gagal — drop new R2 object dgn fresh ctx.
		delCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if derr := s.store.DeleteObject(delCtx, objectKey); derr != nil {
			s.logAudit(ctx, "banksoal_image_orphan", &callerID, callerRole, &soalID, nil, ip, userAgent, map[string]any{
				"object_key": objectKey,
				"slot":       string(in.Slot),
				"reason":     "compensating_delete_failed",
				"err":        derr.Error(),
			})
		}
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("banksoal image db: %w", err)
	}

	// Compensating R2 cleanup pada slot replace — best effort, audit on fail.
	replaced := oldKey != nil && *oldKey != "" && *oldKey != objectKey
	if replaced {
		delCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if derr := s.store.DeleteObject(delCtx, *oldKey); derr != nil {
			s.logAudit(ctx, "banksoal_image_orphan", &callerID, callerRole, &soalID, nil, ip, userAgent, map[string]any{
				"object_key": *oldKey,
				"slot":       string(in.Slot),
				"reason":     "replace_old_key_delete_failed",
				"err":        derr.Error(),
			})
		}
	}

	// Refetch soal supaya respons reflect kolom terbaru (version tidak
	// bump untuk image swap — orthogonal to text edits).
	fresh, err := s.repo.FindSoalByID(ctx, soalID)
	if err != nil {
		return nil, fmt.Errorf("banksoal image refetch: %w", err)
	}

	s.logAudit(ctx, "banksoal_image_uploaded", &callerID, callerRole, &soalID, nil, ip, userAgent, map[string]any{
		"soal_id":    soalID.String(),
		"slot":       string(in.Slot),
		"object_key": objectKey,
		"mime_type":  finalMime,
		"size_bytes": int64(len(resizedBody)),
		"width":      w,
		"height":     h,
		"replaced":   replaced,
	})

	return &ImageUploadResult{
		Soal:      fresh,
		Slot:      in.Slot,
		ObjectKey: objectKey,
		MimeType:  finalMime,
		SizeBytes: int64(len(resizedBody)),
		Width:     w,
		Height:    h,
		Replaced:  replaced,
	}, nil
}

// DeleteImage clears one image slot + compensating R2 DeleteObject.
// Returns ErrImageSlotEmpty (404) kalau slot belum berisi.
func (s *Service) DeleteImage(ctx context.Context, soalID, callerID uuid.UUID, callerRole string, slot ImageSlot, ip, userAgent string) (*BankSoal, error) {
	if s.store == nil {
		return nil, ErrR2Required
	}
	if !slot.Valid() {
		return nil, fmt.Errorf("%w: %q", ErrImageSlotInvalid, slot)
	}

	repoIF := s.repo
	soal, err := repoIF.FindSoalByID(ctx, soalID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("banksoal image delete find: %w", err)
	}
	if !canManageSoal(soal, callerID, callerRole) {
		return nil, ErrForbidden
	}

	// Pre-check: slot must currently have a key.
	currentKey := imageKeyForSlot(soal, slot)
	if currentKey == nil || *currentKey == "" {
		return nil, ErrImageSlotEmpty
	}

	// Validate post-delete row would still be valid (jawaban + opsi text).
	preview := *soal
	clearImageSlot(&preview, slot)
	if err := s.validateSoalFields(&preview); err != nil {
		return nil, err
	}

	oldKey, err := repoIF.UpdateSoalImageSlot(ctx, soalID, slot.columnFor(), nil)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("banksoal image delete db: %w", err)
	}

	if oldKey != nil && *oldKey != "" {
		delCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if derr := s.store.DeleteObject(delCtx, *oldKey); derr != nil {
			s.logAudit(ctx, "banksoal_image_orphan", &callerID, callerRole, &soalID, nil, ip, userAgent, map[string]any{
				"object_key": *oldKey,
				"slot":       string(slot),
				"reason":     "delete_object_failed_after_db",
				"err":        derr.Error(),
			})
		}
	}

	fresh, err := repoIF.FindSoalByID(ctx, soalID)
	if err != nil {
		return nil, fmt.Errorf("banksoal image delete refetch: %w", err)
	}

	clearedKey := ""
	if oldKey != nil {
		clearedKey = *oldKey
	}
	s.logAudit(ctx, "banksoal_image_deleted", &callerID, callerRole, &soalID, nil, ip, userAgent, map[string]any{
		"soal_id":    soalID.String(),
		"slot":       string(slot),
		"object_key": clearedKey,
	})
	return fresh, nil
}

// BankSoalImageURLResult is returned by PresignImageURL.
type BankSoalImageURLResult struct {
	URL       string
	ExpiresAt time.Time
	Slot      ImageSlot
	ObjectKey string
}

// PresignImageURL issues a short-lived inline GET URL untuk gambar slot.
// Owner guru atau admin only — guru lain BLOCKED (locked #84).
func (s *Service) PresignImageURL(ctx context.Context, soalID, callerID uuid.UUID, callerRole string, slot ImageSlot, ip, userAgent string) (*BankSoalImageURLResult, error) {
	if s.store == nil {
		return nil, ErrR2Required
	}
	if !slot.Valid() {
		return nil, fmt.Errorf("%w: %q", ErrImageSlotInvalid, slot)
	}

	soal, err := s.repo.FindSoalByID(ctx, soalID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("banksoal image presign find: %w", err)
	}
	if !canManageSoal(soal, callerID, callerRole) {
		return nil, ErrForbidden
	}

	currentKey := imageKeyForSlot(soal, slot)
	if currentKey == nil || *currentKey == "" {
		return nil, ErrImageSlotEmpty
	}

	url, perr := s.store.PresignGet(ctx, *currentKey, BankSoalImagePresignTTL)
	if perr != nil {
		if errors.Is(perr, storage.ErrObjectNotFound) {
			return nil, ErrImageSlotEmpty
		}
		return nil, fmt.Errorf("banksoal image presign: %w", perr)
	}

	s.logAudit(ctx, "banksoal_image_url_issued", &callerID, callerRole, &soalID, nil, ip, userAgent, map[string]any{
		"soal_id":    soalID.String(),
		"slot":       string(slot),
		"object_key": *currentKey,
		"ttl":        int(BankSoalImagePresignTTL.Seconds()),
	})
	return &BankSoalImageURLResult{
		URL:       url,
		ExpiresAt: s.now().Add(BankSoalImagePresignTTL),
		Slot:      slot,
		ObjectKey: *currentKey,
	}, nil
}

// ---------- helpers ----------

// imageKeyForSlot returns the pointer to the slot's object_key column on
// the in-memory bank_soal row.
func imageKeyForSlot(s *BankSoal, slot ImageSlot) *string {
	switch slot {
	case SlotPertanyaan:
		return s.PertanyaanObjectKey
	case SlotOpsiA:
		return s.OpsiAObjectKey
	case SlotOpsiB:
		return s.OpsiBObjectKey
	case SlotOpsiC:
		return s.OpsiCObjectKey
	case SlotOpsiD:
		return s.OpsiDObjectKey
	case SlotOpsiE:
		return s.OpsiEObjectKey
	}
	return nil
}

// clearImageSlot nilai slot pointer di in-memory copy ke nil supaya
// validate flow ngecek post-delete state benar.
func clearImageSlot(s *BankSoal, slot ImageSlot) {
	switch slot {
	case SlotPertanyaan:
		s.PertanyaanObjectKey = nil
	case SlotOpsiA:
		s.OpsiAObjectKey = nil
	case SlotOpsiB:
		s.OpsiBObjectKey = nil
	case SlotOpsiC:
		s.OpsiCObjectKey = nil
	case SlotOpsiD:
		s.OpsiDObjectKey = nil
	case SlotOpsiE:
		s.OpsiEObjectKey = nil
	}
}

// resizeBankSoalImage decodes raw bytes, applies EXIF auto-orient,
// resizes if longest side > 1920px, dan re-encodes (JPEG q85 untuk
// JPEG/WebP, PNG lossless untuk PNG). Returns (encoded body, final mime,
// final ext, width, height, err).
func resizeBankSoalImage(raw []byte, srcMime, hintExt string) ([]byte, string, string, int, int, error) {
	img, err := imaging.Decode(bytes.NewReader(raw), imaging.AutoOrientation(true))
	if err != nil {
		return nil, "", "", 0, 0, fmt.Errorf("%w: %v", ErrImageDecodeFailed, err)
	}
	bounds := img.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	longest := w
	if h > longest {
		longest = h
	}
	if longest > MaxBankSoalImageDimension {
		img = imaging.Fit(img, MaxBankSoalImageDimension, MaxBankSoalImageDimension, imaging.Lanczos)
		bounds = img.Bounds()
		w, h = bounds.Dx(), bounds.Dy()
	}

	var buf bytes.Buffer
	switch {
	case srcMime == "image/png":
		if err := png.Encode(&buf, img); err != nil {
			return nil, "", "", 0, 0, fmt.Errorf("%w: %v", ErrImageEncodeFailed, err)
		}
		return buf.Bytes(), "image/png", "png", w, h, nil
	default:
		// JPEG (juga WebP fallback).
		if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 85}); err != nil {
			return nil, "", "", 0, 0, fmt.Errorf("%w: %v", ErrImageEncodeFailed, err)
		}
		return buf.Bytes(), "image/jpeg", "jpg", w, h, nil
	}
}

// _ avoids "imported and not used" if image is referenced only via subpackages.
var _ = image.Black
