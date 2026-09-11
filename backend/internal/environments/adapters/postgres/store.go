// Package postgres implements environment persistence with sqlc-generated queries.
package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/HappyHackingSpace/Misko/backend/internal/environments/adapters/postgres/sqlcgen"
	"github.com/HappyHackingSpace/Misko/backend/internal/environments/application"
	"github.com/HappyHackingSpace/Misko/backend/internal/environments/domain"
	"github.com/HappyHackingSpace/Misko/backend/internal/platform/pgtx"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"math"
)

// constraintErrors maps named constraints to the error a violation means.
var constraintErrors = map[string]error{
	"environments_name_key":                  application.ErrNameTaken,
	"environment_revisions_environment_fkey": application.ErrEnvironmentNotFound,
}

type Store struct {
	pool    *pgxpool.Pool
	queries *sqlcgen.Queries
}

func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool, queries: sqlcgen.New(pool)} }

func (s *Store) Transaction(ctx context.Context, fn func(application.Store) error) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		return fn(&Store{pool: s.pool, queries: s.queries.WithTx(tx)})
	})
}

func (s *Store) Environments(ctx context.Context, paradigmKey string) ([]domain.Environment, error) {
	rows, err := s.queries.ListEnvironments(ctx, paradigmKey)
	if err != nil {
		return nil, fmt.Errorf("list environments: %w", err)
	}
	out := make([]domain.Environment, 0, len(rows))
	for _, row := range rows {
		out = append(out, toEnvironment(row))
	}
	return out, nil
}

func (s *Store) Environment(ctx context.Context, id string) (domain.Environment, error) {
	if !pgtx.ValidUUID(id) {
		return domain.Environment{}, application.ErrEnvironmentNotFound
	}
	row, err := s.queries.GetEnvironment(ctx, id)
	if err != nil {
		return domain.Environment{}, translate(err, application.ErrEnvironmentNotFound, "get environment")
	}
	return toEnvironment(sqlcgen.ListEnvironmentsRow(row)), nil
}

func (s *Store) CreateEnvironment(ctx context.Context, e domain.Environment) (domain.Environment, error) {
	row, err := s.queries.CreateEnvironment(ctx, sqlcgen.CreateEnvironmentParams{Name: e.Name, ParadigmKey: e.ParadigmKey, Notes: optional(e.Notes)})
	if err != nil {
		return domain.Environment{}, translate(err, nil, "create environment")
	}
	return toEnvironment(withoutRevision(row)), nil
}

func (s *Store) UpdateEnvironment(ctx context.Context, id string, c application.Changes) (domain.Environment, error) {
	if !pgtx.ValidUUID(id) {
		return domain.Environment{}, application.ErrEnvironmentNotFound
	}
	row, err := s.queries.UpdateEnvironment(ctx, sqlcgen.UpdateEnvironmentParams{ID: id, Name: c.Name, Notes: c.Notes})
	if err != nil {
		return domain.Environment{}, translate(err, application.ErrEnvironmentNotFound, "update environment")
	}
	return toEnvironment(sqlcgen.ListEnvironmentsRow(row)), nil
}

func (s *Store) LockEnvironment(ctx context.Context, id string) (domain.Environment, error) {
	if !pgtx.ValidUUID(id) {
		return domain.Environment{}, application.ErrEnvironmentNotFound
	}
	row, err := s.queries.LockEnvironment(ctx, id)
	if err != nil {
		return domain.Environment{}, translate(err, application.ErrEnvironmentNotFound, "lock environment")
	}
	return toEnvironment(withoutRevision(row)), nil
}

func (s *Store) CreateRevision(ctx context.Context, r domain.Revision) (domain.Revision, error) {
	if !pgtx.ValidUUID(r.EnvironmentID) {
		return domain.Revision{}, application.ErrEnvironmentNotFound
	}
	apparatus, err := encode(r.Apparatus)
	if err != nil {
		return domain.Revision{}, err
	}
	row, err := s.queries.CreateRevision(ctx, sqlcgen.CreateRevisionParams{
		EnvironmentID: r.EnvironmentID, ParadigmKey: r.ParadigmKey, ParadigmVersion: int32(r.ParadigmVersion),
		Apparatus: apparatus, Notes: optional(r.Notes), CreatedBy: r.CreatedBy,
	})
	if err != nil {
		return domain.Revision{}, translate(err, nil, "create revision")
	}
	return toRevision(row)
}

func (s *Store) Revisions(ctx context.Context, environmentID string) ([]domain.Revision, error) {
	if !pgtx.ValidUUID(environmentID) {
		return []domain.Revision{}, nil
	}
	rows, err := s.queries.ListRevisions(ctx, environmentID)
	if err != nil {
		return nil, fmt.Errorf("list revisions: %w", err)
	}
	out := make([]domain.Revision, 0, len(rows))
	for _, row := range rows {
		r, err := toRevision(row)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, nil
}

func (s *Store) Revision(ctx context.Context, environmentID string, number int) (domain.Revision, error) {
	if !pgtx.ValidUUID(environmentID) || number < 1 || number > math.MaxInt32 {
		return domain.Revision{}, application.ErrRevisionNotFound
	}
	row, err := s.queries.GetRevision(ctx, sqlcgen.GetRevisionParams{EnvironmentID: environmentID, Number: int32(number)})
	if err != nil {
		return domain.Revision{}, translate(err, application.ErrRevisionNotFound, "get revision")
	}
	return toRevision(row)
}

// translate maps a missing row to notFound and named constraint violations to
// their errors. Other errors are wrapped so PostgreSQL codes stay inspectable.
func translate(err, notFound error, operation string) error {
	if notFound != nil && errors.Is(err, pgx.ErrNoRows) {
		return notFound
	}
	if _, constraint := pgtx.Violation(err); constraintErrors[constraint] != nil {
		return constraintErrors[constraint]
	}
	return fmt.Errorf("%s: %w", operation, err)
}

func withoutRevision(r sqlcgen.MiskoEnvironment) sqlcgen.ListEnvironmentsRow {
	return sqlcgen.ListEnvironmentsRow{ID: r.ID, Name: r.Name, ParadigmKey: r.ParadigmKey, Notes: r.Notes, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt}
}

func toEnvironment(r sqlcgen.ListEnvironmentsRow) domain.Environment {
	return domain.Environment{
		ID: r.ID, Name: r.Name, ParadigmKey: r.ParadigmKey, Notes: value(r.Notes), LatestRevision: int(r.LatestRevision),
		CreatedAt: r.CreatedAt.UTC(), UpdatedAt: r.UpdatedAt.UTC(),
	}
}

func toRevision(r sqlcgen.MiskoEnvironmentRevision) (domain.Revision, error) {
	apparatus := map[string]float64{}
	if err := json.Unmarshal(r.Apparatus, &apparatus); err != nil {
		return domain.Revision{}, fmt.Errorf("decode apparatus of revision %s: %w", r.ID, err)
	}
	return domain.Revision{
		ID: r.ID, EnvironmentID: r.EnvironmentID, ParadigmKey: r.ParadigmKey, Number: int(r.Number), ParadigmVersion: int(r.ParadigmVersion),
		Apparatus: apparatus, Notes: value(r.Notes), CreatedBy: r.CreatedBy, CreatedAt: r.CreatedAt.UTC(),
	}, nil
}

func encode(values map[string]float64) ([]byte, error) {
	if values == nil {
		values = map[string]float64{}
	}
	out, err := json.Marshal(values)
	if err != nil {
		return nil, fmt.Errorf("encode apparatus: %w", err)
	}
	return out, nil
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
