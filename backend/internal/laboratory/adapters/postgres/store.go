// Package postgres implements laboratory persistence with sqlc-generated queries.
package postgres

import (
	"context"
	"errors"
	"fmt"
	"github.com/HappyHackingSpace/Misko/backend/internal/laboratory/adapters/postgres/sqlcgen"
	"github.com/HappyHackingSpace/Misko/backend/internal/laboratory/application"
	"github.com/HappyHackingSpace/Misko/backend/internal/laboratory/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"time"
)

type Store struct{ queries *sqlcgen.Queries }

func NewStore(pool *pgxpool.Pool) *Store { return &Store{queries: sqlcgen.New(pool)} }

func (s *Store) Laboratory(ctx context.Context) (domain.Laboratory, error) {
	row, err := s.queries.GetLaboratory(ctx)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Laboratory{}, application.ErrNotInitialized
	}
	if err != nil {
		return domain.Laboratory{}, fmt.Errorf("get laboratory: %w", err)
	}
	return toLaboratory(row.Name, row.Code, row.Timezone, row.CreatedAt, row.UpdatedAt), nil
}

// Create relies on the singleton primary key: concurrent inserts wait for the
// winner and then insert nothing.
func (s *Store) Create(ctx context.Context, lab domain.Laboratory) (bool, error) {
	inserted, err := s.queries.CreateLaboratory(ctx, sqlcgen.CreateLaboratoryParams{Name: lab.Name, Code: optional(lab.Code), Timezone: lab.Timezone})
	if err != nil {
		return false, fmt.Errorf("create laboratory: %w", err)
	}
	return inserted == 1, nil
}

func (s *Store) Update(ctx context.Context, c application.Change) (domain.Laboratory, error) {
	params := sqlcgen.UpdateLaboratoryParams{Name: c.Name, Timezone: c.Timezone, SetCode: c.Code != nil}
	if c.Code != nil {
		params.Code = optional(*c.Code)
	}
	row, err := s.queries.UpdateLaboratory(ctx, params)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Laboratory{}, application.ErrNotInitialized
	}
	if err != nil {
		return domain.Laboratory{}, fmt.Errorf("update laboratory: %w", err)
	}
	return toLaboratory(row.Name, row.Code, row.Timezone, row.CreatedAt, row.UpdatedAt), nil
}

func toLaboratory(name string, code *string, timezone string, createdAt, updatedAt time.Time) domain.Laboratory {
	lab := domain.Laboratory{Name: name, Timezone: timezone, CreatedAt: createdAt.UTC(), UpdatedAt: updatedAt.UTC()}
	if code != nil {
		lab.Code = *code
	}
	return lab
}

// optional stores an unset code as NULL.
func optional(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}
