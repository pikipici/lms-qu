// Service layer untuk ujian: input validation, ownership guard
// (kelas.guru_id == caller, locked Fase 3), optimistic concurrency,
// audit logging, source-mode dispatch (locked #85). Handler stays thin.
//
// Authorization (Task 6.C.1 + 6.C.2):
//
//   - Create/Update/Delete/Duplicate: guru pemilik kelas (kelas.guru_id ==
//     caller) atau admin.
//   - List/Get: guru pemilik OR siswa enrolled (siswa hanya lihat status
//     published; published-only filter di handler/service).
//
// Locked decisions referenced:
//   - #56 optimistic concurrency: PATCH wajib `version`.
//   - #67 R2 CopyObject pattern duplicate (BankSoal pribadi guru — image
//     keys SHARED, tidak deep-copy karena soal_ids referensi soal yang
//     sama; berbeda dari tugas/bab yang punya per-row attachment).
//   - #84 BankSoal scope per-guru pribadi.
//   - #85 source mode discriminated SourceConfigJSON manual/random.
//   - #88 backend coverage gate 70%.
package ujian

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"

	"github.com/pikip/lms/backend/internal/auth"
	"github.com/pikip/lms/backend/internal/banksoal"
	"github.com/pikip/lms/backend/internal/kelas"
)

// Sentinel errors mapped to HTTP status di handler.
var (
	ErrInvalidInput        = errors.New("ujian: invalid input")
	ErrNotFound            = errors.New("ujian: not found")
	ErrForbidden           = errors.New("ujian: forbidden")
	ErrKelasArchived       = errors.New("ujian: kelas archived")
	ErrSourceMissing       = errors.New("ujian: source not configured")
	ErrSoalNotInBank       = errors.New("ujian: soal not in caller's bank")
	ErrSoalEmpty           = errors.New("ujian: source pool empty")
	ErrAttemptsExist       = errors.New("ujian: hasil ujian rows exist; cannot delete")
	ErrActiveAttemptsBlock = errors.New("ujian: active attempts block edit")
)

// Length caps for ujian fields.
const (
	MaxJudulBytes     = 256
	MaxDeskripsiBytes = 8 * 1024
	MaxJumlahSoal     = 200 // hard cap untuk random-mode jumlah_soal
	MinJumlahSoal     = 1
	MinDurasiMenit    = 1
	MaxDurasiMenit    = 300 // 5 jam max (matches DB CHECK ujian_durasi_menit_check di migration 000011)
	MaxManualSoalIDs  = 200
	MinBatasAttempt   = 1
	MaxBatasAttempt   = 999
)

// repoAPI is the subset of *Repo the service depends on.
type repoAPI interface {
	CreateUjian(ctx context.Context, u *Ujian) error
	FindUjianByID(ctx context.Context, id uuid.UUID) (*Ujian, error)
	ListByKelas(ctx context.Context, kelasID uuid.UUID, f UjianListFilter) ([]Ujian, error)
	UpdateUjianBasic(ctx context.Context, id uuid.UUID, expectedVersion int, fields map[string]any) error
	DeleteUjian(ctx context.Context, id uuid.UUID, expectedVersion int) error
	HasActiveAttempts(ctx context.Context, ujianID uuid.UUID) (bool, error)
	CountHasilByUjian(ctx context.Context, ujianID uuid.UUID) (int64, error)
	SetUjianSoalIDs(ctx context.Context, ujianID uuid.UUID, soalIDs []uuid.UUID) error
	ListUjianSoalIDs(ctx context.Context, ujianID uuid.UUID) ([]uuid.UUID, error)
}

// kelasLookup hydrates kelas ownership/lifecycle.
type kelasLookup interface {
	FindByID(ctx context.Context, id uuid.UUID) (*kelas.Kelas, error)
}

// bankSoalLookup ekspor query yang Service butuh dari BankSoal:
//   - per-id verification (manual mode)
//   - filter+pluck-id (random mode preview)
type bankSoalLookup interface {
	FindSoalByID(ctx context.Context, id uuid.UUID) (*banksoal.BankSoal, error)
	ListIDsByOwnerFilter(ctx context.Context, guruID uuid.UUID, f banksoal.ListFilter) ([]uuid.UUID, error)
}

// auditLogger lets the service write audit rows without a hard auth dep.
type auditLogger interface {
	LogAudit(ctx context.Context, entry *auth.AuditLog) error
}

// Service handles ujian business logic.
type Service struct {
	repo  repoAPI
	kelas kelasLookup
	bank  bankSoalLookup
	audit auditLogger
	now   func() time.Time
}

