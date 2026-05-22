// Bulk paste pipe-delimited untuk BankSoal (Task 6.B.3).
//
// Format per line (8 kolom, pipe `|` separator, escape `\|`):
//
//	pertanyaan|opsi_a|opsi_b|opsi_c|opsi_d|opsi_e|jawaban|poin
//
// - jawaban: a|b|c|d|e (single-answer MVP).
// - poin: integer 1-100. Kosong → 1.
// - Pipe escape: `\|` di body → `|` literal. `\\` → `\` literal.
// - Skip blank line.
// - Skip line dimulai `#` (komentar).
// - Cap 200 line per request (anti-DoS, mirror Task 5.B.3).
//
// Beda dari SoalBab.BulkCreate (Fase 5.B.3):
//   - 8 kolom (drop `mode` column — BankSoal cross-bab, no mode field).
//   - Top-level body: tag default (mapel/tingkat/topik) di-apply ke semua
//     soal hasil parse — locked #84 BankSoal punya tag tapi non-FK.
//   - Owner-scope guard via canWriteBank (admin+guru), ownership otomatis
//     karena setiap soal dibikin dengan owner_guru_id = caller.
//
// Endpoint: POST /api/v1/bank-soal/bulk body
//
//	{rows: string, mapel?: string, tingkat?: string, topik?: string}
//
// Response 200 partial-success:
//
//	{created: int, errors: [{line: int, reason: string, raw: string}]}
//
// Hard preconditions (4xx, abort sebelum parse):
//   - 400 invalid_body / rows_required / too_many / mapel_too_long /
//     tingkat_too_long / topik_too_long
//   - 401 unauthorized (handled di middleware)
//   - 403 forbidden (siswa BLOCKED via RoleGuard di main.go)
//
// Soft per-line failures (200, kumpul di errors[]): invalid_columns,
// empty_pertanyaan, empty_jawaban_option, invalid_jawaban, invalid_poin,
// pertanyaan_too_long, opsi_too_long.
//
// Audit: banksoal_bulk_created w/ meta {created, error_count, source,
// mapel, tingkat, topik}.
package banksoal

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/google/uuid"
)

// MaxBulkLines caps the number of lines accepted per bulk paste request
// (anti-DoS + bounds parse cost). Mirror SoalBab Task 5.B.3.
const MaxBulkLines = 200

// Reason codes per line — stable enum, FE maps ke localized copy.
const (
	ReasonInvalidColumns     = "invalid_columns"
	ReasonEmptyPertanyaan    = "empty_pertanyaan"
	ReasonEmptyJawabanOption = "empty_jawaban_option"
	ReasonInvalidJawaban     = "invalid_jawaban"
	ReasonInvalidPoin        = "invalid_poin"
	ReasonPertanyaanTooLong  = "pertanyaan_too_long"
	ReasonOpsiTooLong        = "opsi_too_long"
	ReasonInternal           = "internal"
)

// Sentinel errors mapped ke HTTP di handler (hard precondition only).
var (
	ErrBulkRowsRequired = errors.New("banksoal: rows required")
	ErrBulkTooMany      = errors.New("banksoal: too many lines")
)

// BulkCreateInput holds the pasted body + default tag overrides.
type BulkCreateInput struct {
	Rows    string
	Mapel   string
	Tingkat string
	Topik   string
}

// BulkLineError reports one line-level failure with stable reason code +
// raw line text untuk FE display.
type BulkLineError struct {
	Line   int    `json:"line"`
	Reason string `json:"reason"`
	Raw    string `json:"raw"`
}

// BulkCreateResult is the service-level response shape.
type BulkCreateResult struct {
	Created int             `json:"created"`
	Errors  []BulkLineError `json:"errors"`
}

// BulkCreate parses + validates + inserts bank_soal rows in batch.
//
// Owner = caller (locked #84 per-guru pribadi). All lines that pass
// validation are inserted in a single transaction; lines that fail land
// in errors[] dengan stable reason code. Returns 200 partial-success
// bahkan kalau seluruh batch gagal (created=0, errors populated) — caller
// distinguish via created count.
func (s *Service) BulkCreate(ctx context.Context, callerID uuid.UUID, callerRole string, in BulkCreateInput, ip, userAgent string) (*BulkCreateResult, error) {
	// Hard preconditions FIRST (skill #1).
	if !canWriteBank(callerRole) {
		return nil, ErrForbidden
	}
	rows := strings.TrimSpace(in.Rows)
	if rows == "" {
		return nil, ErrBulkRowsRequired
	}
	mapel := strings.TrimSpace(in.Mapel)
	tingkat := strings.TrimSpace(in.Tingkat)
	topik := strings.TrimSpace(in.Topik)
	if len(mapel) > MaxTagBytes {
		return nil, fmt.Errorf("%w: mapel exceeds %d bytes", ErrInvalidInput, MaxTagBytes)
	}
	if len(tingkat) > MaxTagBytes {
		return nil, fmt.Errorf("%w: tingkat exceeds %d bytes", ErrInvalidInput, MaxTagBytes)
	}
	if len(topik) > MaxTagBytes {
		return nil, fmt.Errorf("%w: topik exceeds %d bytes", ErrInvalidInput, MaxTagBytes)
	}

	// Split + cap line count BEFORE per-line work.
	lines := splitBulkLines(rows)
	if len(lines) > MaxBulkLines {
		return nil, fmt.Errorf("%w: %d lines exceeds %d", ErrBulkTooMany, len(lines), MaxBulkLines)
	}

	result := &BulkCreateResult{
		Created: 0,
		Errors:  []BulkLineError{}, // never nil (skill #4)
	}

	soals := make([]BankSoal, 0, len(lines))
	for _, ln := range lines {
		// Skip blank + comment.
		trimmed := strings.TrimSpace(ln.Raw)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		soal, reason := parseBulkLine(ln.Raw, callerID, mapel, tingkat, topik)
		if reason != "" {
			result.Errors = append(result.Errors, BulkLineError{
				Line:   ln.Number,
				Reason: reason,
				Raw:    ln.Raw,
			})
			continue
		}
		soals = append(soals, soal)
	}

	// Insert valid soals dalam tx (best-effort batch).
	if len(soals) > 0 {
		n, ierr := s.repo.BulkCreateSoal(ctx, soals)
		if ierr != nil {
			return nil, fmt.Errorf("banksoal bulk insert: %w", ierr)
		}
		result.Created = n
	}

	s.logAudit(ctx, "banksoal_bulk_created", &callerID, callerRole, nil, nil, ip, userAgent, map[string]any{
		"created":     result.Created,
		"error_count": len(result.Errors),
		"source":      "paste",
		"mapel":       mapel,
		"tingkat":     tingkat,
		"topik":       topik,
	})
	return result, nil
}

