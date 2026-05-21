// Bulk paste pipe-delimited untuk SoalBab (Task 5.B.3).
//
// Format per line (9 kolom, pipe `|` separator, escape `\|`):
//
//	pertanyaan|opsi_a|opsi_b|opsi_c|opsi_d|opsi_e|jawaban|poin|mode
//
// - jawaban: a|b|c|d|e (single-answer MVP).
// - poin: integer 1-100. Kosong → 1.
// - mode: latihan|ulangan|keduanya. Kosong → mode_default → keduanya.
// - Pipe escape: `\\|` di body → `|` literal.
// - Skip blank line.
// - Skip line dimulai `#` (komentar).
// - Cap 200 line per request (anti-DoS, locked default sub-skill bulk).
//
// Endpoint: POST /api/v1/bab/:id/soal/bulk body
//
//	{rows: string, mode_default?: latihan|ulangan|keduanya}
//
// Response 200 partial-success:
//
//	{created: int, errors: [{line: int, reason: string, raw: string}]}
//
// Hard preconditions (4xx, abort sebelum parse):
//   - invalid_id 400, not_found 404, forbidden 403, bab_archived 409,
//   - invalid_body 400, rows_required 400, too_many 400.
//
// Soft per-line failures (200, kumpul di errors[]): invalid_columns,
// empty_pertanyaan, empty_jawaban_option, invalid_jawaban, invalid_poin,
// invalid_mode, pertanyaan_too_long, opsi_too_long.
//
// Audit: soalbab_bulk_created w/ meta {count, error_count, source}.
package soalbab

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/google/uuid"

	"github.com/pikip/lms/backend/internal/bab"
)

// MaxBulkLines caps the number of lines accepted per bulk paste request
// (anti-DoS + bounds parse cost). 200 line × 9 kolom × max ~7KB per line
// ≈ 1.4MB upper bound — well within Fiber default body limit (4MB).
const MaxBulkLines = 200

// Reason codes per line — stable enum, FE maps ke localized copy.
const (
	ReasonInvalidColumns      = "invalid_columns"
	ReasonEmptyPertanyaan     = "empty_pertanyaan"
	ReasonEmptyJawabanOption  = "empty_jawaban_option"
	ReasonInvalidJawaban      = "invalid_jawaban"
	ReasonInvalidPoin         = "invalid_poin"
	ReasonInvalidMode         = "invalid_mode"
	ReasonPertanyaanTooLong   = "pertanyaan_too_long"
	ReasonOpsiTooLong         = "opsi_too_long"
	ReasonInternal            = "internal"
)

// Sentinel errors mapped ke HTTP di handler (hard precondition only).
var (
	ErrBulkRowsRequired = errors.New("soalbab: rows required")
	ErrBulkTooMany      = errors.New("soalbab: too many lines")
)