// NewService wires Ujian Repo + kelas/bank lookups + audit logger.
func NewService(repo repoAPI, kelas kelasLookup, bank bankSoalLookup, audit auditLogger) *Service {
	return &Service{repo: repo, kelas: kelas, bank: bank, audit: audit, now: time.Now}
}

// ---------------------------------------------------------------------------
// Source config (locked #85)
// ---------------------------------------------------------------------------

// ManualSourceConfig is the persisted shape for SourceMode=manual.
type ManualSourceConfig struct {
	Mode    SourceMode  `json:"mode"`
	SoalIDs []uuid.UUID `json:"soal_ids"`
}

// RandomFilter narrows BankSoal pool for SourceMode=random.
type RandomFilter struct {
	Mapel   string `json:"mapel,omitempty"`
	Tingkat string `json:"tingkat,omitempty"`
	Topik   string `json:"topik,omitempty"`
}

// RandomSourceConfig is the persisted shape for SourceMode=random.
type RandomSourceConfig struct {
	Mode       SourceMode   `json:"mode"`
	Filter     RandomFilter `json:"filter"`
	JumlahSoal int          `json:"jumlah_soal"`
}

// SourceInput is the union the handler accepts (one of Manual/Random
// must be set when caller wants to (re)configure source).
type SourceInput struct {
	Manual *ManualSourceConfig
	Random *RandomSourceConfig
}

// SourceMode reads the discriminator from a stored SourceConfigJSON
// blob (returns "" if blob empty/invalid).
func PeekSourceMode(blob datatypes.JSON) SourceMode {
	if len(blob) == 0 {
		return ""
	}
	var probe struct {
		Mode SourceMode `json:"mode"`
	}
	if err := json.Unmarshal(blob, &probe); err != nil {
		return ""
	}
	return probe.Mode
}

// ---------------------------------------------------------------------------
// Create
// ---------------------------------------------------------------------------

// CreateInput holds fields for POST /api/v1/kelas/:id/ujian.
type CreateInput struct {
	Judul                      string
	Deskripsi                  string
	DurasiMenit                int16
	WaktuMulai                 *time.Time
	WaktuSelesai               *time.Time
	IzinkanReviewSetelahSubmit bool
	WaktuBukaReview            *time.Time
	BatasAttempt               int16
	AttemptUnlimited           bool
	Bobot                      *int
	Status                     *Status      // optional; default = draft
	Source                     *SourceInput // optional; bisa di-set saat create atau lewat PATCH source
}

