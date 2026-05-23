package soalbab

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

func TestEnumValid(t *testing.T) {
	for _, m := range []Mode{ModeLatihan, ModeUlangan, ModeKeduanya} {
		if !m.Valid() {
			t.Fatalf("Mode(%q).Valid() = false", m)
		}
	}
	if Mode("x").Valid() {
		t.Fatal("invalid Mode returned true")
	}

	for _, s := range []HasilStatus{HasilBerlangsung, HasilSelesai, HasilDibatalkan} {
		if !s.Valid() {
			t.Fatalf("HasilStatus(%q).Valid() = false", s)
		}
	}
	if HasilStatus("expired").Valid() {
		t.Fatal("invalid HasilStatus returned true")
	}

	for _, m := range []HasilMode{HasilModeLatihan, HasilModeUlangan} {
		if !m.Valid() {
			t.Fatalf("HasilMode(%q).Valid() = false", m)
		}
	}
	if HasilMode("remedial").Valid() {
		t.Fatal("invalid HasilMode returned true")
	}

	for _, j := range []Jawaban{JawabanA, JawabanB, JawabanC, JawabanD, JawabanE} {
		if !j.Valid() {
			t.Fatalf("Jawaban(%q).Valid() = false", j)
		}
	}
	if Jawaban("z").Valid() {
		t.Fatal("invalid Jawaban returned true")
	}
}

