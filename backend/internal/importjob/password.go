// Random password generator for bulk-imported users (Task 2.D.4).
//
// Uses crypto/rand with a hand-picked alphabet:
//   - 26 lowercase + 26 uppercase + 10 digits = 62 chars
//   - Symbols deliberately omitted: avoids URL-encoding/CSV-quoting/email-paste
//     issues, and the generated CSV is going to schools whose admins forward
//     credentials via WhatsApp / printed slips. 12 chars from a 62-char set
//     ≈ 71 bits of entropy, well above the 60-bit floor for short-lived initial
//     passwords (force-change required on first login).
package importjob

import (
	"crypto/rand"
	"fmt"
)

// GeneratedPasswordLength is the fixed length of the random password emitted
// per imported user. 12 chars yields log2(62^12) ≈ 71.5 bits of entropy.
const GeneratedPasswordLength = 12

// pwAlphabet is the URL-safe, paste-friendly character set. Order doesn't
// matter for entropy; we keep it explicit for debuggability.
const pwAlphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"

// GeneratePassword returns a cryptographically random password of length
// GeneratedPasswordLength drawn uniformly from pwAlphabet.
//
// Implementation note: we read len*1 bytes and bit-mask each into [0,64)
// then reject 62/63 to keep the distribution uniform. Avoiding modulo bias
// matters because skew toward early letters is observable across thousands
// of imported users (and a fingerprint for any future audit).
func GeneratePassword() (string, error) {
	out := make([]byte, GeneratedPasswordLength)
	// Over-fetch slightly so a few rejections don't trigger a second read.
	buf := make([]byte, GeneratedPasswordLength*2)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("import: read random: %w", err)
	}
	written := 0
	bi := 0
	for written < GeneratedPasswordLength {
		if bi >= len(buf) {
			// Topped up: extend buffer (rare; loop runs at most a few times).
			extra := make([]byte, GeneratedPasswordLength)
			if _, err := rand.Read(extra); err != nil {
				return "", fmt.Errorf("import: read random topup: %w", err)
			}
			buf = append(buf, extra...)
		}
		b := buf[bi] & 0x3F // 6 bits → [0,64)
		bi++
		if b >= byte(len(pwAlphabet)) {
			continue // reject 62/63 to remove modulo bias
		}
		out[written] = pwAlphabet[b]
		written++
	}
	return string(out), nil
}