// Create publishes an ujian. Owner-only + kelas active.
func (s *Service) Create(ctx context.Context, kelasID, callerID uuid.UUID, callerRole string, in CreateInput, ip, userAgent string) (*Ujian, error) {
	judul := strings.TrimSpace(in.Judul)
	if judul == "" {
		return nil, fmt.Errorf("%w: judul is required", ErrInvalidInput)
	}
	if len(judul) > MaxJudulBytes {
		return nil, fmt.Errorf("%w: judul exceeds %d bytes", ErrInvalidInput, MaxJudulBytes)
	}
	if len(in.Deskripsi) > MaxDeskripsiBytes {
		return nil, fmt.Errorf("%w: deskripsi exceeds %d bytes", ErrInvalidInput, MaxDeskripsiBytes)
	}
	durasi := in.DurasiMenit
	if durasi == 0 {
		durasi = 60
	}
	if durasi < MinDurasiMenit || durasi > MaxDurasiMenit {
		return nil, fmt.Errorf("%w: durasi_menit must be between %d and %d", ErrInvalidInput, MinDurasiMenit, MaxDurasiMenit)
	}
	if in.WaktuMulai != nil && in.WaktuSelesai != nil && in.WaktuSelesai.Before(*in.WaktuMulai) {
		return nil, fmt.Errorf("%w: waktu_selesai before waktu_mulai", ErrInvalidInput)
	}
	batasAttempt := in.BatasAttempt
	if batasAttempt == 0 {
		batasAttempt = 1
	}
	if batasAttempt < MinBatasAttempt || batasAttempt > MaxBatasAttempt {
		return nil, fmt.Errorf("%w: batas_attempt must be between %d and %d", ErrInvalidInput, MinBatasAttempt, MaxBatasAttempt)
	}
	bobot := 100
	if in.Bobot != nil {
		bobot = *in.Bobot
	}
	if bobot < 0 {
		return nil, fmt.Errorf("%w: bobot must be greater than or equal to 0", ErrInvalidInput)
	}

	status := StatusDraft
	if in.Status != nil {
		if !in.Status.Valid() {
			return nil, fmt.Errorf("%w: status must be draft|published|archived", ErrInvalidInput)
		}
		status = *in.Status
	}

	k, err := s.findKelasOrForbidden(ctx, kelasID, callerID, callerRole)
	if err != nil {
		return nil, err
	}
	if k.ArchivedAt != nil {
		return nil, ErrKelasArchived
	}

	// Validate + serialize source if provided.
	var sourceBlob datatypes.JSON
	var manualSoalIDs []uuid.UUID
	if in.Source != nil {
		blob, ids, verr := s.validateAndSerializeSource(ctx, callerID, *in.Source)
		if verr != nil {
			return nil, verr
		}
		sourceBlob = blob
		manualSoalIDs = ids
	} else {
		sourceBlob = datatypes.JSON([]byte("{}"))
	}

	u := &Ujian{
		KelasID:                    kelasID,
		GuruID:                     k.GuruID,
		Judul:                      judul,
		Deskripsi:                  in.Deskripsi,
		DurasiMenit:                durasi,
		WaktuMulai:                 in.WaktuMulai,
		WaktuSelesai:               in.WaktuSelesai,
		SourceConfigJSON:           sourceBlob,
		IzinkanReviewSetelahSubmit: in.IzinkanReviewSetelahSubmit,
		WaktuBukaReview:            in.WaktuBukaReview,
		BatasAttempt:               batasAttempt,
		AttemptUnlimited:           in.AttemptUnlimited,
		Bobot:                      bobot,
		Status:                     status,
		Version:                    1,
	}
	if err := s.repo.CreateUjian(ctx, u); err != nil {
		return nil, fmt.Errorf("ujian create: %w", err)
	}
	// Persist manual junction kalau source.mode=manual.
	if len(manualSoalIDs) > 0 {
		if err := s.repo.SetUjianSoalIDs(ctx, u.ID, manualSoalIDs); err != nil {
			return nil, fmt.Errorf("ujian create junction: %w", err)
		}
	}

	s.logAudit(ctx, "ujian_created", &callerID, callerRole, &u.ID, &kelasID, ip, userAgent, map[string]any{
		"ujian_id":     u.ID.String(),
		"kelas_id":     kelasID.String(),
		"judul":        u.Judul,
		"status":       string(u.Status),
		"durasi_menit": u.DurasiMenit,
		"bobot":        u.Bobot,
		"source_mode":  string(PeekSourceMode(u.SourceConfigJSON)),
	})
	return u, nil
}

// ---------------------------------------------------------------------------
// List
// ---------------------------------------------------------------------------

// ListInput narrows ListByKelas results.
type ListInput struct {
	Status *Status
	Limit  int
	Offset int
}

// ListByKelas returns ujian in a kelas. Authorization branches by role:
// guru/admin see all + can filter status; siswa enrolled forced to
// status='published'. Siswa enrolment check delegated to handler-level
// (kelas pre-check) — tetap mirror tugas pattern di Fase 6.G.
func (s *Service) ListByKelas(ctx context.Context, kelasID, callerID uuid.UUID, callerRole string, in ListInput) ([]Ujian, error) {
	if callerRole == string(auth.Siswa) {
		// Siswa: pin status published. Enrolment guard di Fase 6.G FE flow.
		if _, err := s.kelas.FindByID(ctx, kelasID); err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, ErrForbidden
			}
			return nil, fmt.Errorf("ujian list kelas: %w", err)
		}
		f := UjianListFilter{
			Status: StatusPublished,
			Limit:  in.Limit,
			Offset: in.Offset,
		}
		return s.repo.ListByKelas(ctx, kelasID, f)
	}

	if _, err := s.findKelasOrForbidden(ctx, kelasID, callerID, callerRole); err != nil {
		return nil, err
	}
	f := UjianListFilter{Limit: in.Limit, Offset: in.Offset}
	if in.Status != nil {
		f.Status = *in.Status
	}
	return s.repo.ListByKelas(ctx, kelasID, f)
}

// ---------------------------------------------------------------------------
// Get
// ---------------------------------------------------------------------------

// Get returns an ujian by id. Owner-only / siswa-pinned-published.
func (s *Service) Get(ctx context.Context, id, callerID uuid.UUID, callerRole string) (*Ujian, error) {
	u, err := s.repo.FindUjianByID(ctx, id)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("ujian get: %w", err)
	}
	if callerRole == string(auth.Siswa) {
		if u.Status != StatusPublished {
			return nil, ErrNotFound
		}
		return u, nil
	}
	if _, err := s.findKelasOrForbidden(ctx, u.KelasID, callerID, callerRole); err != nil {
		return nil, err
	}
	return u, nil
}

