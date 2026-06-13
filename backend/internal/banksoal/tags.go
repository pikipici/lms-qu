package banksoal

import (
	"database/sql/driver"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

const (
	MaxTagsPerSoal = 20
	MaxTagLength   = 40
)

var tagAllowedPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)

// Tags stores normalized Bank Soal free-form tags in a PostgreSQL text[].
type Tags []string

// Value implements driver.Valuer for PostgreSQL text[] literals.
func (t Tags) Value() (driver.Value, error) {
	if len(t) == 0 {
		return "{}", nil
	}
	parts := make([]string, 0, len(t))
	for _, tag := range t {
		parts = append(parts, `"`+strings.ReplaceAll(tag, `"`, `\"`)+`"`)
	}
	return "{" + strings.Join(parts, ",") + "}", nil
}

// Scan implements sql.Scanner for PostgreSQL text[]. It supports the simple
// array format produced by our normalized tags (no commas/quotes in values).
func (t *Tags) Scan(value any) error {
	if value == nil {
		*t = Tags{}
		return nil
	}
	var raw string
	switch v := value.(type) {
	case string:
		raw = v
	case []byte:
		raw = string(v)
	default:
		return fmt.Errorf("banksoal tags: unsupported scan type %T", value)
	}
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "{}" {
		*t = Tags{}
		return nil
	}
	raw = strings.TrimPrefix(strings.TrimSuffix(raw, "}"), "{")
	if raw == "" {
		*t = Tags{}
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.Trim(part, `"`)
		part = strings.ReplaceAll(part, `\"`, `"`)
		if part != "" {
			out = append(out, part)
		}
	}
	*t = Tags(out)
	return nil
}

func NormalizeTags(raw []string) Tags {
	tags, _ := normalizeTags(raw)
	return tags
}

func normalizeTags(raw []string) (Tags, error) {
	seen := make(map[string]struct{}, len(raw))
	out := make([]string, 0, len(raw))
	for _, raw := range raw {
		tag := normalizeTag(raw)
		if tag == "" {
			continue
		}
		if len(tag) > MaxTagLength {
			return nil, fmt.Errorf("%w: tag_too_long", ErrInvalidInput)
		}
		if !tagAllowedPattern.MatchString(tag) {
			return nil, fmt.Errorf("%w: invalid_tag", ErrInvalidInput)
		}
		if _, ok := seen[tag]; ok {
			continue
		}
		seen[tag] = struct{}{}
		out = append(out, tag)
	}
	if len(out) > MaxTagsPerSoal {
		return nil, fmt.Errorf("%w: too_many_tags", ErrInvalidInput)
	}
	sort.Strings(out)
	return Tags(out), nil
}

func normalizeTag(raw string) string {
	tag := strings.ToLower(strings.TrimSpace(raw))
	tag = strings.Join(strings.Fields(tag), "-")
	tag = strings.Trim(tag, "-_")
	return tag
}

func tagsEqual(a, b Tags) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func tagsArrayLiteral(tags Tags) string {
	v, _ := tags.Value()
	return v.(string)
}
