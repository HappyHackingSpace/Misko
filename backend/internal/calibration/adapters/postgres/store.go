// Package postgres implements calibration persistence with sqlc-generated
// queries. The effective calibration of a test subject is read from the
// effective_calibrations view.
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
	"math"
)

// constraintErrors maps named constraints to the error a violation means.
var constraintErrors = map[string]error{
	"calibrations_first_key":        domain.ErrStaleCorrection,
	"calibrations_supersedes_key":   domain.ErrStaleCorrection,
	"calibrations_supersedes_fkey":  domain.ErrStaleCorrection,
	"calibrations_crop_check":       domain.ErrInvalidCrop,
	"calibrations_scope":            application.ErrInvalidScope,
	"calibrations_override_check":   application.ErrInvalidScope,
	"calibration_zones_frame":       domain.ErrZoneOutsideFrame,
	"calibration_zones_shape_check": domain.ErrInvalidZone,
	"calibration_zones_frozen":      domain.ErrZonesFrozen,
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

func (s *Store) TestExists(ctx context.Context, testID string) (bool, error) {
	if !pgtx.ValidUUID(testID) {
		return false, nil
	}
	exists, err := s.queries.GetTestExists(ctx, testID)
	if err != nil {
		return false, fmt.Errorf("get test: %w", err)
	}
	return exists, nil
}

func (s *Store) TestSubject(ctx context.Context, testID, testSubjectID string) (application.TestSubjectRef, error) {
	if !pgtx.ValidUUID(testID) || !pgtx.ValidUUID(testSubjectID) {
		return application.TestSubjectRef{}, application.ErrTestSubjectNotFound
	}
	row, err := s.queries.GetTestSubjectRef(ctx, sqlcgen.GetTestSubjectRefParams{TestID: testID, ID: testSubjectID})
	if err != nil {
		return application.TestSubjectRef{}, translate(err, application.ErrTestSubjectNotFound, "get test subject")
	}
	return toSubjectRef(row)
}

func (s *Store) LockTestSubject(ctx context.Context, testID, testSubjectID string) (application.TestSubjectRef, error) {
	if !pgtx.ValidUUID(testID) || !pgtx.ValidUUID(testSubjectID) {
		return application.TestSubjectRef{}, application.ErrTestSubjectNotFound
	}
	row, err := s.queries.LockTestSubjectRef(ctx, sqlcgen.LockTestSubjectRefParams{TestID: testID, ID: testSubjectID})
	if err != nil {
		return application.TestSubjectRef{}, translate(err, application.ErrTestSubjectNotFound, "lock test subject")
	}
	return toSubjectRef(sqlcgen.GetTestSubjectRefRow(row))
}

func (s *Store) CurrentVideo(ctx context.Context, testID string) (application.VideoRef, error) {
	if !pgtx.ValidUUID(testID) {
		return application.VideoRef{}, application.ErrNoVideo
	}
	row, err := s.queries.GetCurrentVideo(ctx, testID)
	if err != nil {
		return application.VideoRef{}, translate(err, application.ErrNoVideo, "get current video")
	}
	return application.VideoRef{RecordingID: row.ID, Status: row.Status}, nil
}

func (s *Store) Revision(ctx context.Context, environmentID string, number int) (application.RevisionRef, error) {
	if !pgtx.ValidUUID(environmentID) || number < 1 || number > math.MaxInt32 {
		return application.RevisionRef{}, application.ErrRevisionNotFound
	}
	row, err := s.queries.GetRevisionRef(ctx, sqlcgen.GetRevisionRefParams{EnvironmentID: environmentID, Number: int32(number)})
	if err != nil {
		return application.RevisionRef{}, translate(err, application.ErrRevisionNotFound, "get revision")
	}
	return toRevisionRef(row)
}

func (s *Store) LockRevision(ctx context.Context, environmentID string, number int) (application.RevisionRef, error) {
	if !pgtx.ValidUUID(environmentID) || number < 1 || number > math.MaxInt32 {
		return application.RevisionRef{}, application.ErrRevisionNotFound
	}
	row, err := s.queries.LockRevisionRef(ctx, sqlcgen.LockRevisionRefParams{EnvironmentID: environmentID, Number: int32(number)})
	if err != nil {
		return application.RevisionRef{}, translate(err, application.ErrRevisionNotFound, "lock revision")
	}
	return toRevisionRef(sqlcgen.GetRevisionRefRow(row))
}

func (s *Store) Overrides(ctx context.Context, testSubjectID string) ([]domain.Calibration, error) {
	if !pgtx.ValidUUID(testSubjectID) {
		return nil, nil
	}
	rows, err := s.queries.ListOverrides(ctx, &testSubjectID)
	if err != nil {
		return nil, fmt.Errorf("list overrides: %w", err)
	}
	return s.toCalibrations(ctx, rows)
}

func (s *Store) EnvironmentCalibrations(ctx context.Context, revisionID string) ([]domain.Calibration, error) {
	if !pgtx.ValidUUID(revisionID) {
		return nil, nil
	}
	rows, err := s.queries.ListEnvironmentCalibrations(ctx, revisionID)
	if err != nil {
		return nil, fmt.Errorf("list environment calibrations: %w", err)
	}
	return s.toCalibrations(ctx, rows)
}

func (s *Store) Calibration(ctx context.Context, id string) (domain.Calibration, error) {
	row, err := s.queries.GetCalibration(ctx, id)
	if err != nil {
		return domain.Calibration{}, fmt.Errorf("get calibration: %w", err)
	}
	list, err := s.toCalibrations(ctx, []sqlcgen.MiskoCalibration{row})
	if err != nil {
		return domain.Calibration{}, err
	}
	return list[0], nil
}

func (s *Store) Effective(ctx context.Context, testID string) ([]application.Effective, error) {
	if !pgtx.ValidUUID(testID) {
		return nil, nil
	}
	rows, err := s.queries.ListEffective(ctx, testID)
	if err != nil {
		return nil, fmt.Errorf("list effective calibrations: %w", err)
	}
	out := make([]application.Effective, 0, len(rows))
	for _, row := range rows {
		subject, err := toSubjectRef(sqlcgen.GetTestSubjectRefRow{ID: row.TestSubjectID, TestID: testID, EnvironmentRevisionID: row.EnvironmentRevisionID,
			ParadigmKey: row.ParadigmKey, ParadigmVersion: row.ParadigmVersion, Parameters: row.Parameters})
		if err != nil {
			return nil, err
		}
		out = append(out, application.Effective{Subject: subject, RecordingID: value(row.RecordingID), VideoStatus: value(row.VideoStatus),
			Source: domain.Source(row.Source), CalibrationID: row.CalibrationID})
	}
	return out, nil
}

// toCalibrations converts rows and reads their zones in one query.
func (s *Store) toCalibrations(ctx context.Context, rows []sqlcgen.MiskoCalibration) ([]domain.Calibration, error) {
	out := make([]domain.Calibration, 0, len(rows))
	ids := make([]string, 0, len(rows))
	for _, row := range rows {
		c, err := toCalibration(row)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
		ids = append(ids, c.ID)
	}
	if len(ids) == 0 {
		return out, nil
	}
	zones, err := s.queries.ListCalibrationZones(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("list calibration zones: %w", err)
	}
	byCalibration := map[string][]domain.Zone{}
	for _, z := range zones {
		zone, err := toZone(z)
		if err != nil {
			return nil, err
		}
		byCalibration[z.CalibrationID] = append(byCalibration[z.CalibrationID], zone)
	}
	for i := range out {
		out[i].Zones = byCalibration[out[i].ID]
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
		EnvironmentRevisionID: c.EnvironmentRevisionID, TestSubjectID: optional(c.TestSubjectID), RecordingID: optional(c.RecordingID),
		SupersedesID: optional(c.SupersedesID), CameraID: c.CameraID,
		FrameWidth: int32(c.FrameWidth), FrameHeight: int32(c.FrameHeight),
		CropX: int32(c.Crop.X), CropY: int32(c.Crop.Y), CropWidth: int32(c.Crop.Width), CropHeight: int32(c.Crop.Height),
		ReferenceFrameUs: referenceFrame(c), MeasurementPlane: string(c.Plane), FitPoints: fit, CheckPoints: check, Transform: c.Transform[:],
		FitRmsErrorCm: c.FitRMSErrorCm, CheckRmsErrorCm: c.CheckRMSErrorCm, CheckMaxErrorCm: c.CheckMaxErrorCm, ToleranceCm: c.ToleranceCm,
		AlgorithmVersion: c.AlgorithmVersion, Status: string(c.Status), RejectionReason: optional(c.RejectionReason), CreatedBy: c.CreatedBy,
	})
	if err != nil {
		return domain.Calibration{}, translate(err, nil, "create calibration")
	}
	created, err := toCalibration(row)
	if err != nil {
		return domain.Calibration{}, err
	}
	positions := map[string]int32{}
	for _, z := range c.Zones {
		kind, pixels, err := encodeShape(z.Pixels)
		if err != nil {
			return domain.Calibration{}, err
		}
		_, centimeters, err := encodeShape(z.Centimeters)
		if err != nil {
			return domain.Calibration{}, err
		}
		if err := s.queries.CreateCalibrationZone(ctx, sqlcgen.CreateCalibrationZoneParams{CalibrationID: created.ID, ZoneKey: z.Key,
			Position: positions[z.Key], Kind: kind, Pixels: pixels, Centimeters: centimeters}); err != nil {
			return domain.Calibration{}, translate(err, nil, "create calibration zone")
		}
		positions[z.Key]++
		created.Zones = append(created.Zones, z)
	}
	return created, nil
}