// ---------------------------------------------------------------------------
// Update (Task 6.C.1)
// ---------------------------------------------------------------------------

// UpdateInput is the PATCH payload.
type UpdateInput struct {
	ExpectedVersion            int
	Judul                      *string
	Deskripsi                  *string
	DurasiMenit                *int16
	WaktuMulai                 *time.Time
	WaktuMulaiExplicit         bool // distinguish absent vs explicit null
	WaktuSelesai               *time.Time
	WaktuSelesaiExplicit       bool
	IzinkanReviewSetelahSubmit *bool
	WaktuBukaReview            *time.Time
	WaktuBukaReviewExplicit    bool
	BatasAttempt               *int16
	AttemptUnlimited           *bool
	Bobot                      *int
	Status                     *Status
	Source                     *SourceInput // optional source change (Task 6.C.2)
}

// Update applies a partial update with optimistic concurrency.
//
// Active-attempts guard: Source/DurasiMenit/WaktuSelesai/WaktuMulai
// changes BLOCKED kalau ada HasilUjian.Status='berlangsung' aktif.
// Status flip (draft↔published↔archived), judul, deskripsi,
// review-gating boleh diubah meskipun ada attempt aktif.
func (s *Service) Update(ctx context.Context, id, callerID uuid.UUID, callerRole string, in UpdateInput, ip, userAgent string) (*Ujian, error) {
	if in.ExpectedVersion <= 0 {
		return nil, fmt.Errorf("%w: version must be positive", ErrInvalidInput)
	}
	existing, err := s.repo.FindUjianByID(ctx, id)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("ujian update find: %w", err)
	}
	if _, err := s.findKelasOrForbidden(ctx, existing.KelasID, callerID, callerRole); err != nil {
		return nil, err
	}

	merged := *existing
	fields := map[string]any{}

	if in.Judul != nil {
		v := strings.TrimSpace(*in.Judul)
		if v == "" {
			return nil, fmt.Errorf("%w: judul cannot be empty", ErrInvalidInput)
		}
		if len(v) > MaxJudulBytes {
			return nil, fmt.Errorf("%w: judul exceeds %d bytes", ErrInvalidInput, MaxJudulBytes)
		}
		if v != merged.Judul {
			merged.Judul = v
			fields["judul"] = v
		}
	}
	if in.Deskripsi != nil {
		if len(*in.Deskripsi) > MaxDeskripsiBytes {
			return nil, fmt.Errorf("%w: deskripsi exceeds %d bytes", ErrInvalidInput, MaxDeskripsiBytes)
		}
		if *in.Deskripsi != merged.Deskripsi {
			merged.Deskripsi = *in.Deskripsi
			fields["deskripsi"] = *in.Deskripsi
		}
	}
	if in.DurasiMenit != nil {
		if *in.DurasiMenit < MinDurasiMenit || *in.DurasiMenit > MaxDurasiMenit {
			return nil, fmt.Errorf("%w: durasi_menit must be between %d and %d", ErrInvalidInput, MinDurasiMenit, MaxDurasiMenit)
		}
		if *in.DurasiMenit != merged.DurasiMenit {
			merged.DurasiMenit = *in.DurasiMenit
			fields["durasi_menit"] = *in.DurasiMenit
		}
	}
	if in.WaktuMulaiExplicit {
		merged.WaktuMulai = in.WaktuMulai
		fields["waktu_mulai"] = in.WaktuMulai
	}
	if in.WaktuSelesaiExplicit {
		merged.WaktuSelesai = in.WaktuSelesai
		fields["waktu_selesai"] = in.WaktuSelesai
	}
	if merged.WaktuMulai != nil && merged.WaktuSelesai != nil && merged.WaktuSelesai.Before(*merged.WaktuMulai) {
		return nil, fmt.Errorf("%w: waktu_selesai before waktu_mulai", ErrInvalidInput)
	}
	if in.IzinkanReviewSetelahSubmit != nil && *in.IzinkanReviewSetelahSubmit != merged.IzinkanReviewSetelahSubmit {
		merged.IzinkanReviewSetelahSubmit = *in.IzinkanReviewSetelahSubmit
		fields["izinkan_review_setelah_submit"] = *in.IzinkanReviewSetelahSubmit
	}
	if in.WaktuBukaReviewExplicit {
		merged.WaktuBukaReview = in.WaktuBukaReview
		fields["waktu_buka_review"] = in.WaktuBukaReview
	}
	if in.BatasAttempt != nil {
		if *in.BatasAttempt < MinBatasAttempt || *in.BatasAttempt > MaxBatasAttempt {
			return nil, fmt.Errorf("%w: batas_attempt must be between %d and %d", ErrInvalidInput, MinBatasAttempt, MaxBatasAttempt)
		}
		if *in.BatasAttempt != merged.BatasAttempt {
			merged.BatasAttempt = *in.BatasAttempt
			fields["batas_attempt"] = *in.BatasAttempt
		}
	}
	if in.AttemptUnlimited != nil && *in.AttemptUnlimited != merged.AttemptUnlimited {
		merged.AttemptUnlimited = *in.AttemptUnlimited
		fields["attempt_unlimited"] = *in.AttemptUnlimited
	}
	if in.Bobot != nil {
		if *in.Bobot < 0 {
			return nil, fmt.Errorf("%w: bobot must be greater than or equal to 0", ErrInvalidInput)
		}
		if *in.Bobot != merged.Bobot {
			merged.Bobot = *in.Bobot
			fields["bobot"] = *in.Bobot
		}
	}
	if in.Status != nil {
		if !in.Status.Valid() {
			return nil, fmt.Errorf("%w: status must be draft|published|archived", ErrInvalidInput)
		}
		if *in.Status != merged.Status {
			merged.Status = *in.Status
			fields["status"] = *in.Status
		}
	}

	// Source change requires active-attempt guard + bank validation.
	var newSourceIDs []uuid.UUID
	sourceChange := in.Source != nil
	if sourceChange {
		blob, ids, verr := s.validateAndSerializeSource(ctx, existing.GuruID, *in.Source)
		if verr != nil {
			return nil, verr
		}
		merged.SourceConfigJSON = blob
		fields["source_config_json"] = blob
		newSourceIDs = ids
	}

	// Active-attempts guard: timing/source changes BLOCKED kalau ada attempt aktif.
	timingChange := false
	for _, k := range []string{"durasi_menit", "waktu_mulai", "waktu_selesai"} {
		if _, ok := fields[k]; ok {
			timingChange = true
			break
		}
	}
	if sourceChange || timingChange {
		active, aerr := s.repo.HasActiveAttempts(ctx, id)
		if aerr != nil {
			return nil, fmt.Errorf("ujian update active-attempts probe: %w", aerr)
		}
		if active {
			return nil, ErrActiveAttemptsBlock
		}
	}

	if len(fields) == 0 {
		return existing, nil
	}

	if err := s.repo.UpdateUjianBasic(ctx, id, in.ExpectedVersion, fields); err != nil {
		return nil, mapRepoErr(err)
	}

	// Persist manual junction kalau source flipped ke manual; clear kalau ke random.
	if sourceChange {
		if err := s.repo.SetUjianSoalIDs(ctx, id, newSourceIDs); err != nil {
			return nil, fmt.Errorf("ujian update junction: %w", err)
		}
	}

	fresh, err := s.repo.FindUjianByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("ujian update refetch: %w", err)
	}

	s.logAudit(ctx, "ujian_updated", &callerID, callerRole, &id, &existing.KelasID, ip, userAgent, map[string]any{
		"ujian_id":    id.String(),
		"old_version": existing.Version,
		"new_version": fresh.Version,
		"changed":     fieldKeys(fields),
		"source_mode": string(PeekSourceMode(fresh.SourceConfigJSON)),
	})
	return fresh, nil
}

