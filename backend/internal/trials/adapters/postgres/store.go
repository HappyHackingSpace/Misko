// Package postgres implements paradigm trial persistence with sqlc-generated
// queries.
package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/HappyHackingSpace/Misko/backend/internal/platform/pgtx"
	"github.com/HappyHackingSpace/Misko/backend/internal/trials/adapters/postgres/sqlcgen"
	"github.com/HappyHackingSpace/Misko/backend/internal/trials/application"
	"github.com/HappyHackingSpace/Misko/backend/internal/trials/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// constraintErrors maps named constraints and trigger rules to the error a
// violation means, so the API answers with a code instead of a 500.
var constraintErrors = map[string]error{
	"paradigm_trials_arena_fkey":    application.ErrParadigmMismatch,
	"paradigm_trials_name_check":    domain.ErrInvalidName,
	"paradigm_trials_notes_check":   domain.ErrInvalidNotes,
	"paradigm_trials_clip_check":    domain.ErrInvalidClip,
	"paradigm_trials_frozen":        domain.ErrDeleted,
	"paradigm_trial_jobs_requester": application.ErrNotReady,
}

type Store struct {
	pool    *pgxpool.Pool
	queries *sqlcgen.Queries
	inTx    bool
}

func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool, queries: sqlcgen.New(pool)} }

// Transaction runs fn in a transaction, or in the current one when nested.
func (s *Store) Transaction(ctx context.Context, fn func(application.Store) error) error {
	if s.inTx {
		return fn(s)
	}
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		return fn(&Store{pool: s.pool, queries: s.queries.WithTx(tx), inTx: true})
	})
}

func (s *Store) Create(ctx context.Context, t domain.Trial) (domain.Trial, error) {
	row, err := s.queries.CreateTrial(ctx, sqlcgen.CreateTrialParams{Name: t.Name, ParadigmKey: t.ParadigmKey, ParadigmVersion: int32(t.ParadigmVersion),
		EnvironmentRevisionID: t.EnvironmentRevisionID, Notes: optional(t.Notes), CreatedBy: t.CreatedBy})
	if err != nil {
		return domain.Trial{}, translate(err, nil, "create trial")
	}
	return toTrial(row), nil
}

func (s *Store) Trial(ctx context.Context, id string) (domain.Trial, error) {
	if !pgtx.ValidUUID(id) {
		return domain.Trial{}, application.ErrTrialNotFound
	}
	row, err := s.queries.GetTrial(ctx, id)
	if err != nil {
		return domain.Trial{}, translate(err, application.ErrTrialNotFound, "get trial")
	}
	return toTrial(row), nil
}

// LockTrial must run inside Transaction; the lock lasts until it ends.
func (s *Store) LockTrial(ctx context.Context, id string) (domain.Trial, error) {
	if !pgtx.ValidUUID(id) {
		return domain.Trial{}, application.ErrTrialNotFound
	}
	row, err := s.queries.LockTrial(ctx, id)
	if err != nil {
		return domain.Trial{}, translate(err, application.ErrTrialNotFound, "lock trial")
	}
	return toTrial(row), nil
}

func (s *Store) SetVideo(ctx context.Context, id, videoID string, clip domain.Clip) (domain.Trial, error) {
	row, err := s.queries.SetVideo(ctx, sqlcgen.SetVideoParams{ID: id, VideoAssetID: &videoID, ClipStartUs: clip.StartUs, ClipEndUs: clip.EndUs})
	if err != nil {
		return domain.Trial{}, translate(err, application.ErrTrialNotFound, "set trial video")
	}
	return toTrial(row), nil
}

func (s *Store) Delete(ctx context.Context, id string, at time.Time) error {
	if err := s.queries.DeleteTrial(ctx, sqlcgen.DeleteTrialParams{ID: id, DeletedAt: &at}); err != nil {
		return translate(err, nil, "delete trial")
	}
	return nil
}

