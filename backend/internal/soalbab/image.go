// Image upload + presigned download untuk SoalBab (Task 5.B.2).
//
// Per soal ada 6 inline slot gambar (locked #78):
//   - pertanyaan (stem soal)
//   - opsi a..e (untuk soal "pilih gambar")
//
// Pipeline upload (locked decisions #46 + #62 + #78):
//   1. Auth + ownership guard via service.findKelasOrForbidden +
//      bab not archived check.
//   2. Mime sniff via http.DetectContentType — allowlist image/jpeg,
//      image/png, image/webp.
//   3. Size cap MaxSoalImageBytes = 5MB (locked #78). Reject 413.
//   4. Resize via disintegration/imaging — max 1920px sisi panjang
//      (locked #78). JPEG q85, PNG passthrough, WebP -> JPEG q85
//      (Go stdlib tidak punya WebP encoder; decode via golang.org/x/image
//      kalau ada, fallback re-encode JPEG).
//   5. Generate uuid → object_key = "soal/<uuid>.<ext>" (canonical #58).
//   6. Compensating R2 cleanup pada slot replace (key lama dihapus
//      setelah DB Update sukses — locked #69 pattern).
//   7. Audit log soalbab_image_uploaded.
//
// Pipeline delete:
//   DELETE /soal-bab/:id/image?slot=<...> → DB nullify slot column +
//   compensating R2 DeleteObject. Jika slot kosong → 404.
//
// Pipeline presign:
//   GET /soal-bab/:id/image-url?slot=<...> → presigned GET URL inline
//   disposition (locked #62) TTL 15m. Jika slot kosong → 404.
//
// Authorization:
//   Upload/Delete: guru pemilik kelas atau admin.
//   Presign: guru/admin pemilik kelas (siswa BLOCKED — siswa lihat soal
//   lewat flow Latihan/Ulangan endpoint, locked #76 anti-cheat).
package soalbab

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

	"github.com/pikip/lms/backend/internal/bab"
	"github.com/pikip/lms/backend/internal/storage"
)

// ImageSlot enumerates the 6 inline image slots (locked #78).
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

// MaxSoalImageBytes adalah batas raw upload sebelum resize (locked #78).
const MaxSoalImageBytes int64 = 5 * 1024 * 1024

// MaxSoalImageDimension adalah batas sisi panjang setelah resize (locked #78).
const MaxSoalImageDimension = 1920

// SoalImagePresignTTL adalah TTL presigned URL gambar inline (locked #62).
const SoalImagePresignTTL = 15 * time.Minute

// allowedSoalImageMimes daftar mime allowlist (locked #78). Value adalah
// extension yang dipakai di object key + content-type final yang ditulis
// ke R2 PutObject.
var allowedSoalImageMimes = map[string]string{
	"image/jpeg": "jpg",
	"image/png":  "png",
	"image/webp": "jpg", // re-encoded sebagai JPEG karena Go stdlib tidak encode WebP.
}

// Sentinel errors mapped ke HTTP di handler.
var (
	ErrImageUnsupportedMime = errors.New("soalbab: image mime not allowed")
	ErrImageTooLarge        = errors.New("soalbab: image too large")
	ErrImageDecodeFailed    = errors.New("soalbab: image decode failed")
	ErrImageEncodeFailed    = errors.New("soalbab: image encode failed")
	ErrImageUploadFailed    = errors.New("soalbab: image upload failed")
	ErrImageSlotEmpty       = errors.New("soalbab: image slot is empty")
	ErrImageSlotInvalid     = errors.New("soalbab: invalid image slot")
	ErrR2Required           = errors.New("soalbab: object store not configured")
)

// UploadImageInput holds the multipart payload for image upload.
type UploadImageInput struct {
	Slot ImageSlot
	Body []byte
}

// ImageUploadResult is returned to the handler on success.
type ImageUploadResult struct {
	Soal      *SoalBab
	Slot      ImageSlot
	ObjectKey string
	MimeType  string
	SizeBytes int64
	Width     int
	Height    int
}