// ---------------------------------------------------------------------------
// Delete (Task 6.C.1)
// ---------------------------------------------------------------------------

// Delete hard-deletes an ujian. Allowed only when no HasilUjian rows
// exist (ujian belum pernah dimulai). Otherwise return ErrAttemptsExist
// — guru harus archive instead.
func (s *Service) Delete(ctx context.Context, id, callerID uuid.UUID, callerRole string, expectedVersion int, ip, userAgent string) (*Ujian, error) {
	if expectedVersion <= 0 {
		return nil, fmt.Errorf("%w: version must be positive", ErrInvalidInput)
	}
	existing, err := s.repo.FindUjianByID(ctx, id)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("ujian delete find: %w", err)
	}
	if _, err := s.findKelasOrForbidden(ctx, existing.KelasID, callerID, callerRole); err != nil {
		return nil, err
	}

	count, err := s.repo.CountHasilByUjian(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("ujian delete probe: %w", err)
	}
	if count > 0 {
		return nil, ErrAttemptsExist
	}

	if err := s.repo.DeleteUjian(ctx, id, expectedVersion); err != nil {
		return nil, mapRepoErr(err)
	}

	s.logAudit(ctx, "ujian_deleted", &callerID, callerRole, &id, &existing.KelasID, ip, userAgent, map[string]any{
		"ujian_id": id.String(),
		"judul":    existing.Judul,
	})
	return existing, nil
}

