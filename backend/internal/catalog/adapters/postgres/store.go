// Package postgres implements catalog persistence with sqlc-generated queries.
package postgres

import (
	"context"
	"errors"
	"fmt"
	"github.com/HappyHackingSpace/Misko/backend/internal/catalog/adapters/postgres/sqlcgen"
	"github.com/HappyHackingSpace/Misko/backend/internal/catalog/application"
	"github.com/HappyHackingSpace/Misko/backend/internal/catalog/domain"
	"github.com/HappyHackingSpace/Misko/backend/internal/platform/pgtx"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// foreignKeyViolation is the SQLSTATE of a row still referenced elsewhere.
const foreignKeyViolation = "23503"

// constraintErrors maps named constraints to the error a violation means.
var constraintErrors = map[string]error{
	"substances_name_key":     application.ErrNameTaken,
	"disease_models_name_key": application.ErrNameTaken,
}

type Store struct{ queries *sqlcgen.Queries }

func NewStore(pool *pgxpool.Pool) *Store { return &Store{queries: sqlcgen.New(pool)} }

func (s *Store) Substances(ctx context.Context, f application.SubstanceFilter) ([]domain.Substance, error) {
	rows, err := s.queries.ListSubstances(ctx, sqlcgen.ListSubstancesParams{Search: pgtx.EscapeLike(f.Search), Kind: string(f.Kind)})
	if err != nil {
		return nil, fmt.Errorf("list substances: %w", err)
	}
	out := make([]domain.Substance, 0, len(rows))
	for _, row := range rows {
		out = append(out, toSubstance(row))
	}
	return out, nil
}

func (s *Store) Substance(ctx context.Context, id string) (domain.Substance, error) {
	if !pgtx.ValidUUID(id) {
		return domain.Substance{}, application.ErrSubstanceNotFound
	}
	row, err := s.queries.GetSubstance(ctx, id)
	if err != nil {
		return domain.Substance{}, translate(err, application.ErrSubstanceNotFound, nil, "get substance")
	}
	return toSubstance(row), nil
}

func (s *Store) CreateSubstance(ctx context.Context, v domain.Substance) (domain.Substance, error) {
	row, err := s.queries.CreateSubstance(ctx, sqlcgen.CreateSubstanceParams{Name: v.Name, Kind: string(v.Kind), Description: optional(v.Description)})
	if err != nil {
		return domain.Substance{}, translate(err, nil, nil, "create substance")
	}
	return toSubstance(row), nil
}

func (s *Store) UpdateSubstance(ctx context.Context, id string, c application.SubstanceChanges) (domain.Substance, error) {
	if !pgtx.ValidUUID(id) {
		return domain.Substance{}, application.ErrSubstanceNotFound
	}
	row, err := s.queries.UpdateSubstance(ctx, sqlcgen.UpdateSubstanceParams{ID: id, Name: c.Name, Kind: text(c.Kind), Description: c.Description})
	if err != nil {
		return domain.Substance{}, translate(err, application.ErrSubstanceNotFound, nil, "update substance")
	}
	return toSubstance(row), nil
}

func (s *Store) DeleteSubstance(ctx context.Context, id string) error {
	if !pgtx.ValidUUID(id) {
		return application.ErrSubstanceNotFound
	}
	deleted, err := s.queries.DeleteSubstance(ctx, id)
	if err != nil {
		return translate(err, nil, application.ErrSubstanceInUse, "delete substance")
	}
	if deleted == 0 {
		return application.ErrSubstanceNotFound
	}
	return nil
}

func (s *Store) DiseaseModels(ctx context.Context, f application.DiseaseModelFilter) ([]domain.DiseaseModel, error) {
	rows, err := s.queries.ListDiseaseModels(ctx, sqlcgen.ListDiseaseModelsParams{Search: pgtx.EscapeLike(f.Search), InductionType: string(f.InductionType)})
	if err != nil {
		return nil, fmt.Errorf("list disease models: %w", err)
	}
	out := make([]domain.DiseaseModel, 0, len(rows))
	for _, row := range rows {
		out = append(out, toDiseaseModel(row))
	}
	return out, nil
}

func (s *Store) DiseaseModel(ctx context.Context, id string) (domain.DiseaseModel, error) {
	if !pgtx.ValidUUID(id) {
		return domain.DiseaseModel{}, application.ErrDiseaseModelNotFound
	}
	row, err := s.queries.GetDiseaseModel(ctx, id)
	if err != nil {
		return domain.DiseaseModel{}, translate(err, application.ErrDiseaseModelNotFound, nil, "get disease model")
	}
	return toDiseaseModel(row), nil
}

func (s *Store) CreateDiseaseModel(ctx context.Context, m domain.DiseaseModel) (domain.DiseaseModel, error) {
	row, err := s.queries.CreateDiseaseModel(ctx, sqlcgen.CreateDiseaseModelParams{
		Name: m.Name, InductionType: string(m.InductionType), ConfirmationCriterion: optional(m.ConfirmationCriterion), Description: optional(m.Description),
	})
	if err != nil {
		return domain.DiseaseModel{}, translate(err, nil, nil, "create disease model")
	}
	return toDiseaseModel(row), nil
}

func (s *Store) UpdateDiseaseModel(ctx context.Context, id string, c application.DiseaseModelChanges) (domain.DiseaseModel, error) {
	if !pgtx.ValidUUID(id) {
		return domain.DiseaseModel{}, application.ErrDiseaseModelNotFound
	}
	row, err := s.queries.UpdateDiseaseModel(ctx, sqlcgen.UpdateDiseaseModelParams{
		ID: id, Name: c.Name, InductionType: text(c.InductionType), ConfirmationCriterion: c.ConfirmationCriterion, Description: c.Description,
	})
	if err != nil {
		return domain.DiseaseModel{}, translate(err, application.ErrDiseaseModelNotFound, nil, "update disease model")
	}
	return toDiseaseModel(row), nil
}

func (s *Store) DeleteDiseaseModel(ctx context.Context, id string) error {
	if !pgtx.ValidUUID(id) {
		return application.ErrDiseaseModelNotFound
	}
	deleted, err := s.queries.DeleteDiseaseModel(ctx, id)
	if err != nil {
		return translate(err, nil, application.ErrDiseaseModelInUse, "delete disease model")
	}
	if deleted == 0 {
		return application.ErrDiseaseModelNotFound
	}
	return nil
}

// translate maps a missing row to notFound, a row still referenced to inUse
// and named constraint violations to their errors. Other errors are wrapped so
// PostgreSQL codes stay inspectable.
func translate(err, notFound, inUse error, operation string) error {
	if notFound != nil && errors.Is(err, pgx.ErrNoRows) {
		return notFound
	}
	code, constraint := pgtx.Violation(err)
	if mapped := constraintErrors[constraint]; mapped != nil {
		return mapped
	}
	if inUse != nil && code == foreignKeyViolation {
		return inUse
	}
	return fmt.Errorf("%s: %w", operation, err)
}

func toSubstance(r sqlcgen.MiskoSubstance) domain.Substance {
	return domain.Substance{
		ID: r.ID, Name: r.Name, Kind: domain.SubstanceKind(r.Kind), Description: value(r.Description),
		CreatedAt: r.CreatedAt.UTC(), UpdatedAt: r.UpdatedAt.UTC(),
	}
}

func toDiseaseModel(r sqlcgen.MiskoDiseaseModel) domain.DiseaseModel {
	return domain.DiseaseModel{
		ID: r.ID, Name: r.Name, InductionType: domain.InductionType(r.InductionType),
		ConfirmationCriterion: value(r.ConfirmationCriterion), Description: value(r.Description),
		CreatedAt: r.CreatedAt.UTC(), UpdatedAt: r.UpdatedAt.UTC(),
	}
}

// optional stores an empty value as NULL.
func optional(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func value(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func text[T ~string](v *T) *string {
	if v == nil {
		return nil
	}
	s := string(*v)
	return &s
}