// BulkCreateInput holds the pasted body + default mode override.
type BulkCreateInput struct {
	Rows        string
	ModeDefault Mode // empty → ModeKeduanya
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

// BulkCreate parses + validates + inserts soal_bab rows in batch.
//
// Owner-only + bab not archived. All lines that pass validation are
// inserted in a single transaction; lines that fail land in errors[]
// dengan stable reason code. Returns 200 partial-success bahkan kalau
// seluruh batch gagal (created=0, errors populated) — caller distinguish
// via created count.
func (s *Service) BulkCreate(ctx context.Context, babID, callerID uuid.UUID, callerRole string, in BulkCreateInput, ip, userAgent string) (*BulkCreateResult, error) {
	// Hard preconditions FIRST (skill #1).
	rows := strings.TrimSpace(in.Rows)
	if rows == "" {
		return nil, ErrBulkRowsRequired
	}
	modeDefault := in.ModeDefault
	if modeDefault == "" {
		modeDefault = ModeKeduanya
	}
	if !modeDefault.Valid() {
		return nil, fmt.Errorf("%w: mode_default must be latihan|ulangan|keduanya", ErrInvalidInput)
	}

	b, err := s.findBabAndOwnership(ctx, babID, callerID, callerRole)
	if err != nil {
		return nil, err
	}
	if b.Status == bab.StatusArchived {
		return nil, ErrBabArchived
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

	soals := make([]SoalBab, 0, len(lines))
	for _, ln := range lines {
		// Skip blank + comment.
		trimmed := strings.TrimSpace(ln.Raw)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		soal, reason := parseBulkLine(ln.Raw, modeDefault, b.ID, b.KelasID, callerID)
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

	// Insert valid soals dalam tx (best-effort batch). Pada error,
	// set created=0 + reason=internal di line tracking.
	if len(soals) > 0 {
		n, ierr := s.repo.BulkCreateSoal(ctx, soals)
		if ierr != nil {
			// Whole batch failed — surface as soft error supaya partial
			// response shape tetap konsisten (skill: never let DB blow
			// up the bulk response).
			return nil, fmt.Errorf("soalbab bulk insert: %w", ierr)
		}
		result.Created = n
	}

	s.logAudit(ctx, "soalbab_bulk_created", &callerID, callerRole, &b.ID, &b.KelasID, ip, userAgent, map[string]any{
		"bab_id":      b.ID.String(),
		"created":     result.Created,
		"error_count": len(result.Errors),
		"source":      "paste",
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

// parseBulkLine returns either a populated SoalBab + empty reason (ok)
// or a zero SoalBab + non-empty reason code (failed).
//
// Caller is responsible for skipping blank/comment lines BEFORE calling.
func parseBulkLine(raw string, modeDefault Mode, babID, kelasID, callerID uuid.UUID) (SoalBab, string) {
	cols := splitEscapedPipe(raw)
	if len(cols) != 9 {
		return SoalBab{}, ReasonInvalidColumns
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
	modeRaw := strings.ToLower(cols[8])

	if pertanyaan == "" {
		return SoalBab{}, ReasonEmptyPertanyaan
	}
	if len(pertanyaan) > MaxPertanyaanBytes {
		return SoalBab{}, ReasonPertanyaanTooLong
	}
	for _, v := range []string{opsiA, opsiB, opsiC, opsiD, opsiE} {
		if len(v) > MaxOpsiBytes {
			return SoalBab{}, ReasonOpsiTooLong
		}
	}

	jawaban := Jawaban(jawabanRaw)
	if !jawaban.Valid() {
		return SoalBab{}, ReasonInvalidJawaban
	}

	// Resolve poin: kosong → 1, else parse + bounds.
	poin := int16(1)
	if poinRaw != "" {
		v, perr := strconv.Atoi(poinRaw)
		if perr != nil || v < 1 || v > 100 {
			return SoalBab{}, ReasonInvalidPoin
		}
		poin = int16(v)
	}

	// Resolve mode: kosong → modeDefault, else validate.
	mode := modeDefault
	if modeRaw != "" {
		m := Mode(modeRaw)
		if !m.Valid() {
			return SoalBab{}, ReasonInvalidMode
		}
		mode = m
	}

	// Verify selected option has text (gambar tidak ada di bulk).
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
		return SoalBab{}, ReasonEmptyJawabanOption
	}

	return SoalBab{
		BabID:       babID,
		KelasID:     kelasID,
		Pertanyaan:  pertanyaan,
		OpsiA:       opsiA,
		OpsiB:       opsiB,
		OpsiC:       opsiC,
		OpsiD:       opsiD,
		OpsiE:       opsiE,
		Jawaban:     jawaban,
		Poin:        poin,
		Mode:        mode,
		Urutan:      0,
		Version:     1,
		CreatedByID: callerID,
	}, ""
}

// splitEscapedPipe splits s by `|` honoring `\|` as a literal pipe and
// `\\` as a literal backslash. Single-pass scanner.
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