// ---------------------------------------------------------------------------
// Duplicate (Task 6.C.1, locked #67)
// ---------------------------------------------------------------------------

// DuplicateInput overrides for POST /api/v1/ujian/:id/duplicate.
type DuplicateInput struct {
	Judul string // empty = "<source.judul> (Salinan)"
}

// Duplicate clones an ujian into the SAME kelas. Source config + manual
// junction are deep-copied. Status reset ke draft, version=1.
//
// Locked #67 R2 CopyObject: BankSoal images are PER-GURU PRIBADI (locked
// #84) and shared across ujian — kita TIDAK deep-copy R2 keys (beda
// dari tugas/bab yang punya per-row attachment). Soal_ids di junction
// referensi BankSoal id yang sama; gambar tetap valid karena guru
// pemilik soal tidak berubah.
func (s *Service) Duplicate(ctx context.Context, srcID, callerID uuid.UUID, callerRole string, in DuplicateInput, ip, userAgent string) (*Ujian, error) {
	src, err := s.repo.FindUjianByID(ctx, srcID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("ujian duplicate find: %w", err)
	}
	k, err := s.findKelasOrForbidden(ctx, src.KelasID, callerID, callerRole)
	if err != nil {
		return nil, err
	}
	if k.ArchivedAt != nil {
		return nil, ErrKelasArchived
	}

	judul := strings.TrimSpace(in.Judul)
	if judul == "" {
		judul = src.Judul + " (Salinan)"
	}
	if len(judul) > MaxJudulBytes {
		return nil, fmt.Errorf("%w: judul exceeds %d bytes", ErrInvalidInput, MaxJudulBytes)
	}

	// Deep-copy source blob (datatypes.JSON is a []byte alias — clone
	// to avoid sharing slice header with src row in cache).
	var srcBlobCopy datatypes.JSON
	if len(src.SourceConfigJSON) > 0 {
		buf := make([]byte, len(src.SourceConfigJSON))
		copy(buf, src.SourceConfigJSON)
		srcBlobCopy = datatypes.JSON(buf)
	} else {
		srcBlobCopy = datatypes.JSON([]byte("{}"))
	}

	dst := &Ujian{
		KelasID:                    src.KelasID,
		GuruID:                     k.GuruID,
		Judul:                      judul,
		Deskripsi:                  src.Deskripsi,
		DurasiMenit:                src.DurasiMenit,
		WaktuMulai:                 src.WaktuMulai,
		WaktuSelesai:               src.WaktuSelesai,
		SourceConfigJSON:           srcBlobCopy,
		IzinkanReviewSetelahSubmit: src.IzinkanReviewSetelahSubmit,
		WaktuBukaReview:            src.WaktuBukaReview,
		Bobot:                      src.Bobot,
		Status:                     StatusDraft, // duplicate selalu reset ke draft
		Version:                    1,
	}
	if err := s.repo.CreateUjian(ctx, dst); err != nil {
		return nil, fmt.Errorf("ujian duplicate create: %w", err)
	}

	// Mirror manual junction kalau source mode=manual.
	srcIDs, err := s.repo.ListUjianSoalIDs(ctx, src.ID)
	if err != nil {
		return nil, fmt.Errorf("ujian duplicate junction read: %w", err)
	}
	if len(srcIDs) > 0 {
		if err := s.repo.SetUjianSoalIDs(ctx, dst.ID, srcIDs); err != nil {
			return nil, fmt.Errorf("ujian duplicate junction write: %w", err)
		}
	}

	s.logAudit(ctx, "ujian_duplicated", &callerID, callerRole, &dst.ID, &src.KelasID, ip, userAgent, map[string]any{
		"source_ujian_id": src.ID.String(),
		"new_ujian_id":    dst.ID.String(),
		"new_judul":       dst.Judul,
		"soal_count":      len(srcIDs),
	})
	return dst, nil
}

// ---------------------------------------------------------------------------
// Source preview (Task 6.C.2)
// ---------------------------------------------------------------------------

// SourcePreview captures what a guru sees before saving source config.
// PreviewIDs is capped to a reasonable number to keep the response
// small (FE shows count + a sample).
type SourcePreview struct {
	Mode       SourceMode  `json:"mode"`
	PoolSize   int         `json:"pool_size"`
	JumlahSoal int         `json:"jumlah_soal"` // for manual: len(soal_ids); for random: filter cap
	SoalIDs    []uuid.UUID `json:"soal_ids,omitempty"`
}

