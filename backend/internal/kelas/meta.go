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
