// Package postgres implements video asset and recording persistence with
// sqlc-generated queries.
package postgres

import (
	"context"
	"errors"
	"fmt"
	"github.com/HappyHackingSpace/Misko/backend/internal/media/adapters/postgres/sqlcgen"
	"github.com/HappyHackingSpace/Misko/backend/internal/media/application"
	"github.com/HappyHackingSpace/Misko/backend/internal/media/domain"
	"github.com/HappyHackingSpace/Misko/backend/internal/platform/pgtx"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// constraintErrors maps named constraints to the error a violation means.
var constraintErrors = map[string]error{
	"test_recordings_test_fkey": application.ErrTestNotFound,
	"video_assets_immutable":    domain.ErrAlreadyFinalized,
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

func (s *Store) Test(ctx context.Context, id string) (application.TestRef, error) {
	if !pgtx.ValidUUID(id) {
		return application.TestRef{}, application.ErrTestNotFound
	}
	row, err := s.queries.GetTestRef(ctx, id)
	if err != nil {
		return application.TestRef{}, translate(err, application.ErrTestNotFound, "get test")
	}
	return application.TestRef{ID: row.ID, ExperimentID: row.ExperimentID, Status: row.Status}, nil
}

// CreateRecording must run inside Transaction so the asset and recording are stored together.
func (s *Store) CreateRecording(ctx context.Context, test application.TestRef, asset domain.Asset, clip domain.Clip) (domain.Recording, error) {
	a, err := s.queries.CreateAsset(ctx, sqlcgen.CreateAssetParams{
		Bucket: asset.Bucket, TestID: test.ID, ContentType: asset.ContentType, FileName: optional(asset.FileName),
		SizeBytes: asset.SizeBytes, Crc32c: int64(asset.CRC32C), CreatedBy: asset.CreatedBy,
	})
	if err != nil {
		return domain.Recording{}, translate(err, nil, "create video asset")
	}
	r, err := s.queries.CreateRecording(ctx, sqlcgen.CreateRecordingParams{
		ExperimentID: test.ExperimentID, TestID: test.ID, VideoAssetID: a.ID, ClipStartUs: clip.StartUs, ClipEndUs: clip.EndUs, CreatedBy: asset.CreatedBy,
	})
	if err != nil {
		return domain.Recording{}, translate(err, nil, "create recording")
	}
	return domain.Recording{
		ID: r.ID, ExperimentID: r.ExperimentID, TestID: r.TestID, Clip: domain.Clip{StartUs: r.ClipStartUs, EndUs: r.ClipEndUs},
		Asset: toAsset(a), CreatedBy: r.CreatedBy, CreatedAt: r.CreatedAt.UTC(),
	}, nil
}

func (s *Store) Recordings(ctx context.Context, testID string) ([]domain.Recording, error) {
	if !pgtx.ValidUUID(testID) {
		return []domain.Recording{}, nil
	}
	rows, err := s.queries.ListRecordings(ctx, testID)
	if err != nil {
		return nil, fmt.Errorf("list recordings: %w", err)
	}
	out := make([]domain.Recording, 0, len(rows))
	for _, row := range rows {
		out = append(out, toRecording(sqlcgen.GetRecordingRow(row)))
	}
	return out, nil
}

func (s *Store) Recording(ctx context.Context, testID, recordingID string) (domain.Recording, error) {
	if !pgtx.ValidUUID(testID) || !pgtx.ValidUUID(recordingID) {
		return domain.Recording{}, application.ErrRecordingNotFound
	}
	row, err := s.queries.GetRecording(ctx, sqlcgen.GetRecordingParams{TestID: testID, ID: recordingID})
	if err != nil {
		return domain.Recording{}, translate(err, application.ErrRecordingNotFound, "get recording")
	}
	return toRecording(row), nil
}

func (s *Store) LockRecording(ctx context.Context, testID, recordingID string) (domain.Recording, error) {
	if !pgtx.ValidUUID(testID) || !pgtx.ValidUUID(recordingID) {
		return domain.Recording{}, application.ErrRecordingNotFound
	}
	row, err := s.queries.LockRecording(ctx, sqlcgen.LockRecordingParams{TestID: testID, ID: recordingID})
	if err != nil {
		return domain.Recording{}, translate(err, application.ErrRecordingNotFound, "lock recording")
	}
	return toRecording(sqlcgen.GetRecordingRow(row)), nil
}

func (s *Store) FinishAsset(ctx context.Context, a domain.Asset) (domain.Asset, error) {
	generation := a.Generation
	row, err := s.queries.FinishAsset(ctx, sqlcgen.FinishAssetParams{
		ID: a.ID, Status: string(a.Status), Generation: &generation, RejectionReason: optional(a.RejectionReason), VerifiedAt: a.VerifiedAt,
	})
	if err != nil {
		return domain.Asset{}, translate(err, domain.ErrAlreadyFinalized, "finish video asset")
	}
	return toAsset(row), nil
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

func toAsset(r sqlcgen.MiskoVideoAsset) domain.Asset {
	a := domain.Asset{
		ID: r.ID, Kind: domain.Kind(r.Kind), Bucket: r.Bucket, ObjectName: r.ObjectName, ContentType: r.ContentType, FileName: value(r.FileName),
		SizeBytes: r.SizeBytes, CRC32C: uint32(r.Crc32c), Status: domain.Status(r.Status), RejectionReason: value(r.RejectionReason),
		CreatedBy: r.CreatedBy, CreatedAt: r.CreatedAt.UTC(),
	}
	if r.Generation != nil {
		a.Generation = *r.Generation
	}
	if r.VerifiedAt != nil {
		t := r.VerifiedAt.UTC()
		a.VerifiedAt = &t
	}
	return a
}

func toRecording(r sqlcgen.GetRecordingRow) domain.Recording {
	return domain.Recording{
		ID: r.ID, ExperimentID: r.ExperimentID, TestID: r.TestID, Clip: domain.Clip{StartUs: r.ClipStartUs, EndUs: r.ClipEndUs},
		CreatedBy: r.CreatedBy, CreatedAt: r.CreatedAt.UTC(),
		Asset: toAsset(sqlcgen.MiskoVideoAsset{
			ID: r.AssetID, Kind: r.Kind, Bucket: r.Bucket, ObjectName: r.ObjectName, ContentType: r.ContentType, FileName: r.FileName,
			SizeBytes: r.SizeBytes, Crc32c: r.Crc32c, Status: r.Status, Generation: r.Generation, RejectionReason: r.RejectionReason,
			VerifiedAt: r.VerifiedAt, CreatedBy: r.AssetCreatedBy, CreatedAt: r.AssetCreatedAt,
		}),
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
