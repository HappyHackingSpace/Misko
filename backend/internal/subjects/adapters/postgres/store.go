// Package postgres implements subject persistence with sqlc-generated queries.
package postgres

import (
	"context"
	"errors"
	"fmt"
	"github.com/HappyHackingSpace/Misko/backend/internal/platform/pgtx"
	"github.com/HappyHackingSpace/Misko/backend/internal/subjects/adapters/postgres/sqlcgen"
	"github.com/HappyHackingSpace/Misko/backend/internal/subjects/application"
	"github.com/HappyHackingSpace/Misko/backend/internal/subjects/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"time"
)

type Store struct{ queries *sqlcgen.Queries }

func NewStore(pool *pgxpool.Pool) *Store { return &Store{queries: sqlcgen.New(pool)} }

func (s *Store) Create(ctx context.Context, subject domain.Subject) (domain.Subject, error) {
	row, err := s.queries.CreateSubject(ctx, sqlcgen.CreateSubjectParams{
		Code: subject.Code, Species: string(subject.Species), Sex: string(subject.Sex),
		Strain: optional(subject.Strain), BirthDate: date(subject.BirthDate), Notes: optional(subject.Notes),
	})
	if err != nil {
		return domain.Subject{}, translate(err, "create subject")
	}
	return toSubject(row), nil
}

func (s *Store) Subject(ctx context.Context, id string) (domain.Subject, error) {
	if !pgtx.ValidUUID(id) {
		return domain.Subject{}, application.ErrNotFound
	}
	row, err := s.queries.GetSubject(ctx, id)
	if err != nil {
		return domain.Subject{}, translate(err, "get subject")
	}
	return toSubject(row), nil
}

func (s *Store) List(ctx context.Context, f application.Filter) ([]domain.Subject, int, error) {
	search := pgtx.EscapeLike(f.Search)
	rows, err := s.queries.ListSubjects(ctx, sqlcgen.ListSubjectsParams{
		Search: search, Species: string(f.Species), Sex: string(f.Sex), SortKey: f.Sort, Descending: f.Descending,
		PageLimit: int32(f.Limit), PageOffset: int32(f.Offset),
	})
	if err != nil {
		return nil, 0, fmt.Errorf("list subjects: %w", err)
	}
	total, err := s.queries.CountSubjects(ctx, sqlcgen.CountSubjectsParams{Search: search, Species: string(f.Species), Sex: string(f.Sex)})
	if err != nil {
		return nil, 0, fmt.Errorf("count subjects: %w", err)
	}
	subjects := make([]domain.Subject, 0, len(rows))
	for _, row := range rows {
		subjects = append(subjects, toSubject(row))
	}
	return subjects, int(total), nil
}

func (s *Store) Update(ctx context.Context, id string, c application.Changes) (domain.Subject, error) {
	if !pgtx.ValidUUID(id) {
		return domain.Subject{}, application.ErrNotFound
	}
	row, err := s.queries.UpdateSubject(ctx, sqlcgen.UpdateSubjectParams{
		ID: id, Code: c.Code, Species: text(c.Species), Sex: text(c.Sex), Strain: c.Strain, Notes: c.Notes,
		BirthDate: date(c.BirthDate), ClearBirthDate: c.ClearBirthDate,
	})
	if err != nil {
		return domain.Subject{}, translate(err, "update subject")
	}
	return toSubject(row), nil
}

func (s *Store) Delete(ctx context.Context, id string) error {
	if !pgtx.ValidUUID(id) {
		return application.ErrNotFound
	}
	deleted, err := s.queries.DeleteSubject(ctx, id)
	if err != nil {
		return translate(err, "delete subject")
	}
	if deleted == 0 {
		return application.ErrNotFound
	}
	return nil
}

func translate(err error, operation string) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return application.ErrNotFound
	}
	switch code, constraint := pgtx.Violation(err); {
	case constraint == "subjects_code_key":
		return application.ErrCodeTaken
	case code == "23503":
		return application.ErrInUse
	}
	return fmt.Errorf("%s: %w", operation, err)
}

func toSubject(row sqlcgen.MiskoSubject) domain.Subject {
	s := domain.Subject{
		ID: row.ID, Code: row.Code, Species: domain.Species(row.Species), Sex: domain.Sex(row.Sex),
		CreatedAt: row.CreatedAt.UTC(), UpdatedAt: row.UpdatedAt.UTC(),
	}
	if row.Strain != nil {
		s.Strain = *row.Strain
	}
	if row.Notes != nil {
		s.Notes = *row.Notes
	}
	if row.BirthDate.Valid {
		birth := time.Date(row.BirthDate.Time.Year(), row.BirthDate.Time.Month(), row.BirthDate.Time.Day(), 0, 0, 0, 0, time.UTC)
		s.BirthDate = &birth
	}
	return s
}

func date(t *time.Time) pgtype.Date {
	if t == nil {
		return pgtype.Date{}
	}
	return pgtype.Date{Time: *t, Valid: true}
}

// optional stores an empty value as NULL.
func optional(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func text[T ~string](value *T) *string {
	if value == nil {
		return nil
	}
	s := string(*value)
	return &s
}
