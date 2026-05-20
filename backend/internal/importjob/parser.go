// CSV parser for bulk-import jobs (Task 2.D.1).
//
// Lifecycle: admin uploads CSV → backend calls Parse(reader) → returns
// ParseResult { Rows, Errors, Stats } which is later persisted as
// PreviewRowsJSON + ErrorsJSON on the ImportJob row (Task 2.D.2).
//
// Required columns (header row, case-insensitive):
//
//	nama|name           — display name (1-100 chars)
//	email               — must parse via net/mail (RFC 5322 subset)
//
// Optional columns:
//
//	kode_kelas|kode|kode_invite — auto-enroll to a kelas after creation
//
// Locked decisions:
//   - #54 ImportJob lifecycle (preview→processing→completed)
//   - Section 5.13 admin import flow (kolom nama, email + optional kode)
//   - Password is NOT in the CSV — generated server-side at confirm time.
package importjob

import (
	"bufio"
	"bytes"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"net/mail"
	"strings"
	"unicode/utf8"
)

// Parser limits (locked at MVP — tunable via config later if needed).
const (
	// MaxCSVBytes is the hard cap on raw CSV upload size. Mirrors the 5MB
	// limit referenced in Task 2.D.2 (file upload validation).
	MaxCSVBytes = 5 * 1024 * 1024

	// MaxCSVRows is the hard cap on data rows (excludes header). 5000 is
	// enough for a full school-year intake without abusing memory.
	MaxCSVRows = 5000

	// MaxNamaLen / MaxEmailLen / MaxKodeKelasLen are per-field caps — these
	// match the DB column reality (User.Name unbounded but rendered in UI;
	// citext email; Kelas.KodeInvite ~16 chars).
	MaxNamaLen      = 100
	MaxEmailLen     = 254 // RFC 5321 SMTP path limit
	MaxKodeKelasLen = 32
)

// Row is a single data row after parsing + normalization. Status indicates
// whether the row is includable in the import (Valid), rejected because of
// validation errors (Invalid), or rejected because an earlier row had the
// same email (Duplicate).
type Row struct {
	// LineNo is the 1-based source line number including the header
	// (i.e., the first data row is LineNo=2). Used in UI error messages.
	LineNo int `json:"line_no"`

	Nama      string `json:"nama"`
	Email     string `json:"email"`      // lowercased, trimmed
	KodeKelas string `json:"kode_kelas"` // uppercased, trimmed, may be ""

	Status RowStatus `json:"status"`
	// Errors is the per-row validation message list. Empty for Valid rows.
	Errors []string `json:"errors,omitempty"`
}

// RowStatus enumerates the parse outcome per row.
type RowStatus string

const (
	RowValid     RowStatus = "valid"
	RowInvalid   RowStatus = "invalid"
	RowDuplicate RowStatus = "duplicate"
)

// ParseResult is the full output. Stats counts mirror ImportJob columns.
type ParseResult struct {
	Rows  []Row     `json:"rows"`
	Stats ParseStat `json:"stats"`
}

// ParseStat aggregates row counts. Total = Valid + Invalid + Duplicate.
type ParseStat struct {
	Total      int `json:"total"`
	Valid      int `json:"valid"`
	Invalid    int `json:"invalid"`
	Duplicates int `json:"duplicates"`
}

// Recognised header aliases. Lowercased + trimmed.
var (
	aliasNama      = []string{"nama", "name", "nama_lengkap", "full_name", "fullname"}
	aliasEmail     = []string{"email", "e-mail", "alamat_email"}
	aliasKodeKelas = []string{"kode_kelas", "kode_invite", "kode", "invite_code"}
)

// Sentinel errors. Callers should errors.Is() these so the API can return
// stable codes (e.g. "csv_too_large", "csv_no_rows").
var (
	ErrCSVTooLarge        = errors.New("csv: payload exceeds maximum size")
	ErrTooManyRows        = errors.New("csv: too many rows")
	ErrEmptyCSV           = errors.New("csv: empty input")
	ErrMalformedHeader    = errors.New("csv: header row malformed")
	ErrMissingNamaColumn  = errors.New("csv: required column 'nama' (or 'name') not found")
	ErrMissingEmailColumn = errors.New("csv: required column 'email' not found")
	ErrInvalidUTF8        = errors.New("csv: invalid UTF-8 (re-save as UTF-8 from your spreadsheet)")
)