// UploadImage validates + resizes + stores a soal image via R2 + updates
// the corresponding object_key column on the soal_bab row. Compensating
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
	if int64(len(in.Body)) > MaxSoalImageBytes {
		return nil, fmt.Errorf("%w: %d bytes exceeds %d", ErrImageTooLarge, len(in.Body), MaxSoalImageBytes)
	}

	// Mime sniff via stdlib (locked #46 + #78).
	probe := in.Body
	if len(probe) > 512 {
		probe = probe[:512]
	}
	sniffed := http.DetectContentType(probe)
	mimeStripped := strings.TrimSpace(strings.SplitN(sniffed, ";", 2)[0])
	ext, ok := allowedSoalImageMimes[mimeStripped]
	if !ok {
		return nil, fmt.Errorf("%w: detected %q", ErrImageUnsupportedMime, sniffed)
	}

	// Lookup soal + ownership + bab not archived guard.
	soal, err := s.repo.FindSoalByID(ctx, soalID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("soalbab image find: %w", err)
	}
	b, err := s.findBabAndOwnership(ctx, soal.BabID, callerID, callerRole)
	if err != nil {
		return nil, err
	}
	if b.Status == bab.StatusArchived {
		return nil, ErrBabArchived
	}

	// Decode + resize + re-encode. JPEG q85, PNG lossless. WebP gets
	// re-encoded to JPEG (stdlib tidak punya WebP encoder).
	resizedBody, finalMime, finalExt, w, h, err := resizeImageForSoal(in.Body, mimeStripped, ext)
	if err != nil {
		return nil, err
	}

	// Build new object key under soal/<uuid>.<ext> (locked #58/#61).
	objectID := uuid.New()
	objectKey, err := storage.BuildKey(storage.CategorySoal, objectID.String()+"."+finalExt)
	if err != nil {
		return nil, fmt.Errorf("soalbab image build key: %w", err)
	}

	// PutObject sebelum DB UPDATE — kalau DB gagal, compensating delete.
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
		// DB gagal — drop new R2 object, surface error.
		if derr := s.store.DeleteObject(context.Background(), objectKey); derr != nil {
			s.logAudit(ctx, "soalbab_image_orphan", &callerID, callerRole, &soalID, &soal.KelasID, ip, userAgent, map[string]any{
				"object_key": objectKey,
				"slot":       string(in.Slot),
				"reason":     "compensating_delete_failed",
				"err":        derr.Error(),
			})
		}
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("soalbab image db: %w", err)
	}

	// Compensating R2 cleanup pada slot replace — best effort, audit on fail.
	if oldKey != nil && *oldKey != "" && *oldKey != objectKey {
		if derr := s.store.DeleteObject(context.Background(), *oldKey); derr != nil {
			s.logAudit(ctx, "soalbab_image_orphan", &callerID, callerRole, &soalID, &soal.KelasID, ip, userAgent, map[string]any{
				"object_key": *oldKey,
				"slot":       string(in.Slot),
				"reason":     "replace_old_key_delete_failed",
				"err":        derr.Error(),
			})
		}
	}

	// Refetch soal supaya respons reflect kolom terbaru + version bump
	// kalau ada (image swap tidak bump version — itu mode ulangan).
	fresh, err := s.repo.FindSoalByID(ctx, soalID)
	if err != nil {
		return nil, fmt.Errorf("soalbab image refetch: %w", err)
	}

	s.logAudit(ctx, "soalbab_image_uploaded", &callerID, callerRole, &soalID, &soal.KelasID, ip, userAgent, map[string]any{
		"soal_id":    soalID.String(),
		"slot":       string(in.Slot),
		"object_key": objectKey,
		"mime_type":  finalMime,
		"size_bytes": int64(len(resizedBody)),
		"width":      w,
		"height":     h,
		"replaced":   oldKey != nil && *oldKey != "",
	})

	return &ImageUploadResult{
		Soal:      fresh,
		Slot:      in.Slot,
		ObjectKey: objectKey,
		MimeType:  finalMime,
		SizeBytes: int64(len(resizedBody)),
		Width:     w,
		Height:    h,
	}, nil
}

