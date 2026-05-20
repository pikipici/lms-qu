package kelas

import (
	"encoding/json"

	"gorm.io/datatypes"
)

// ptrString returns a pointer to s, or nil if s is empty. Mirrors strPtr in
// internal/admin/handler.go so audit log fields stay sparse.
func ptrString(s string) *string {
	if s == "" {
		return nil
	}
	v := s
	return &v
}

// strRefPtr returns a pointer to s unconditionally. Useful for partial-update
// payloads where empty string ("") is a meaningful "set to empty" signal.
func strRefPtr(s string) *string {
	v := s
	return &v
}

// intPtr returns a pointer to i. Useful for partial-update payloads.
func intPtr(i int) *int {
	v := i
	return &v
}

// marshalMeta serializes a map to a JSONB blob. Returns nil when the map is
// empty or marshalling fails — audit rows with no meta are valid.
func marshalMeta(fields map[string]any) datatypes.JSON {
	if len(fields) == 0 {
		return nil
	}
	b, err := json.Marshal(fields)
	if err != nil {
		return nil
	}
	return datatypes.JSON(b)
}
