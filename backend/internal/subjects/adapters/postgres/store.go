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
	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct{ queries *sqlcgen.Queries }

func NewStore(pool *pgxpool.Pool) *Store { return &Store{queries: sqlcgen.New(pool)} }

func (s *Store) Create(ctx context.Context, subject domain.Subject) (domain.Subject, error) {
	row, err := s.queries.CreateSubject(ctx, sqlcgen.CreateSubjectParams{
		Code: subject.Code, Species: string(subject.Species), Sex: string(subject.Sex),
	})
	if err != nil {
		return domain.Subject{}, translate(err, "create subject")
	}
	return toSubject(sqlcgen.ListSubjectsRow{
		ID: row.ID, Code: row.Code, Species: row.Species, Sex: row.Sex, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
	}), nil
}

func (s *Store) Subject(ctx context.Context, id string) (domain.Subject, error) {
	if !pgtx.ValidUUID(id) {
		return domain.Subject{}, application.ErrNotFound
	}
	row, err := s.queries.GetSubject(ctx, id)
	if err != nil {
		return domain.Subject{}, translate(err, "get subject")
	}
	return toSubject(sqlcgen.ListSubjectsRow(row)), nil
}

func (s *Store) List(ctx context.Context, f application.Filter) ([]domain.Subject, int, error) {
	search := pgtx.EscapeLike(f.Search)
	rows, err := s.queries.ListSubjects(ctx, sqlcgen.ListSubjectsParams{
		Search: search, Species: string(f.Species), Sex: string(f.Sex), Unallocated: f.Unallocated, SortKey: f.Sort, Descending: f.Descending,
		PageLimit: int32(f.Limit), PageOffset: int32(f.Offset),
	})
	if err != nil {
		return nil, 0, fmt.Errorf("list subjects: %w", err)
	}
	total, err := s.queries.CountSubjects(ctx, sqlcgen.CountSubjectsParams{
		Search: search, Species: string(f.Species), Sex: string(f.Sex), Unallocated: f.Unallocated,
	})
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
	if _, err := s.queries.UpdateSubject(ctx, sqlcgen.UpdateSubjectParams{
		ID: id, Code: c.Code, Species: text(c.Species), Sex: text(c.Sex),
	}); err != nil {
		return domain.Subject{}, translate(err, "update subject")
	}
	return s.Subject(ctx, id)
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

func (s *Store) CreateWeight(ctx context.Context, w domain.Weight) (domain.Weight, error) {
	if !pgtx.ValidUUID(w.SubjectID) {
		return domain.Weight{}, application.ErrNotFound
	}
	row, err := s.queries.CreateWeight(ctx, sqlcgen.CreateWeightParams{
		SubjectID: w.SubjectID, BodyMilligrams: w.Milligrams, MeasuredAt: w.MeasuredAt, RecordedBy: w.RecordedBy,
	})
	if _, constraint := pgtx.Violation(err); constraint == "weight_measurements_subject_id_fkey" {
		return domain.Weight{}, application.ErrNotFound
	}
	if err != nil {
		return domain.Weight{}, translate(err, "create weight")
	}
	return toWeight(row), nil
}

func (s *Store) Weights(ctx context.Context, subjectID string) ([]domain.Weight, error) {
	if !pgtx.ValidUUID(subjectID) {
		return []domain.Weight{}, nil
	}
	rows, err := s.queries.ListWeights(ctx, subjectID)
	if err != nil {
		return nil, fmt.Errorf("list weights: %w", err)
	}
	weights := make([]domain.Weight, 0, len(rows))
	for _, row := range rows {
		weights = append(weights, toWeight(row))
	}
	return weights, nil
}

// translate maps a missing row to ErrNotFound and a subject still referenced
// by an allocation or a weight to ErrInUse.
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

func toSubject(row sqlcgen.ListSubjectsRow) domain.Subject {
	s := domain.Subject{
		ID: row.ID, Code: row.Code, Species: domain.Species(row.Species), Sex: domain.Sex(row.Sex),
		CreatedAt: row.CreatedAt.UTC(), UpdatedAt: row.UpdatedAt.UTC(),
	}
	if row.AllocationExperimentID != nil {
		s.Allocation = &domain.Allocation{
			ExperimentID: *row.AllocationExperimentID, ExperimentCode: value(row.AllocationExperimentCode),
			GroupID: value(row.AllocationGroupID), GroupCode: value(row.AllocationGroupCode), GroupRole: value(row.AllocationGroupRole),
			Excluded: row.AllocationExcluded,
		}
	}
	return s
}

func toWeight(row sqlcgen.MiskoWeightMeasurement) domain.Weight {
	return domain.Weight{
		ID: row.ID, SubjectID: row.SubjectID, Milligrams: row.BodyMilligrams, MeasuredAt: row.MeasuredAt.UTC(),
		RecordedBy: row.RecordedBy, CreatedAt: row.CreatedAt.UTC(),
	}
}

func value(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func text[T ~string](value *T) *string {
	if value == nil {
		return nil
	}
	s := string(*value)
	return &s
}