func TestValidateSoalFields(t *testing.T) {
	svc := &Service{}
	valid := func() *SoalBab {
		return &SoalBab{
			Pertanyaan: "Apa ibu kota Indonesia?",
			OpsiA:      "Jakarta",
			OpsiB:      "Bandung",
			OpsiC:      "Surabaya",
			OpsiD:      "Medan",
			OpsiE:      "Makassar",
			Jawaban:    JawabanA,
			Poin:       10,
			Mode:       ModeKeduanya,
		}
	}

	tests := []struct {
		name string
		mut  func(*SoalBab)
		want error
	}{
		{name: "valid", mut: func(*SoalBab) {}, want: nil},
		{name: "invalid answer letter", mut: func(s *SoalBab) { s.Jawaban = "z" }, want: ErrInvalidInput},
		{name: "empty question text and image", mut: func(s *SoalBab) { s.Pertanyaan = "" }, want: ErrInvalidInput},
		{name: "selected answer empty", mut: func(s *SoalBab) { s.Jawaban = JawabanE; s.OpsiE = "" }, want: ErrJawabanInvalid},
		{name: "selected answer can use image only", mut: func(s *SoalBab) { key := "soal/e.png"; s.Jawaban = JawabanE; s.OpsiE = ""; s.OpsiEObjectKey = &key }, want: nil},
		{name: "poin too low", mut: func(s *SoalBab) { s.Poin = -1 }, want: ErrInvalidInput},
		{name: "poin zero allowed default", mut: func(s *SoalBab) { s.Poin = 0 }, want: nil},
		{name: "invalid mode", mut: func(s *SoalBab) { s.Mode = "quiz" }, want: ErrInvalidInput},
		{name: "oversize pertanyaan", mut: func(s *SoalBab) { s.Pertanyaan = string(make([]byte, MaxPertanyaanBytes+1)) }, want: ErrInvalidInput},
		{name: "oversize opsi", mut: func(s *SoalBab) { s.OpsiB = string(make([]byte, MaxOpsiBytes+1)) }, want: ErrInvalidInput},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			soal := valid()
			tt.mut(soal)
			err := svc.validateSoalFields(soal)
			if tt.want == nil {
				if err != nil {
					t.Fatalf("validateSoalFields() error = %v", err)
				}
				return
			}
			if !errors.Is(err, tt.want) {
				t.Fatalf("validateSoalFields() error = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestOptionHelpers(t *testing.T) {
	keys := []string{"ka", "kb", "kc", "kd", "ke"}
	soal := &SoalBab{
		OpsiA: "A", OpsiB: "B", OpsiC: "C", OpsiD: "D", OpsiE: "E",
		OpsiAObjectKey: &keys[0], OpsiBObjectKey: &keys[1], OpsiCObjectKey: &keys[2], OpsiDObjectKey: &keys[3], OpsiEObjectKey: &keys[4],
	}
	for _, tt := range []struct {
		j       Jawaban
		wantTxt string
		wantKey string
	}{
		{JawabanA, "A", "ka"}, {JawabanB, "B", "kb"}, {JawabanC, "C", "kc"}, {JawabanD, "D", "kd"}, {JawabanE, "E", "ke"},
	} {
		if got := optionTextFor(soal, tt.j); got != tt.wantTxt {
			t.Fatalf("optionTextFor(%s) = %q", tt.j, got)
		}
		gotKey := optionImageFor(soal, tt.j)
		if gotKey == nil || *gotKey != tt.wantKey {
			t.Fatalf("optionImageFor(%s) = %v, want %s", tt.j, gotKey, tt.wantKey)
		}
	}
	if optionTextFor(soal, "z") != "" {
		t.Fatal("optionTextFor invalid should be empty")
	}
	if optionImageFor(soal, "z") != nil {
		t.Fatal("optionImageFor invalid should be nil")
	}
}

func TestServiceSmallHelpers(t *testing.T) {
	if !errors.Is(mapRepoErr(gorm.ErrRecordNotFound), ErrNotFound) {
		t.Fatal("mapRepoErr record not found should map to ErrNotFound")
	}
	if !errors.Is(mapRepoErr(ErrVersionConflict), ErrVersionConflict) {
		t.Fatal("mapRepoErr version conflict should preserve ErrVersionConflict")
	}
	if err := mapRepoErr(errors.New("boom")); err == nil || !errors.Is(err, errors.New("boom")) {
		// errors.Is cannot match a new error value; just assert wrapper is non-nil.
		if err == nil {
			t.Fatal("mapRepoErr unexpected nil")
		}
	}

	fields := map[string]any{"b": 2, "a": 1}
	if len(fieldKeys(fields)) != 2 {
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
	if marshalMeta(nil) != nil {
		t.Fatal("marshalMeta empty should be nil")
	}
}

func TestSettingHelpers(t *testing.T) {
	valid := UpsertSettingInput{JumlahSoal: SettingMinJumlahSoal, DurasiMenit: SettingMinDurasiMenit, BatasAttempt: SettingMinBatasAttempt}
	if err := validateSettingBounds(valid); err != nil {
		t.Fatalf("validateSettingBounds(valid) error = %v", err)
	}
	for _, tt := range []UpsertSettingInput{
		{JumlahSoal: SettingMinJumlahSoal - 1, DurasiMenit: SettingMinDurasiMenit, BatasAttempt: SettingMinBatasAttempt},
		{JumlahSoal: SettingMinJumlahSoal, DurasiMenit: SettingMaxDurasiMenit + 1, BatasAttempt: SettingMinBatasAttempt},
		{JumlahSoal: SettingMinJumlahSoal, DurasiMenit: SettingMinDurasiMenit, BatasAttempt: SettingMaxBatasAttempt + 1},
	} {
		if !errors.Is(validateSettingBounds(tt), ErrInvalidInput) {
			t.Fatalf("validateSettingBounds(%+v) should return ErrInvalidInput", tt)
		}
	}

	babID := uuid.New()
	def := defaultSettingView(babID, 12)
	if def.BabID != babID || def.PoolSize != 12 || def.Configured || !def.IzinkanReviewSetelahSubmit {
		t.Fatalf("defaultSettingView mismatch: %+v", def)
	}
	lobby := defaultLobbyView(babID)
	if lobby.BabID != babID || lobby.Configured || !lobby.IzinkanReviewSetelahSubmit {
		t.Fatalf("defaultLobbyView mismatch: %+v", lobby)
	}

	now := time.Date(2026, 5, 23, 10, 0, 0, 0, time.UTC)
	reviewAt := now.Add(time.Hour)
	row := &UlanganBabSetting{BabID: babID, JumlahSoal: 7, DurasiMenit: 20, BatasAttempt: 3, IzinkanReviewSetelahSubmit: false, WaktuBukaReview: &reviewAt, Version: 4, CreatedAt: now, UpdatedAt: now}
	view := settingViewFromRow(row, 9)
	if !view.Configured || view.JumlahSoal != 7 || view.PoolSize != 9 || view.CreatedAt == nil || view.UpdatedAt == nil || view.WaktuBukaReview == nil {
		t.Fatalf("settingViewFromRow mismatch: %+v", view)
	}
}