// ---------- parser ----------

// bulkLine is one numbered line from the input body.
type bulkLine struct {
	Number int
	Raw    string
}

// splitBulkLines splits the body at \n and assigns 1-based line numbers.
// CRLF/CR handled — trim \r per line.
func splitBulkLines(body string) []bulkLine {
	if body == "" {
		return nil
	}
	parts := strings.Split(body, "\n")
	out := make([]bulkLine, 0, len(parts))
	for i, p := range parts {
		p = strings.TrimRight(p, "\r")
		out = append(out, bulkLine{Number: i + 1, Raw: p})
	}
	return out
}

// parseBulkLine returns either a populated BankSoal + empty reason (ok)
// or a zero BankSoal + non-empty reason code (failed).
//
// Caller is responsible for skipping blank/comment lines BEFORE calling.
func parseBulkLine(raw string, callerID uuid.UUID, mapel, tingkat, topik string) (BankSoal, string) {
	cols := splitEscapedPipe(raw)
	if len(cols) != 8 {
		return BankSoal{}, ReasonInvalidColumns
	}
	for i := range cols {
		cols[i] = strings.TrimSpace(cols[i])
	}

	pertanyaan := cols[0]
	opsiA := cols[1]
	opsiB := cols[2]
	opsiC := cols[3]
	opsiD := cols[4]
	opsiE := cols[5]
	jawabanRaw := strings.ToLower(cols[6])
	poinRaw := cols[7]

	if pertanyaan == "" {
		return BankSoal{}, ReasonEmptyPertanyaan
	}
	if len(pertanyaan) > MaxPertanyaanBytes {
		return BankSoal{}, ReasonPertanyaanTooLong
	}
	for _, v := range []string{opsiA, opsiB, opsiC, opsiD, opsiE} {
		if len(v) > MaxOpsiBytes {
			return BankSoal{}, ReasonOpsiTooLong
		}
	}

	jawaban := Jawaban(jawabanRaw)
	if !jawaban.Valid() {
		return BankSoal{}, ReasonInvalidJawaban
	}

	// Resolve poin: kosong → 1, else parse + bounds.
	poin := int16(1)
	if poinRaw != "" {
		v, perr := strconv.Atoi(poinRaw)
		if perr != nil || v < 1 || v > 100 {
			return BankSoal{}, ReasonInvalidPoin
		}
		poin = int16(v)
	}

	// Verify selected option has text (gambar tidak ada di bulk paste).
	var answerOpt string
	switch jawaban {
	case JawabanA:
		answerOpt = opsiA
	case JawabanB:
		answerOpt = opsiB
	case JawabanC:
		answerOpt = opsiC
	case JawabanD:
		answerOpt = opsiD
	case JawabanE:
		answerOpt = opsiE
	}
	if strings.TrimSpace(answerOpt) == "" {
		return BankSoal{}, ReasonEmptyJawabanOption
	}

	return BankSoal{
		OwnerGuruID: callerID,
		Mapel:       mapel,
		Tingkat:     tingkat,
		Topik:       topik,
		Pertanyaan:  pertanyaan,
		OpsiA:       opsiA,
		OpsiB:       opsiB,
		OpsiC:       opsiC,
		OpsiD:       opsiD,
		OpsiE:       opsiE,
		Jawaban:     jawaban,
		Poin:        poin,
		Version:     1,
	}, ""
}

// splitEscapedPipe splits s by `|` honoring `\|` as a literal pipe and
// `\\` as a literal backslash. Single-pass scanner (mirror SoalBab).
func splitEscapedPipe(s string) []string {
	var out []string
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '\\' && i+1 < len(s) {
			next := s[i+1]
			switch next {
			case '|':
				b.WriteByte('|')
				i++
				continue
			case '\\':
				b.WriteByte('\\')
				i++
				continue
			}
			// Other backslash escapes pass through as-is.
			b.WriteByte(c)
			continue
		}
		if c == '|' {
			out = append(out, b.String())
			b.Reset()
			continue
		}
		b.WriteByte(c)
	}
	out = append(out, b.String())
	return out
}