func (s *Store) List(ctx context.Context, f application.ListFilter) ([]application.ListItem, int, error) {
	rows, err := s.queries.ListTrials(ctx, sqlcgen.ListTrialsParams{Search: f.Search, ParadigmKey: f.ParadigmKey, PageLimit: int32(f.Limit),
		PageOffset: int32(f.Offset)})
	if err != nil {
		return nil, 0, fmt.Errorf("list trials: %w", err)
	}
	out := make([]application.ListItem, 0, len(rows))
	for _, r := range rows {
		out = append(out, application.ListItem{
			Trial: toTrial(sqlcgen.MiskoParadigmTrial{ID: r.ID, Name: r.Name, ParadigmKey: r.ParadigmKey, ParadigmVersion: r.ParadigmVersion,
				EnvironmentRevisionID: r.EnvironmentRevisionID, VideoAssetID: r.VideoAssetID, ClipStartUs: r.ClipStartUs, ClipEndUs: r.ClipEndUs,
				Notes: r.Notes, CreatedBy: r.CreatedBy, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt, DeletedAt: r.DeletedAt}),
			EnvironmentID: r.EnvironmentID, EnvironmentName: r.EnvironmentName, RevisionNumber: int(r.RevisionNumber),
		})
	}
	total, err := s.queries.CountTrials(ctx, sqlcgen.CountTrialsParams{Search: f.Search, ParadigmKey: f.ParadigmKey})
	if err != nil {
		return nil, 0, fmt.Errorf("count trials: %w", err)
	}
	return out, int(total), nil
}

func (s *Store) Candidates(ctx context.Context, limit int) ([]domain.Trial, error) {
	rows, err := s.queries.Candidates(ctx, int32(limit))
	if err != nil {
		return nil, fmt.Errorf("trial candidates: %w", err)
	}
	out := make([]domain.Trial, 0, len(rows))
	for _, r := range rows {
		out = append(out, toTrial(r))
	}
	return out, nil
}

func (s *Store) CreateLink(ctx context.Context, l domain.Link) (bool, error) {
	overrides := l.Overrides
	if overrides == nil {
		overrides = map[string]float64{}
	}
	raw, err := json.Marshal(overrides)
	if err != nil {
		return false, fmt.Errorf("encode overrides: %w", err)
	}
	n, err := s.queries.CreateLink(ctx, sqlcgen.CreateLinkParams{JobID: l.JobID, TrialID: l.TrialID, VideoAssetID: l.VideoAssetID,
		CalibrationID: optional(l.CalibrationID), Trigger: string(l.Trigger), Overrides: raw, CreatedBy: optional(l.CreatedBy)})
	if err != nil {
		return false, translate(err, nil, "create trial link")
	}
	return n == 1, nil
}

func (s *Store) Links(ctx context.Context, trialID string) ([]domain.Link, error) {
	if !pgtx.ValidUUID(trialID) {
		return []domain.Link{}, nil
	}
	rows, err := s.queries.ListLinks(ctx, trialID)
	if err != nil {
		return nil, fmt.Errorf("list trial links: %w", err)
	}
	out := make([]domain.Link, 0, len(rows))
	for _, r := range rows {
		overrides := map[string]float64{}
		if err := json.Unmarshal(r.Overrides, &overrides); err != nil {
			return nil, fmt.Errorf("decode overrides of job %s: %w", r.JobID, err)
		}
		out = append(out, domain.Link{JobID: r.JobID, TrialID: r.TrialID, VideoAssetID: r.VideoAssetID, CalibrationID: value(r.CalibrationID),
			Trigger: domain.Trigger(r.Trigger), Overrides: overrides, CreatedBy: value(r.CreatedBy), CreatedAt: r.CreatedAt.UTC()})
	}
	return out, nil
}

func toTrial(r sqlcgen.MiskoParadigmTrial) domain.Trial {
	var deleted *time.Time
	if r.DeletedAt != nil {
		t := r.DeletedAt.UTC()
		deleted = &t
	}
	return domain.Trial{
		ID: r.ID, Name: r.Name, ParadigmKey: r.ParadigmKey, ParadigmVersion: int(r.ParadigmVersion), EnvironmentRevisionID: r.EnvironmentRevisionID,
		VideoAssetID: value(r.VideoAssetID), Clip: domain.Clip{StartUs: r.ClipStartUs, EndUs: r.ClipEndUs}, Notes: value(r.Notes),
		CreatedBy: r.CreatedBy, CreatedAt: r.CreatedAt.UTC(), UpdatedAt: r.UpdatedAt.UTC(), DeletedAt: deleted,
	}
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
