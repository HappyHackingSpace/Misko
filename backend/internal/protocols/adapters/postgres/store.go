// Package postgres implements protocol persistence with sqlc-generated queries.
package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/HappyHackingSpace/Misko/backend/internal/platform/pgtx"
	"github.com/HappyHackingSpace/Misko/backend/internal/protocols/adapters/postgres/sqlcgen"
	"github.com/HappyHackingSpace/Misko/backend/internal/protocols/application"
	"github.com/HappyHackingSpace/Misko/backend/internal/protocols/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"math"
)

// constraintErrors maps named constraints to the error a violation means.
var constraintErrors = map[string]error{
	"protocols_name_key":              application.ErrNameTaken,
	"protocols_experiment_id_fkey":    application.ErrExperimentNotFound,
	"protocol_versions_protocol_fkey": application.ErrProtocolNotFound,
	"protocol_steps_environment_fkey": application.ErrIncompatibleEnvironment,
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

func (s *Store) Protocols(ctx context.Context, experimentID string) ([]domain.Protocol, error) {
	if !pgtx.ValidUUID(experimentID) {
		return []domain.Protocol{}, nil
	}
	rows, err := s.queries.ListProtocols(ctx, experimentID)
	if err != nil {
		return nil, fmt.Errorf("list protocols: %w", err)
	}
	out := make([]domain.Protocol, 0, len(rows))
	for _, row := range rows {
		out = append(out, toProtocol(row))
	}
	return out, nil
}

func (s *Store) Protocol(ctx context.Context, experimentID, protocolID string) (domain.Protocol, error) {
	if !pgtx.ValidUUID(experimentID) || !pgtx.ValidUUID(protocolID) {
		return domain.Protocol{}, application.ErrProtocolNotFound
	}
	row, err := s.queries.GetProtocol(ctx, sqlcgen.GetProtocolParams{ExperimentID: experimentID, ID: protocolID})
	if err != nil {
		return domain.Protocol{}, translate(err, application.ErrProtocolNotFound, "get protocol")
	}
	return toProtocol(sqlcgen.ListProtocolsRow(row)), nil
}

func (s *Store) CreateProtocol(ctx context.Context, p domain.Protocol) (domain.Protocol, error) {
	if !pgtx.ValidUUID(p.ExperimentID) {
		return domain.Protocol{}, application.ErrExperimentNotFound
	}
	row, err := s.queries.CreateProtocol(ctx, sqlcgen.CreateProtocolParams{ExperimentID: p.ExperimentID, Name: p.Name, Description: optional(p.Description)})
	if err != nil {
		return domain.Protocol{}, translate(err, nil, "create protocol")
	}
	return toProtocol(withoutVersion(row)), nil
}

func (s *Store) UpdateProtocol(ctx context.Context, experimentID, protocolID string, c application.Changes) (domain.Protocol, error) {
	if !pgtx.ValidUUID(experimentID) || !pgtx.ValidUUID(protocolID) {
		return domain.Protocol{}, application.ErrProtocolNotFound
	}
	row, err := s.queries.UpdateProtocol(ctx, sqlcgen.UpdateProtocolParams{ExperimentID: experimentID, ID: protocolID, Name: c.Name, Description: c.Description})
	if err != nil {
		return domain.Protocol{}, translate(err, application.ErrProtocolNotFound, "update protocol")
	}
	return toProtocol(sqlcgen.ListProtocolsRow(row)), nil
}

func (s *Store) LockProtocol(ctx context.Context, experimentID, protocolID string) (domain.Protocol, error) {
	if !pgtx.ValidUUID(experimentID) || !pgtx.ValidUUID(protocolID) {
		return domain.Protocol{}, application.ErrProtocolNotFound
	}
	row, err := s.queries.LockProtocol(ctx, sqlcgen.LockProtocolParams{ExperimentID: experimentID, ID: protocolID})
	if err != nil {
		return domain.Protocol{}, translate(err, application.ErrProtocolNotFound, "lock protocol")
	}
	return toProtocol(withoutVersion(row)), nil
}

func (s *Store) EnvironmentRevision(ctx context.Context, id string) (application.RevisionRef, error) {
	if !pgtx.ValidUUID(id) {
		return application.RevisionRef{}, application.ErrEnvironmentRevisionNotFound
	}
	row, err := s.queries.GetEnvironmentRevision(ctx, id)
	if err != nil {
		return application.RevisionRef{}, translate(err, application.ErrEnvironmentRevisionNotFound, "get environment revision")
	}
	apparatus, err := decode(row.Apparatus)
	if err != nil {
		return application.RevisionRef{}, err
	}
	return application.RevisionRef{
		ID: row.ID, EnvironmentID: row.EnvironmentID, Number: int(row.Number),
		ParadigmKey: row.ParadigmKey, ParadigmVersion: int(row.ParadigmVersion), Apparatus: apparatus,
	}, nil
}

// CreateVersion must run inside Transaction: a deferred constraint checks at
// commit that the version has exactly its steps.
func (s *Store) CreateVersion(ctx context.Context, v domain.Version) (domain.Version, error) {
	if !pgtx.ValidUUID(v.ExperimentID) || !pgtx.ValidUUID(v.ProtocolID) {
		return domain.Version{}, application.ErrProtocolNotFound
	}
	row, err := s.queries.CreateVersion(ctx, sqlcgen.CreateVersionParams{
		ExperimentID: v.ExperimentID, ProtocolID: v.ProtocolID, StepCount: int32(len(v.Steps)), Notes: optional(v.Notes), CreatedBy: v.CreatedBy,
	})
	if err != nil {
		return domain.Version{}, translate(err, nil, "create version")
	}
	for _, step := range v.Steps {
		if !pgtx.ValidUUID(step.EnvironmentRevisionID) {
			return domain.Version{}, application.ErrEnvironmentRevisionNotFound
		}
		session, err := encode(step.Session)
		if err != nil {
			return domain.Version{}, err
		}
		err = s.queries.CreateStep(ctx, sqlcgen.CreateStepParams{
			ProtocolVersionID: row.ID, Position: int32(step.Position), ParadigmKey: step.ParadigmKey, ParadigmVersion: int32(step.ParadigmVersion),
			EnvironmentRevisionID: step.EnvironmentRevisionID, TrialType: step.TrialType, Trials: int32(step.Trials),
			InterTrialIntervalS: int32(step.InterTrialIntervalS), Session: session, Notes: optional(step.Notes),
		})
		if err != nil {
			return domain.Version{}, translate(err, nil, "create step")
		}
	}
	out := toVersion(row)
	out.Steps = v.Steps
	return out, nil
}

func (s *Store) Versions(ctx context.Context, experimentID, protocolID string) ([]domain.Version, error) {
	if !pgtx.ValidUUID(experimentID) || !pgtx.ValidUUID(protocolID) {
		return []domain.Version{}, nil
	}
	rows, err := s.queries.ListVersions(ctx, sqlcgen.ListVersionsParams{ExperimentID: experimentID, ProtocolID: protocolID})
	if err != nil {
		return nil, fmt.Errorf("list versions: %w", err)
	}
	return s.withSteps(ctx, rows)
}

func (s *Store) Version(ctx context.Context, experimentID, protocolID string, number int) (domain.Version, error) {
	if !pgtx.ValidUUID(experimentID) || !pgtx.ValidUUID(protocolID) || number < 1 || number > math.MaxInt32 {
		return domain.Version{}, application.ErrVersionNotFound
	}
	row, err := s.queries.GetVersion(ctx, sqlcgen.GetVersionParams{ExperimentID: experimentID, ProtocolID: protocolID, Number: int32(number)})
	if err != nil {
		return domain.Version{}, translate(err, application.ErrVersionNotFound, "get version")
	}
	versions, err := s.withSteps(ctx, []sqlcgen.MiskoProtocolVersion{row})
	if err != nil {
		return domain.Version{}, err
	}
	return versions[0], nil
}

// withSteps loads the steps of all versions with one query.
func (s *Store) withSteps(ctx context.Context, rows []sqlcgen.MiskoProtocolVersion) ([]domain.Version, error) {
	ids := make([]string, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.ID)
	}
	stepRows, err := s.queries.ListSteps(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("list steps: %w", err)
	}
	steps := map[string][]domain.Step{}
	for _, r := range stepRows {
		session, err := decode(r.Session)
		if err != nil {
			return nil, err
		}
		steps[r.ProtocolVersionID] = append(steps[r.ProtocolVersionID], domain.Step{
			Position: int(r.Position), ParadigmKey: r.ParadigmKey, ParadigmVersion: int(r.ParadigmVersion),
			EnvironmentRevisionID: r.EnvironmentRevisionID, EnvironmentID: r.EnvironmentID, EnvironmentRevision: int(r.EnvironmentRevision),
			TrialType: r.TrialType, Trials: int(r.Trials), InterTrialIntervalS: int(r.InterTrialIntervalS), Session: session, Notes: value(r.Notes),
		})
	}
	out := make([]domain.Version, 0, len(rows))
	for _, row := range rows {
		v := toVersion(row)
		v.Steps = steps[row.ID]
		out = append(out, v)
	}
	return out, nil
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

func withoutVersion(r sqlcgen.MiskoProtocol) sqlcgen.ListProtocolsRow {
	return sqlcgen.ListProtocolsRow{ID: r.ID, ExperimentID: r.ExperimentID, Name: r.Name, Description: r.Description, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt}
}

func toProtocol(r sqlcgen.ListProtocolsRow) domain.Protocol {
	return domain.Protocol{
		ID: r.ID, ExperimentID: r.ExperimentID, Name: r.Name, Description: value(r.Description), LatestVersion: int(r.LatestVersion),
		CreatedAt: r.CreatedAt.UTC(), UpdatedAt: r.UpdatedAt.UTC(),
	}
}

func toVersion(r sqlcgen.MiskoProtocolVersion) domain.Version {
	return domain.Version{
		ID: r.ID, ExperimentID: r.ExperimentID, ProtocolID: r.ProtocolID, Number: int(r.Number),
		Notes: value(r.Notes), CreatedBy: r.CreatedBy, CreatedAt: r.CreatedAt.UTC(),
	}
}

func encode(values map[string]float64) ([]byte, error) {
	if values == nil {
		values = map[string]float64{}
	}
	out, err := json.Marshal(values)
	if err != nil {
		return nil, fmt.Errorf("encode parameters: %w", err)
	}
	return out, nil
}

func decode(raw []byte) (map[string]float64, error) {
	values := map[string]float64{}
	if err := json.Unmarshal(raw, &values); err != nil {
		return nil, fmt.Errorf("decode parameters: %w", err)
	}
	return values, nil
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
