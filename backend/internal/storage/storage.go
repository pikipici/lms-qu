// Package storage provides a small filesystem helper used by upload handlers
// (Fase 4+) and readyz probe. Path convention is locked decision #58:
//
//	./storage/uploads/<kategori>/<uuid>.<ext>
//
// Original filename is stored in DB; on disk we only persist the UUID-named
// file so we can stat / cleanup orphans cheaply.
package storage

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// Category names — keep in sync with locked decision #58.
const (
	CategoryTugas      = "tugas"
	CategorySoal       = "soal"
	CategoryMateri     = "materi"
	CategorySubmission = "submission"
	CategoryImport     = "import"
)

var validCategories = map[string]struct{}{
	CategoryTugas:      {},
	CategorySoal:       {},
	CategoryMateri:     {},
	CategorySubmission: {},
	CategoryImport:     {},
}

// Init ensures the upload root and per-category subdirectories exist.
// Called once on server startup.
func Init(root string) error {
	if root == "" {
		return errors.New("storage: root is empty")
	}
	for cat := range validCategories {
		if err := os.MkdirAll(filepath.Join(root, cat), 0o750); err != nil {
			return fmt.Errorf("storage: mkdir %s: %w", cat, err)
		}
	}
	return nil
}

// Path returns the absolute (or root-relative) destination for a given
// category + relative filename. Use this so all upload paths share the same
// convention. The caller must validate `category` is known.
func Path(root, category, name string) (string, error) {
	if _, ok := validCategories[category]; !ok {
		return "", fmt.Errorf("storage: unknown category %q", category)
	}
	if name == "" {
		return "", errors.New("storage: empty filename")
	}
	return filepath.Join(root, category, name), nil
}
