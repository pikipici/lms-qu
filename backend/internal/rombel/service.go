package rombel

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

var (
	ErrInvalidInput    = errors.New("rombel: invalid input")
	ErrAlreadyArchived = errors.New("rombel: already archived")
)

type Service struct{ repo *Repo }

func NewService(repo *Repo) *Service { return &Service{repo: repo} }

type ListInput struct {
	IncludeArchived bool
	Limit, Offset   int
}
type ListResult struct {
	Items []Rombel
	Total int64
}
type CreateInput struct{ Nama, Deskripsi string }
type UpdateInput struct {
	Version         int
	Nama, Deskripsi string
}

func (s *Service) ListBySekolah(ctx context.Context, sekolahID uuid.UUID, in ListInput) (*ListResult, error) {
	limit, offset := normalize(in.Limit, in.Offset)
	items, total, err := s.repo.ListBySekolah(ctx, sekolahID, in.IncludeArchived, limit, offset)
	if err != nil {
		return nil, err
	}
	return &ListResult{Items: items, Total: total}, nil
}

func (s *Service) ListPublicBySekolah(ctx context.Context, sekolahID uuid.UUID) ([]Rombel, error) {
	return s.repo.ListPublicBySekolah(ctx, sekolahID)
}

func (s *Service) Create(ctx context.Context, sekolahID uuid.UUID, in CreateInput) (*Rombel, error) {
	nama := strings.TrimSpace(in.Nama)
	if nama == "" {
		return nil, fmt.Errorf("%w: nama required", ErrInvalidInput)
	}
	row := &Rombel{SekolahID: sekolahID, Nama: nama, Deskripsi: strings.TrimSpace(in.Deskripsi), Active: true, Version: 1}
	if err := s.repo.Create(ctx, row); err != nil {
		return nil, err
	}
	return row, nil
}

func (s *Service) Update(ctx context.Context, id uuid.UUID, in UpdateInput) (*Rombel, error) {
	if in.Version <= 0 {
		return nil, fmt.Errorf("%w: version required", ErrInvalidInput)
	}
	nama := strings.TrimSpace(in.Nama)
	if nama == "" {
		return nil, fmt.Errorf("%w: nama required", ErrInvalidInput)
	}
	return s.repo.Update(ctx, id, in.Version, nama, strings.TrimSpace(in.Deskripsi))
}

func (s *Service) Archive(ctx context.Context, id uuid.UUID) (*Rombel, error) {
	return s.repo.Archive(ctx, id)
}
func (s *Service) DeleteIfEmpty(ctx context.Context, id uuid.UUID) error {
	return s.repo.DeleteIfEmpty(ctx, id)
}

func normalize(limit, offset int) (int, int) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	return limit, offset
}
