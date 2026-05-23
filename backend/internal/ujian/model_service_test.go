package ujian

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/pikip/lms/backend/internal/auth"
	"github.com/pikip/lms/backend/internal/kelas"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

func TestEnumValid(t *testing.T) {
	for _, m := range []SourceMode{SourceManual, SourceRandom} {
		if !m.Valid() {
			t.Fatalf("SourceMode(%q).Valid() = false", m)
		}
	}
	if SourceMode("bank").Valid() {
		t.Fatal("invalid SourceMode returned true")
	}

	for _, s := range []Status{StatusDraft, StatusPublished, StatusArchived} {
		if !s.Valid() {
			t.Fatalf("Status(%q).Valid() = false", s)
		}
	}
	if Status("deleted").Valid() {
		t.Fatal("invalid Status returned true")
	}

	for _, s := range []HasilStatus{HasilBerlangsung, HasilSelesai, HasilDibatalkan} {
		if !s.Valid() {
			t.Fatalf("HasilStatus(%q).Valid() = false", s)
		}
	}
	if HasilStatus("expired").Valid() {
		t.Fatal("invalid HasilStatus returned true")
	}
}

func TestTableNames(t *testing.T) {
	tests := map[string]string{
		Ujian{}.TableName():        "ujian",
		UjianSoal{}.TableName():    "ujian_soal",
		HasilUjian{}.TableName():   "hasil_ujian",
		JawabanUjian{}.TableName(): "jawaban_ujian",
		EventUjian{}.TableName():   "event_ujian",
	}
	for got, want := range tests {
		if got != want {
			t.Fatalf("TableName() = %q, want %q", got, want)
		}
	}
}

func TestCanManageKelas(t *testing.T) {
	guruID := uuid.New()
	otherID := uuid.New()
	k := &kelas.Kelas{GuruID: guruID}
	if !canManageKelas(k, otherID, string(auth.Admin)) {
		t.Fatal("admin should manage kelas")
	}
	if !canManageKelas(k, guruID, string(auth.Guru)) {
		t.Fatal("owner guru should manage kelas")
	}
	if canManageKelas(k, otherID, string(auth.Guru)) {
		t.Fatal("non-owner guru should not manage kelas")
	}
	if canManageKelas(nil, guruID, string(auth.Admin)) {
		t.Fatal("nil kelas should not be manageable")
	}
}

func TestSmallHelpers(t *testing.T) {
	a := uuid.New()
	b := uuid.New()
	got := dedupUUIDs([]uuid.UUID{uuid.Nil, a, b, a, uuid.Nil})
	if len(got) != 2 || got[0] != a || got[1] != b {
		t.Fatalf("dedupUUIDs() = %v", got)
	}
	capped := capIDs([]uuid.UUID{a, b}, 1)
	if len(capped) != 1 || capped[0] != a {
		t.Fatalf("capIDs() = %v", capped)
	}
	uncapped := capIDs([]uuid.UUID{a}, 5)
	if len(uncapped) != 1 || uncapped[0] != a {
		t.Fatalf("capIDs uncapped = %v", uncapped)
	}

	if !errors.Is(mapRepoErr(gorm.ErrRecordNotFound), ErrNotFound) {
		t.Fatal("mapRepoErr record not found should map ErrNotFound")
	}
	if !errors.Is(mapRepoErr(ErrVersionConflict), ErrVersionConflict) {
		t.Fatal("mapRepoErr version conflict should preserve ErrVersionConflict")
	}
	if err := mapRepoErr(errors.New("boom")); err == nil {
		t.Fatal("mapRepoErr generic should wrap error")
	}
	if len(fieldKeys(map[string]any{"a": 1, "b": 2})) != 2 {
		t.Fatal("fieldKeys length mismatch")
	}
	if ptrString("") != nil {
		t.Fatal("ptrString empty should be nil")
	}
	if got := ptrString("x"); got == nil || *got != "x" {
		t.Fatal("ptrString non-empty mismatch")
	}

	meta := marshalMeta(map[string]any{"x": "y"})
	var decoded map[string]string
	if err := json.Unmarshal(meta, &decoded); err != nil || decoded["x"] != "y" {
		t.Fatalf("marshalMeta decoded=%v err=%v", decoded, err)
	}
	if string(marshalMeta(nil)) != "{}" {
		t.Fatal("marshalMeta nil should be {}")
	}
}

func TestStartHelpers(t *testing.T) {
	ujianID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	siswaID := uuid.MustParse("22222222-2222-2222-2222-222222222222")
	mulai := time.Date(2026, 5, 23, 10, 0, 0, 123000, time.UTC)

	if deriveSeedUjian(mulai, siswaID, ujianID) != deriveSeedUjian(mulai, siswaID, ujianID) {
		t.Fatal("deriveSeedUjian should be deterministic")
	}
	if deriveSeedUjian(mulai, siswaID, ujianID) == deriveSeedUjian(mulai.Add(time.Microsecond), siswaID, ujianID) {
		t.Fatal("deriveSeedUjian should vary by mulai_at")
	}
	if startLockKey(ujianID, siswaID) != startLockKey(ujianID, siswaID) {
		t.Fatal("startLockKey should be deterministic")
	}

	raw, err := json.Marshal([]uuid.UUID{ujianID, siswaID})
	if err != nil {
		t.Fatal(err)
	}
	ids, err := decodeSoalIDsJSONUjian(datatypes.JSON(raw))
	if err != nil || len(ids) != 2 || ids[0] != ujianID || ids[1] != siswaID {
		t.Fatalf("decodeSoalIDsJSONUjian() ids=%v err=%v", ids, err)
	}
	ids, err = decodeSoalIDsJSONUjian(nil)
	if err != nil || ids != nil {
		t.Fatalf("decodeSoalIDsJSONUjian(nil) ids=%v err=%v", ids, err)
	}
	if _, err := decodeSoalIDsJSONUjian(datatypes.JSON(`not-json`)); err == nil {
		t.Fatal("decodeSoalIDsJSONUjian invalid json should error")
	}
	if !containsUUIDUjian([]uuid.UUID{ujianID}, ujianID) {
		t.Fatal("containsUUIDUjian should find target")
	}
	if containsUUIDUjian([]uuid.UUID{ujianID}, siswaID) {
		t.Fatal("containsUUIDUjian should not find absent target")
	}
}