type vertexJSON struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

type circleJSON struct {
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Radius float64 `json:"radius"`
}

// encodeShape stores a circle as {"x", "y", "radius"} and a polygon as an
// array of {"x", "y"} vertices.
func encodeShape(s domain.Shape) (string, []byte, error) {
	var kind string
	var v any
	if s.Circle != nil {
		kind, v = "CIRCLE", circleJSON(*s.Circle)
	} else {
		vertices := make([]vertexJSON, 0, len(s.Polygon))
		for _, p := range s.Polygon {
			vertices = append(vertices, vertexJSON(p))
		}
		kind, v = "POLYGON", vertices
	}
	raw, err := json.Marshal(v)
	if err != nil {
		return "", nil, fmt.Errorf("encode zone shape: %w", err)
	}
	return kind, raw, nil
}

func decodeShape(kind string, raw []byte) (domain.Shape, error) {
	if kind == "CIRCLE" {
		var c circleJSON
		if err := json.Unmarshal(raw, &c); err != nil {
			return domain.Shape{}, fmt.Errorf("decode zone circle: %w", err)
		}
		circle := domain.Circle(c)
		return domain.Shape{Circle: &circle}, nil
	}
	var vertices []vertexJSON
	if err := json.Unmarshal(raw, &vertices); err != nil {
		return domain.Shape{}, fmt.Errorf("decode zone polygon: %w", err)
	}
	out := domain.Shape{Polygon: make([]domain.Vertex, 0, len(vertices))}
	for _, v := range vertices {
		out.Polygon = append(out.Polygon, domain.Vertex(v))
	}
	return out, nil
}