const previewSampleCap = 50

// PreviewSource returns the deterministic pool size + sample IDs for
// a SourceInput WITHOUT persisting it. Used by FE wizard untuk
// confirm before save (Task 6.C.2).
func (s *Service) PreviewSource(ctx context.Context, ujianID, callerID uuid.UUID, callerRole string, in SourceInput) (*SourcePreview, error) {
	u, err := s.repo.FindUjianByID(ctx, ujianID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("ujian preview find: %w", err)
	}
	if _, err := s.findKelasOrForbidden(ctx, u.KelasID, callerID, callerRole); err != nil {
		return nil, err
	}

	switch {
	case in.Manual != nil:
		ids := dedupUUIDs(in.Manual.SoalIDs)
		if len(ids) == 0 {
			return nil, fmt.Errorf("%w: manual.soal_ids empty", ErrInvalidInput)
		}
		if len(ids) > MaxManualSoalIDs {
			return nil, fmt.Errorf("%w: manual.soal_ids exceeds %d", ErrInvalidInput, MaxManualSoalIDs)
		}
		if err := s.assertSoalsOwnedBy(ctx, ids, u.GuruID); err != nil {
			return nil, err
		}
		return &SourcePreview{
			Mode:       SourceManual,
			PoolSize:   len(ids),
			JumlahSoal: len(ids),
			SoalIDs:    capIDs(ids, previewSampleCap),
		}, nil
	case in.Random != nil:
		cfg := *in.Random
		if cfg.JumlahSoal < MinJumlahSoal || cfg.JumlahSoal > MaxJumlahSoal {
			return nil, fmt.Errorf("%w: random.jumlah_soal must be %d..%d", ErrInvalidInput, MinJumlahSoal, MaxJumlahSoal)
		}
		f := banksoal.ListFilter{
			Mapel:   strings.TrimSpace(cfg.Filter.Mapel),
			Tingkat: strings.TrimSpace(cfg.Filter.Tingkat),
			Topik:   strings.TrimSpace(cfg.Filter.Topik),
		}
		ids, perr := s.bank.ListIDsByOwnerFilter(ctx, u.GuruID, f)
		if perr != nil {
			return nil, fmt.Errorf("ujian preview random pool: %w", perr)
		}
		if len(ids) == 0 {
			return nil, ErrSoalEmpty
		}
		if len(ids) < cfg.JumlahSoal {
			return nil, fmt.Errorf("%w: pool has %d soal but jumlah_soal=%d", ErrInvalidInput, len(ids), cfg.JumlahSoal)
		}
		return &SourcePreview{
			Mode:       SourceRandom,
			PoolSize:   len(ids),
			JumlahSoal: cfg.JumlahSoal,
			SoalIDs:    capIDs(ids, previewSampleCap),
		}, nil
	default:
		return nil, ErrSourceMissing
	}
}

// ---------------------------------------------------------------------------
// Source validation helpers
// ---------------------------------------------------------------------------

// validateAndSerializeSource validates the SourceInput against caller's
// BankSoal pool and returns the serialized JSON blob + the manual
// soal_ids slice (empty for random mode).
func (s *Service) validateAndSerializeSource(ctx context.Context, guruID uuid.UUID, in SourceInput) (datatypes.JSON, []uuid.UUID, error) {
	switch {
	case in.Manual != nil:
		ids := dedupUUIDs(in.Manual.SoalIDs)
		if len(ids) == 0 {
			return nil, nil, fmt.Errorf("%w: manual.soal_ids empty", ErrInvalidInput)
		}
		if len(ids) > MaxManualSoalIDs {
			return nil, nil, fmt.Errorf("%w: manual.soal_ids exceeds %d", ErrInvalidInput, MaxManualSoalIDs)
		}
		if err := s.assertSoalsOwnedBy(ctx, ids, guruID); err != nil {
			return nil, nil, err
		}
		cfg := ManualSourceConfig{Mode: SourceManual, SoalIDs: ids}
		buf, err := json.Marshal(cfg)
		if err != nil {
			return nil, nil, fmt.Errorf("ujian source marshal: %w", err)
		}
		return datatypes.JSON(buf), ids, nil
	case in.Random != nil:
		cfg := *in.Random
		cfg.Mode = SourceRandom
		if cfg.JumlahSoal < MinJumlahSoal || cfg.JumlahSoal > MaxJumlahSoal {
			return nil, nil, fmt.Errorf("%w: random.jumlah_soal must be %d..%d", ErrInvalidInput, MinJumlahSoal, MaxJumlahSoal)
		}
		// Eagerly validate filter narrows ke pool non-empty di guru ini.
		f := banksoal.ListFilter{
			Mapel:   strings.TrimSpace(cfg.Filter.Mapel),
			Tingkat: strings.TrimSpace(cfg.Filter.Tingkat),
			Topik:   strings.TrimSpace(cfg.Filter.Topik),
		}
		ids, perr := s.bank.ListIDsByOwnerFilter(ctx, guruID, f)
		if perr != nil {
			return nil, nil, fmt.Errorf("ujian source random pool: %w", perr)
		}
		if len(ids) == 0 {
			return nil, nil, ErrSoalEmpty
		}
		if len(ids) < cfg.JumlahSoal {
			return nil, nil, fmt.Errorf("%w: pool has %d soal but jumlah_soal=%d", ErrInvalidInput, len(ids), cfg.JumlahSoal)
		}
		buf, err := json.Marshal(cfg)
		if err != nil {
			return nil, nil, fmt.Errorf("ujian source marshal: %w", err)
		}
		return datatypes.JSON(buf), nil, nil // random mode: NO junction
	default:
		return nil, nil, ErrSourceMissing
	}
}