// Parse reads the CSV from r, validates, and returns a ParseResult.
//
// Limits enforced:
//   - r is read up to MaxCSVBytes+1 to detect oversized payloads
//   - at most MaxCSVRows data rows are accepted
//   - rejects non-UTF-8 input (Excel users: re-save as "CSV UTF-8")
//   - tolerates UTF-8 BOM at start of file
//   - allows ',' or ';' as delimiter (auto-detected from header line)
//
// Hard errors (returned as the second value) are returned for I/O,
// header validation, and limits — the caller surfaces these as
// 400 Bad Request. Per-row validation errors are reflected in the
// Row.Errors / Row.Status fields and are NOT returned as Go errors.
func Parse(r io.Reader) (ParseResult, error) {
	limited := &io.LimitedReader{R: r, N: int64(MaxCSVBytes) + 1}
	raw, err := io.ReadAll(limited)
	if err != nil {
		return ParseResult{}, fmt.Errorf("csv: read: %w", err)
	}
	if int64(len(raw)) > MaxCSVBytes {
		return ParseResult{}, ErrCSVTooLarge
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		return ParseResult{}, ErrEmptyCSV
	}

	// Strip optional UTF-8 BOM (Excel writes one for "CSV UTF-8" exports).
	raw = bytes.TrimPrefix(raw, []byte{0xEF, 0xBB, 0xBF})

	if !utf8.Valid(raw) {
		return ParseResult{}, ErrInvalidUTF8
	}

	// Auto-detect delimiter from the first non-empty line. Accept either
	// ',' or ';' (common Excel locale default in Indonesia).
	delim := detectDelimiter(raw)

	rd := csv.NewReader(bufio.NewReader(bytes.NewReader(raw)))
	rd.Comma = delim
	rd.FieldsPerRecord = -1 // tolerate ragged rows; we validate field count manually
	rd.LazyQuotes = true
	rd.TrimLeadingSpace = true

	header, err := rd.Read()
	if err == io.EOF {
		return ParseResult{}, ErrEmptyCSV
	}
	if err != nil {
		return ParseResult{}, fmt.Errorf("%w: %v", ErrMalformedHeader, err)
	}

	idx, err := mapHeaderColumns(header)
	if err != nil {
		return ParseResult{}, err
	}

	var (
		out  ParseResult
		seen = make(map[string]int, 64) // lowercased email -> first LineNo seen
	)

	lineNo := 1 // header
	for {
		lineNo++
		fields, err := rd.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			// Malformed row: record as invalid, keep going.
			out.Rows = append(out.Rows, Row{
				LineNo: lineNo,
				Status: RowInvalid,
				Errors: []string{"baris malformed: " + err.Error()},
			})
			out.Stats.Invalid++
			out.Stats.Total++
			if out.Stats.Total > MaxCSVRows {
				return ParseResult{}, ErrTooManyRows
			}
			continue
		}

		if isBlankRow(fields) {
			// Tolerate trailing blank rows silently — they don't count.
			continue
		}

		row := Row{LineNo: lineNo}
		row.Nama = pickField(fields, idx.nama)
		row.Email = strings.ToLower(pickField(fields, idx.email))
		row.KodeKelas = strings.ToUpper(pickField(fields, idx.kode))

		validateRow(&row)

		// Dedup: only mark Duplicate if this email already appeared as a
		// VALID row earlier. An earlier invalid row doesn't claim the email.
		if row.Status == RowValid {
			if first, dup := seen[row.Email]; dup {
				row.Status = RowDuplicate
				row.Errors = []string{
					fmt.Sprintf("email duplikat dengan baris %d", first),
				}
				out.Stats.Duplicates++
			} else {
				seen[row.Email] = row.LineNo
				out.Stats.Valid++
			}
		} else {
			out.Stats.Invalid++
		}

		out.Rows = append(out.Rows, row)
		out.Stats.Total++
		if out.Stats.Total > MaxCSVRows {
			return ParseResult{}, ErrTooManyRows
		}
	}

	if out.Stats.Total == 0 {
		return ParseResult{}, ErrEmptyCSV
	}
	return out, nil
}

// --- helpers ---

type columnIdx struct {
	nama  int
	email int
	kode  int // -1 if optional column absent
}

func mapHeaderColumns(header []string) (columnIdx, error) {
	idx := columnIdx{nama: -1, email: -1, kode: -1}
	for i, raw := range header {
		key := strings.ToLower(strings.TrimSpace(raw))
		switch {
		case contains(aliasNama, key):
			if idx.nama == -1 {
				idx.nama = i
			}
		case contains(aliasEmail, key):
			if idx.email == -1 {
				idx.email = i
			}
		case contains(aliasKodeKelas, key):
			if idx.kode == -1 {
				idx.kode = i
			}
		}
	}
	if idx.nama == -1 {
		return idx, ErrMissingNamaColumn
	}
	if idx.email == -1 {
		return idx, ErrMissingEmailColumn
	}
	return idx, nil
}

func contains(haystack []string, needle string) bool {
	for _, h := range haystack {
		if h == needle {
			return true
		}
	}
	return false
}

func pickField(fields []string, i int) string {
	if i < 0 || i >= len(fields) {
		return ""
	}
	return strings.TrimSpace(fields[i])
}

func isBlankRow(fields []string) bool {
	for _, f := range fields {
		if strings.TrimSpace(f) != "" {
			return false
		}
	}
	return true
}

func validateRow(row *Row) {
	row.Status = RowValid
	row.Errors = nil

	if row.Nama == "" {
		row.Errors = append(row.Errors, "nama wajib diisi")
	} else if utf8.RuneCountInString(row.Nama) > MaxNamaLen {
		row.Errors = append(row.Errors, fmt.Sprintf("nama maksimal %d karakter", MaxNamaLen))
	}

	if row.Email == "" {
		row.Errors = append(row.Errors, "email wajib diisi")
	} else if len(row.Email) > MaxEmailLen {
		row.Errors = append(row.Errors, fmt.Sprintf("email maksimal %d karakter", MaxEmailLen))
	} else if _, err := mail.ParseAddress(row.Email); err != nil {
		row.Errors = append(row.Errors, "format email tidak valid")
	}

	if row.KodeKelas != "" && len(row.KodeKelas) > MaxKodeKelasLen {
		row.Errors = append(row.Errors, fmt.Sprintf("kode kelas maksimal %d karakter", MaxKodeKelasLen))
	}

	if len(row.Errors) > 0 {
		row.Status = RowInvalid
	}
}

// detectDelimiter peeks the first non-empty line and returns ',' or ';'.
// Defaults to ',' if neither is present.
func detectDelimiter(raw []byte) rune {
	end := bytes.IndexByte(raw, '\n')
	if end == -1 {
		end = len(raw)
	}
	first := raw[:end]
	commas := bytes.Count(first, []byte(","))
	semis := bytes.Count(first, []byte(";"))
	if semis > commas {
		return ';'
	}
	return ','
}
