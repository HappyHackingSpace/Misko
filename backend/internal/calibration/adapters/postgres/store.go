// Package postgres implements calibration persistence with sqlc-generated queries.
package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/HappyHackingSpace/Misko/backend/internal/calibration/adapters/postgres/sqlcgen"
	"github.com/HappyHackingSpace/Misko/backend/internal/calibration/application"
	"github.com/HappyHackingSpace/Misko/backend/internal/calibration/domain"
	"github.com/HappyHackingSpace/Misko/backend/internal/platform/pgtx"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// constraintErrors maps named constraints to the error a violation means.
var constraintErrors = map[string]error{
	"calibrations_first_key":       domain.ErrStaleCorrection,
	"calibrations_supersedes_key":  domain.ErrStaleCorrection,
	"calibrations_supersedes_fkey": domain.ErrStaleCorrection,
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

func (s *Store) Recording(ctx context.Context, testID, recordingID string) (application.RecordingRef, error) {
	if !pgtx.ValidUUID(testID) || !pgtx.ValidUUID(recordingID) {
		return application.RecordingRef{}, application.ErrRecordingNotFound
	}
	row, err := s.queries.GetRecordingRef(ctx, sqlcgen.GetRecordingRefParams{TestID: testID, ID: recordingID})
	if err != nil {
		return application.RecordingRef{}, translate(err, application.ErrRecordingNotFound, "get recording")
	}
	return toRef(row)
}

func (s *Store) LockRecording(ctx context.Context, testID, recordingID string) (application.RecordingRef, error) {
	if !pgtx.ValidUUID(testID) || !pgtx.ValidUUID(recordingID) {
		return application.RecordingRef{}, application.ErrRecordingNotFound
	}
	row, err := s.queries.LockRecordingRef(ctx, sqlcgen.LockRecordingRefParams{TestID: testID, ID: recordingID})
	if err != nil {
		return application.RecordingRef{}, translate(err, application.ErrRecordingNotFound, "lock recording")
	}
	return toRef(sqlcgen.GetRecordingRefRow(row))
}

func (s *Store) Calibrations(ctx context.Context, recordingID string) ([]domain.Calibration, error) {
	rows, err := s.queries.ListCalibrations(ctx, recordingID)
	if err != nil {
		return nil, fmt.Errorf("list calibrations: %w", err)
	}
	out := make([]domain.Calibration, 0, len(rows))
	for _, row := range rows {
		c, err := toCalibration(row)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, nil
}

func (s *Store) CreateCalibration(ctx context.Context, c domain.Calibration) (domain.Calibration, error) {
	fit, err := json.Marshal(points(c.Fit))
	if err != nil {
		return domain.Calibration{}, fmt.Errorf("encode FIT points: %w", err)
	}
	check, err := json.Marshal(points(c.Check))
	if err != nil {
		return domain.Calibration{}, fmt.Errorf("encode CHECK points: %w", err)
	}
	row, err := s.queries.CreateCalibration(ctx, sqlcgen.CreateCalibrationParams{
		RecordingID: c.RecordingID, SupersedesID: optional(c.SupersedesID), CameraID: c.CameraID,
		FrameWidth: int32(c.FrameWidth), FrameHeight: int32(c.FrameHeight),
		CropX: int32(c.Crop.X), CropY: int32(c.Crop.Y), CropWidth: int32(c.Crop.Width), CropHeight: int32(c.Crop.Height),
		ReferenceFrameUs: c.ReferenceFrameUs, MeasurementPlane: string(c.Plane), FitPoints: fit, CheckPoints: check, Transform: c.Transform[:],
		FitRmsErrorCm: c.FitRMSErrorCm, CheckRmsErrorCm: c.CheckRMSErrorCm, CheckMaxErrorCm: c.CheckMaxErrorCm, ToleranceCm: c.ToleranceCm,
		AlgorithmVersion: c.AlgorithmVersion, Status: string(c.Status), RejectionReason: optional(c.RejectionReason), CreatedBy: c.CreatedBy,
	})
	if err != nil {
		return domain.Calibration{}, translate(err, nil, "create calibration")
	}
	return toCalibration(row)
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

type pointJSON struct {
	PixelX float64 `json:"pixelX"`
	PixelY float64 `json:"pixelY"`
	WorldX float64 `json:"worldX"`
	WorldY float64 `json:"worldY"`
}

func points(in []domain.Correspondence) []pointJSON {
	out := make([]pointJSON, 0, len(in))
	for _, p := range in {
		out = append(out, pointJSON(p))
	}
	return out
}

func decodePoints(raw []byte) ([]domain.Correspondence, error) {
	var stored []pointJSON
	if err := json.Unmarshal(raw, &stored); err != nil {
		return nil, fmt.Errorf("decode calibration points: %w", err)
	}
	out := make([]domain.Correspondence, 0, len(stored))
	for _, p := range stored {
		out = append(out, domain.Correspondence(p))
	}
	return out, nil
}

func toRef(r sqlcgen.GetRecordingRefRow) (application.RecordingRef, error) {
	apparatus := map[string]float64{}
	if err := json.Unmarshal(r.Apparatus, &apparatus); err != nil {
		return application.RecordingRef{}, fmt.Errorf("decode apparatus: %w", err)
	}
	return application.RecordingRef{
		ID: r.ID, TestID: r.TestID, VideoStatus: r.VideoStatus, ParadigmKey: r.ParadigmKey, ParadigmVersion: int(r.ParadigmVersion), Apparatus: apparatus,
	}, nil
}

func toCalibration(r sqlcgen.MiskoCalibration) (domain.Calibration, error) {
	fit, err := decodePoints(r.FitPoints)
	if err != nil {
		return domain.Calibration{}, err
	}
	check, err := decodePoints(r.CheckPoints)
	if err != nil {
		return domain.Calibration{}, err
	}
	if len(r.Transform) != 9 {
		return domain.Calibration{}, fmt.Errorf("calibration %s has %d transform values", r.ID, len(r.Transform))
	}
	c := domain.Calibration{
		ID: r.ID, RecordingID: r.RecordingID, SupersedesID: value(r.SupersedesID), CameraID: r.CameraID,
		FrameWidth: int(r.FrameWidth), FrameHeight: int(r.FrameHeight),
		Crop:             domain.Rect{X: int(r.CropX), Y: int(r.CropY), Width: int(r.CropWidth), Height: int(r.CropHeight)},
		ReferenceFrameUs: r.ReferenceFrameUs, Plane: domain.Plane(r.MeasurementPlane), Fit: fit, Check: check,
		FitRMSErrorCm: r.FitRmsErrorCm, CheckRMSErrorCm: r.CheckRmsErrorCm, CheckMaxErrorCm: r.CheckMaxErrorCm, ToleranceCm: r.ToleranceCm,
		AlgorithmVersion: r.AlgorithmVersion, Status: domain.Status(r.Status), RejectionReason: value(r.RejectionReason),
		CreatedBy: r.CreatedBy, CreatedAt: r.CreatedAt.UTC(),
	}
	copy(c.Transform[:], r.Transform)
	return c, nil
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