// assertSoalsOwnedBy verifies every id in `ids` exists, is non-deleted,
// and is owned by guruID. Anti-cheat untuk locked #84.
func (s *Service) assertSoalsOwnedBy(ctx context.Context, ids []uuid.UUID, guruID uuid.UUID) error {
	for _, id := range ids {
		soal, err := s.bank.FindSoalByID(ctx, id)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("%w: %s", ErrSoalNotInBank, id)
		}
		if err != nil {
			return fmt.Errorf("ujian source verify: %w", err)
		}
		if soal.OwnerGuruID != guruID {
			return fmt.Errorf("%w: %s", ErrSoalNotInBank, id)
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func (s *Service) findKelasOrForbidden(ctx context.Context, kelasID, callerID uuid.UUID, callerRole string) (*kelas.Kelas, error) {
	k, err := s.kelas.FindByID(ctx, kelasID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("ujian kelas find: %w", err)
	}
	if !canManageKelas(k, callerID, callerRole) {
		return nil, ErrForbidden
	}
	return k, nil
}

// canManageKelas: admin OR guru pemilik kelas.
func canManageKelas(k *kelas.Kelas, callerID uuid.UUID, callerRole string) bool {
	if k == nil {
		return false
	}
	if callerRole == string(auth.Admin) {
		return true
	}
	if callerRole == string(auth.Guru) && k.GuruID == callerID {
		return true
	}
	return false
}

func dedupUUIDs(in []uuid.UUID) []uuid.UUID {
	seen := make(map[uuid.UUID]struct{}, len(in))
	out := make([]uuid.UUID, 0, len(in))
	for _, id := range in {
		if id == uuid.Nil {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

func capIDs(in []uuid.UUID, max int) []uuid.UUID {
	if len(in) <= max {
		return in
	}
	out := make([]uuid.UUID, max)
	copy(out, in[:max])
	return out
}

func mapRepoErr(err error) error {
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		return ErrNotFound
	case errors.Is(err, ErrVersionConflict):
		return ErrVersionConflict
	default:
		return fmt.Errorf("ujian repo: %w", err)
	}
}

func fieldKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// logAudit best-effort.
func (s *Service) logAudit(ctx context.Context, action string, actorID *uuid.UUID, actorRole string, targetID, targetKelasID *uuid.UUID, ip, userAgent string, meta map[string]any) {
	if s.audit == nil {
		return
	}
	targetType := "ujian"
	role := actorRole
	if actorID == nil {
		role = ""
	}
	entry := &auth.AuditLog{
		ActorID:       actorID,
		ActorRole:     ptrString(role),
		Action:        action,
		TargetType:    &targetType,
		TargetID:      targetID,
		TargetKelasID: targetKelasID,
		Meta:          marshalMeta(meta),
		IP:            ptrString(ip),
		UserAgent:     ptrString(userAgent),
		At:            s.now(),
	}
	_ = s.audit.LogAudit(ctx, entry)
}

func ptrString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func marshalMeta(fields map[string]any) datatypes.JSON {
	if len(fields) == 0 {
		return datatypes.JSON([]byte("{}"))
	}
	buf, err := json.Marshal(fields)
	if err != nil {
		return datatypes.JSON([]byte("{}"))
	}
	return datatypes.JSON(buf)
}