func toZone(r sqlcgen.MiskoCalibrationZone) (domain.Zone, error) {
	pixels, err := decodeShape(r.Kind, r.Pixels)
	if err != nil {
		return domain.Zone{}, err
	}
	centimeters, err := decodeShape(r.Kind, r.Centimeters)
	if err != nil {
		return domain.Zone{}, err
	}
	return domain.Zone{Key: r.ZoneKey, Pixels: pixels, Centimeters: centimeters}, nil
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

func decodeParameters(raw []byte) (map[string]float64, error) {
	parameters := map[string]float64{}
	if err := json.Unmarshal(raw, &parameters); err != nil {
		return nil, fmt.Errorf("decode environment parameters: %w", err)
	}
	return parameters, nil
}

func toSubjectRef(r sqlcgen.GetTestSubjectRefRow) (application.TestSubjectRef, error) {
	parameters, err := decodeParameters(r.Parameters)
	if err != nil {
		return application.TestSubjectRef{}, err
	}
	return application.TestSubjectRef{ID: r.ID, TestID: r.TestID, EnvironmentRevisionID: r.EnvironmentRevisionID, ParadigmKey: r.ParadigmKey,
		ParadigmVersion: int(r.ParadigmVersion), Parameters: parameters}, nil
}

func toRevisionRef(r sqlcgen.GetRevisionRefRow) (application.RevisionRef, error) {
	parameters, err := decodeParameters(r.Parameters)
	if err != nil {
		return application.RevisionRef{}, err
	}
	return application.RevisionRef{ID: r.ID, EnvironmentID: r.EnvironmentID, ParadigmKey: r.ParadigmKey, ParadigmVersion: int(r.ParadigmVersion), Parameters: parameters}, nil
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
		ID: r.ID, TestSubjectID: value(r.TestSubjectID), RecordingID: value(r.RecordingID), EnvironmentRevisionID: r.EnvironmentRevisionID, SupersedesID: value(r.SupersedesID), CameraID: r.CameraID,
		FrameWidth: int(r.FrameWidth), FrameHeight: int(r.FrameHeight),
		Crop:             domain.Rect{X: int(r.CropX), Y: int(r.CropY), Width: int(r.CropWidth), Height: int(r.CropHeight)},
		ReferenceFrameUs: int64Value(r.ReferenceFrameUs), Plane: domain.Plane(r.MeasurementPlane), Fit: fit, Check: check,
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

// referenceFrame stores the video time only for overrides, which have a video.
func referenceFrame(c domain.Calibration) *int64 {
	if c.RecordingID == "" {
		return nil
	}
	return &c.ReferenceFrameUs
}

func int64Value(v *int64) int64 {
	if v == nil {
		return 0
	}
	return *v
}
