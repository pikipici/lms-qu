// Helpers shared by handler — RFC3339 timestamp parsing for nullable fields.
package ujian

import (
	"strings"
	"time"
)

// parseOptionalRFC3339 returns nil + nil if input is nil/empty, else
// parses RFC3339 (with optional fractional seconds + timezone offset).
// Empty string treated same as nil — FE may send "" for "leave unset".
func parseOptionalRFC3339(in *string) (*time.Time, error) {
	if in == nil {
		return nil, nil
	}
	s := strings.TrimSpace(*in)
	if s == "" {
		return nil, nil
	}
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		// Fallback ke RFC3339 (no fractional seconds).
		t2, err2 := time.Parse(time.RFC3339, s)
		if err2 != nil {
			return nil, err
		}
		t = t2
	}
	return &t, nil
}