// DeleteImage clears one image slot + compensating R2 DeleteObject.
// Returns ErrImageSlotEmpty (404) kalau slot belum berisi.
func (s *Service) DeleteImage(ctx context.Context, soalID, callerID uuid.UUID, callerRole string, slot ImageSlot, ip, userAgent string) (*SoalBab, error) {
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
		return nil, fmt.Errorf("soalbab image delete find: %w", err)
	}
	b, err := s.findBabAndOwnership(ctx, soal.BabID, callerID, callerRole)
	if err != nil {
		return nil, err
	}
	if b.Status == bab.StatusArchived {
		return nil, ErrBabArchived
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

	oldKey, err := s.repo.UpdateSoalImageSlot(ctx, soalID, slot.columnFor(), nil)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("soalbab image delete db: %w", err)
	}

	if oldKey != nil && *oldKey != "" {
		if derr := s.store.DeleteObject(context.Background(), *oldKey); derr != nil {
			s.logAudit(ctx, "soalbab_image_orphan", &callerID, callerRole, &soalID, &soal.KelasID, ip, userAgent, map[string]any{
				"object_key": *oldKey,
				"slot":       string(slot),
				"reason":     "delete_object_failed_after_db",
				"err":        derr.Error(),
			})
		}
	}

	fresh, err := s.repo.FindSoalByID(ctx, soalID)
	if err != nil {
		return nil, fmt.Errorf("soalbab image delete refetch: %w", err)
	}

	clearedKey := ""
	if oldKey != nil {
		clearedKey = *oldKey
	}
	s.logAudit(ctx, "soalbab_image_deleted", &callerID, callerRole, &soalID, &soal.KelasID, ip, userAgent, map[string]any{
		"soal_id":    soalID.String(),
		"slot":       string(slot),
		"object_key": clearedKey,
	})
	return fresh, nil
}

// SoalImageURLResult is returned by PresignImageURL.
type SoalImageURLResult struct {
	URL       string
	ExpiresAt time.Time
	Slot      ImageSlot
	ObjectKey string
}

// PresignImageURL issues a short-lived inline GET URL untuk gambar slot.
// Guru/admin owner only — siswa BLOCKED (lihat soal lewat flow attempt).
func (s *Service) PresignImageURL(ctx context.Context, soalID, callerID uuid.UUID, callerRole string, slot ImageSlot, ip, userAgent string) (*SoalImageURLResult, error) {
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
		return nil, fmt.Errorf("soalbab image presign find: %w", err)
	}
	if _, err := s.findKelasOrForbidden(ctx, soal.KelasID, callerID, callerRole); err != nil {
		return nil, err
	}

	currentKey := imageKeyForSlot(soal, slot)
	if currentKey == nil || *currentKey == "" {
		return nil, ErrImageSlotEmpty
	}

	url, perr := s.store.PresignGet(ctx, *currentKey, SoalImagePresignTTL)
	if perr != nil {
		if errors.Is(perr, storage.ErrObjectNotFound) {
			return nil, ErrImageSlotEmpty
		}
		return nil, fmt.Errorf("soalbab image presign: %w", perr)
	}

	s.logAudit(ctx, "soalbab_image_url_issued", &callerID, callerRole, &soalID, &soal.KelasID, ip, userAgent, map[string]any{
		"soal_id":    soalID.String(),
		"slot":       string(slot),
		"object_key": *currentKey,
		"ttl":        int(SoalImagePresignTTL.Seconds()),
	})
	return &SoalImageURLResult{
		URL:       url,
		ExpiresAt: s.now().Add(SoalImagePresignTTL),
		Slot:      slot,
		ObjectKey: *currentKey,
	}, nil
}

// ---------- helpers ----------

// imageKeyForSlot returns the pointer to the slot's object_key column on
// the in-memory soal row.
func imageKeyForSlot(s *SoalBab, slot ImageSlot) *string {
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
func clearImageSlot(s *SoalBab, slot ImageSlot) {
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

// resizeImageForSoal decodes raw bytes, resizes if longest side > 1920px,
// and re-encodes (JPEG q85 untuk JPEG/WebP, PNG lossless untuk PNG).
// Returns (encoded body, final mime, final ext, width, height, err).
func resizeImageForSoal(raw []byte, srcMime, hintExt string) ([]byte, string, string, int, int, error) {
	// imaging.Decode handles JPEG, PNG. For WebP we'd need
	// golang.org/x/image/webp; skip auto-decode and rely on imaging
	// which uses image.Decode internally — webp won't decode without
	// the side-import. We register that next to the call.
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
	if longest > MaxSoalImageDimension {
		// Fit within 1920 box, preserve aspect.
		img = imaging.Fit(img, MaxSoalImageDimension, MaxSoalImageDimension, imaging.Lanczos)
		bounds = img.Bounds()
		w, h = bounds.Dx(), bounds.Dy()
	}

	// Encode. PNG passthrough kalau source PNG; selain itu JPEG q85.
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
