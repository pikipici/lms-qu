// Items endpoint — live attempt soal hydration with anti-cheat strip.
//
// Mirror SoalBab HasilService.Items pattern:
//   - Owner check: hasil.SiswaID == caller.
//   - Status guard: HasilBerlangsung only.
//   - Anti-cheat: jawaban_benar (banksoal.Jawaban field) NOT included
//     dalam response (locked #84/#85). Image slot di-presign saat
//     attempt aktif (TTL 15m, mirror locked #62).
//   - Items dispatch identical untuk manual/random — pool dari
//     hasil.SoalIDsJSON snapshot.
//
// Endpoint: GET /api/v1/siswa/hasil-ujian/:id/items
package ujian

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/pikip/lms/backend/internal/banksoal"
	"github.com/pikip/lms/backend/internal/storage"
)

// ItemImage adalah satu presigned image slot. URL kosong artinya soal
// tidak punya gambar di slot itu (FE skip render).
type ItemImage struct {
	Slot      string     `json:"slot"`
	URL       string     `json:"url"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}

// AttemptItem adalah satu soal di payload Items. Anti-cheat: TIDAK
// include jawaban_benar (locked #85 — siswa cuma boleh tau correct
// answer setelah finish via /review yang gated).
type AttemptItem struct {
	SoalID       uuid.UUID   `json:"soal_id"`
	Pertanyaan   string      `json:"pertanyaan"`
	OpsiA        string      `json:"opsi_a"`
	OpsiB        string      `json:"opsi_b"`
	OpsiC        string      `json:"opsi_c"`
	OpsiD        string      `json:"opsi_d"`
	OpsiE        string      `json:"opsi_e"`
	Poin         int16       `json:"poin"`
	Urutan       int         `json:"urutan"`
	JawabanSiswa *string     `json:"jawaban_siswa,omitempty"`
	Images       []ItemImage `json:"images,omitempty"`
}

// ItemsResult adalah live-attempt payload. Mirror soalbab.ItemsResult
// shape supaya FE shared antara Bab dan Ujian.
type ItemsResult struct {
	HasilID    uuid.UUID     `json:"hasil_id"`
	UjianID    uuid.UUID     `json:"ujian_id"`
	Status     HasilStatus   `json:"status"`
	AttemptNo  int16         `json:"attempt_no"`
	MulaiAt    time.Time     `json:"mulai_at"`
	DeadlineAt *time.Time    `json:"deadline_at,omitempty"`
	Total      int           `json:"total"`
	Items      []AttemptItem `json:"items"`
}

// itemsRepoAPI is the subset of *Repo Items service depends on.
type itemsRepoAPI interface {
	FindHasilByID(ctx context.Context, id uuid.UUID) (*HasilUjian, error)
	ListJawabanByHasil(ctx context.Context, hasilID uuid.UUID) ([]JawabanUjian, error)
}

// ItemsService renders live attempt items dengan anti-cheat.
type ItemsService struct {
	repo  itemsRepoAPI
	bank  bankSoalListLookup
	store storage.Storage
	now   func() time.Time
}

// NewItemsService wires items service. store optional — kalau nil,
// images di-skip dari response (FE handle gracefully).
func NewItemsService(repo itemsRepoAPI, bank bankSoalListLookup, store storage.Storage) *ItemsService {
	return &ItemsService{repo: repo, bank: bank, store: store, now: time.Now}
}

// ItemsImagePresignTTL adalah TTL presigned URL gambar inline. Mirror
// banksoal.BankSoalImagePresignTTL (15m).
const ItemsImagePresignTTL = 15 * time.Minute

// ErrHasilNotOwned signals caller is not the siswa owning hasil.
var ErrHasilNotOwned = errors.New("ujian: hasil not owned by caller")

// ErrHasilNotActive signals attempt is not in status=berlangsung.
var ErrHasilNotActive = errors.New("ujian: hasil not active")

// Items returns live attempt items untuk siswa-owned attempt yang
// status=berlangsung. Caller harus auth siswa; dispatcher caller
// (handler) men-validasi siswa role.
func (s *ItemsService) Items(ctx context.Context, hasilID, siswaID uuid.UUID) (*ItemsResult, error) {
	hasil, err := s.repo.FindHasilByID(ctx, hasilID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("ujian items find: %w", err)
	}
	if hasil.SiswaID != siswaID {
		return nil, ErrHasilNotOwned
	}
	if hasil.Status != HasilBerlangsung {
		return nil, ErrHasilNotActive
	}

	pool, perr := decodeSoalIDsJSONUjian(hasil.SoalIDsJSON)
	if perr != nil {
		return nil, fmt.Errorf("ujian items pool decode: %w", perr)
	}
	soals, err := s.bank.FindSoalsByIDs(ctx, pool)
	if err != nil {
		return nil, fmt.Errorf("ujian items soals: %w", err)
	}
	soalByID := make(map[uuid.UUID]*banksoal.BankSoal, len(soals))
	for i := range soals {
		soalByID[soals[i].ID] = &soals[i]
	}

	jawabans, err := s.repo.ListJawabanByHasil(ctx, hasilID)
	if err != nil {
		return nil, fmt.Errorf("ujian items jawabans: %w", err)
	}
	jawByID := make(map[uuid.UUID]*JawabanUjian, len(jawabans))
	for i := range jawabans {
		jawByID[jawabans[i].SoalID] = &jawabans[i]
	}

	items := make([]AttemptItem, 0, len(pool))
	for idx, sid := range pool {
		soal, ok := soalByID[sid]
		if !ok {
			// Soal soft-deleted post-snapshot. FindSoalsByIDs filters
			// deleted_at IS NULL — placeholder so siswa lihat slot.
			items = append(items, AttemptItem{
				SoalID:     sid,
				Pertanyaan: "(soal sudah dihapus guru, lewati)",
				Urutan:     idx + 1,
			})
			continue
		}
		item := AttemptItem{
			SoalID:     soal.ID,
			Pertanyaan: soal.Pertanyaan,
			OpsiA:      soal.OpsiA,
			OpsiB:      soal.OpsiB,
			OpsiC:      soal.OpsiC,
			OpsiD:      soal.OpsiD,
			OpsiE:      soal.OpsiE,
			Poin:       soal.Poin,
			Urutan:     idx + 1,
			// Anti-cheat: TIDAK set Jawaban (banksoal.Jawaban field).
		}
		if j, ok := jawByID[sid]; ok {
			item.JawabanSiswa = j.Jawaban
			// Anti-cheat: TIDAK set IsBenar — ulangan delayed grade
			// (locked #87 sama dengan SoalBab ulangan).
		}
		// Best-effort presign — kalau store nil atau presign error,
		// slot di-skip.
		if s.store != nil {
			item.Images = s.presignSlots(ctx, soal)
		}
		items = append(items, item)
	}

	return &ItemsResult{
		HasilID:    hasil.ID,
		UjianID:    hasil.UjianID,
		Status:     hasil.Status,
		AttemptNo:  hasil.AttemptNo,
		MulaiAt:    hasil.MulaiAt,
		DeadlineAt: hasil.DeadlineAt,
		Total:      len(pool),
		Items:      items,
	}, nil
}

// presignSlots resolves per-soal image slots ke presigned URL (best-effort).
func (s *ItemsService) presignSlots(ctx context.Context, soal *banksoal.BankSoal) []ItemImage {
	type entry struct {
		slot string
		key  *string
	}
	candidates := []entry{
		{"pertanyaan", soal.PertanyaanObjectKey},
		{"a", soal.OpsiAObjectKey},
		{"b", soal.OpsiBObjectKey},
		{"c", soal.OpsiCObjectKey},
		{"d", soal.OpsiDObjectKey},
		{"e", soal.OpsiEObjectKey},
	}
	out := make([]ItemImage, 0, len(candidates))
	exp := s.now().Add(ItemsImagePresignTTL)
	for _, c := range candidates {
		if c.key == nil || *c.key == "" {
			continue
		}
		url, err := s.store.PresignGet(ctx, *c.key, ItemsImagePresignTTL)
		if err != nil {
			continue
		}
		t := exp
		out = append(out, ItemImage{Slot: c.slot, URL: url, ExpiresAt: &t})
	}
	return out
}
