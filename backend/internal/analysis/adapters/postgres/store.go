// Package postgres implements analysis persistence with sqlc-generated
// queries. The jobs table is the queue: claims use FOR UPDATE SKIP LOCKED.
package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/HappyHackingSpace/Misko/backend/internal/analysis/adapters/postgres/sqlcgen"
	"github.com/HappyHackingSpace/Misko/backend/internal/analysis/application"
	"github.com/HappyHackingSpace/Misko/backend/internal/analysis/domain"
	"github.com/HappyHackingSpace/Misko/backend/internal/platform/pgtx"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// constraintErrors maps named constraints and trigger rules to the error a
// violation means, so the API answers with a code instead of a 500.
var constraintErrors = map[string]error{
	"analysis_workers_name_key":           application.ErrWorkerNameTaken,
	"analysis_jobs_transition":            domain.ErrStaleAttempt,
	"analysis_jobs_attempts_check":        domain.ErrStaleAttempt,
	"analysis_job_outputs_fenced":         domain.ErrStaleAttempt,
	"analysis_job_uploads_pkey":           application.ErrInvalidOutput,
	"analysis_jobs_source_asset_id_fkey":  application.ErrSourceNotFound,
	"analysis_jobs_calibration_check":     domain.ErrInvalidCalibration,
	"analysis_jobs_clip_check":            domain.ErrInvalidRequest,
	"analysis_jobs_requester_kind_check":  domain.ErrInvalidRequest,
	"analysis_jobs_paradigm_key_check":    domain.ErrInvalidRequest,
	"analysis_jobs_idempotency_key_check": domain.ErrInvalidRequest,
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

func (s *Store) CreateWorker(ctx context.Context, w domain.Worker, tokenHash []byte) (domain.Worker, error) {
	var out domain.Worker
	err := s.Transaction(ctx, func(tx application.Store) error {
		q := tx.(*Store).queries
		row, err := q.CreateWorker(ctx, sqlcgen.CreateWorkerParams{Name: w.Name, ModelVersion: w.ModelVersion, TokenSha256: tokenHash, CreatedBy: w.CreatedBy})
		if err != nil {
			return translate(err, nil, "create worker")
		}
		for _, c := range w.Capabilities {
			if err := q.AddCapability(ctx, sqlcgen.AddCapabilityParams{WorkerID: row.ID, ParadigmKey: c.ParadigmKey, ParadigmVersion: int32(c.ParadigmVersion)}); err != nil {
				return translate(err, nil, "add capability")
			}
		}
		out = toWorker(sqlcgen.ListWorkersRow(row), append([]domain.Capability{}, w.Capabilities...))
		return nil
	})
	return out, err
}

func (s *Store) Workers(ctx context.Context) ([]domain.Worker, error) {
	rows, err := s.queries.ListWorkers(ctx)
	if err != nil {
		return nil, fmt.Errorf("list workers: %w", err)
	}
	caps, err := s.queries.ListCapabilities(ctx)
	if err != nil {
		return nil, fmt.Errorf("list capabilities: %w", err)
	}
	byWorker := map[string][]domain.Capability{}
	for _, c := range caps {
		byWorker[c.WorkerID] = append(byWorker[c.WorkerID], domain.Capability{ParadigmKey: c.ParadigmKey, ParadigmVersion: int(c.ParadigmVersion)})
	}
	out := make([]domain.Worker, 0, len(rows))
	for _, r := range rows {
		out = append(out, toWorker(r, byWorker[r.ID]))
	}
	return out, nil
}

func (s *Store) DisableWorker(ctx context.Context, id string, at time.Time) (domain.Worker, error) {
	if !pgtx.ValidUUID(id) {
		return domain.Worker{}, application.ErrWorkerNotFound
	}
	row, err := s.queries.DisableWorker(ctx, sqlcgen.DisableWorkerParams{ID: id, At: at})
	if err != nil {
		return domain.Worker{}, translate(err, application.ErrWorkerNotFound, "disable worker")
	}
	caps, err := s.capabilities(ctx, row.ID)
	if err != nil {
		return domain.Worker{}, err
	}
	return toWorker(sqlcgen.ListWorkersRow(row), caps), nil
}

func (s *Store) WorkerByToken(ctx context.Context, tokenHash []byte) (domain.Worker, error) {
	row, err := s.queries.WorkerByToken(ctx, tokenHash)
	if err != nil {
		return domain.Worker{}, translate(err, application.ErrWorkerUnauthenticated, "authenticate worker")
	}
	caps, err := s.capabilities(ctx, row.ID)
	if err != nil {
		return domain.Worker{}, err
	}
	return toWorker(sqlcgen.ListWorkersRow(row), caps), nil
}

func toWorker(r sqlcgen.ListWorkersRow, caps []domain.Capability) domain.Worker {
	return domain.Worker{ID: r.ID, Name: r.Name, ModelVersion: r.ModelVersion, DisabledAt: utc(r.DisabledAt), CreatedBy: r.CreatedBy,
		CreatedAt: r.CreatedAt.UTC(), Capabilities: caps}
}

func (s *Store) capabilities(ctx context.Context, workerID string) ([]domain.Capability, error) {
	rows, err := s.queries.WorkerCapabilities(ctx, workerID)
	if err != nil {
		return nil, fmt.Errorf("worker capabilities: %w", err)
	}
	out := make([]domain.Capability, 0, len(rows))
	for _, r := range rows {
		out = append(out, domain.Capability{ParadigmKey: r.ParadigmKey, ParadigmVersion: int(r.ParadigmVersion)})
	}
	return out, nil
}

func (s *Store) ActiveCapabilities(ctx context.Context) ([]domain.Capability, error) {
	rows, err := s.queries.ActiveCapabilities(ctx)
	if err != nil {
		return nil, fmt.Errorf("active capabilities: %w", err)
	}
	out := make([]domain.Capability, 0, len(rows))
	for _, r := range rows {
		out = append(out, domain.Capability{ParadigmKey: r.ParadigmKey, ParadigmVersion: int(r.ParadigmVersion)})
	}
	return out, nil
}

func capabilityKeys(capabilities []domain.Capability) []string {
	keys := make([]string, 0, len(capabilities))
	for _, c := range capabilities {
		keys = append(keys, c.ParadigmKey+"@"+strconv.Itoa(c.ParadigmVersion))
	}
	return keys
}

// CreateJob inserts the job or, when its idempotency key is taken, returns
// the job that holds it.
func (s *Store) CreateJob(ctx context.Context, j domain.Job) (domain.Job, error) {
	params, err := jobParams(j)
	if err != nil {
		return domain.Job{}, err
	}
	rows, err := s.queries.CreateJob(ctx, params)
	if err != nil {
		return domain.Job{}, translate(err, nil, "create job")
	}
	if len(rows) == 1 {
		return toJob(rows[0])
	}
	if j.IdempotencyKey == "" {
		return domain.Job{}, errors.New("create job: nothing was inserted")
	}
	row, err := s.queries.JobByIdempotencyKey(ctx, &j.IdempotencyKey)
	if err != nil {
		return domain.Job{}, fmt.Errorf("read job by idempotency key: %w", err)
	}
	return toJob(row)
}

func jobParams(j domain.Job) (sqlcgen.CreateJobParams, error) {
	params, err := json.Marshal(nonNil(j.Parameters))
	if err != nil {
		return sqlcgen.CreateJobParams{}, fmt.Errorf("encode parameters: %w", err)
	}
	p := sqlcgen.CreateJobParams{
		RequesterKind: string(j.Requester.Kind), RequesterID: j.Requester.ID, IdempotencyKey: optional(j.IdempotencyKey),
		ParadigmKey: j.ParadigmKey, ParadigmVersion: int32(j.ParadigmVersion), MetricEngineVersion: int32(j.MetricEngineVersion),
		ResultSchemaVersion: int32(j.ResultSchemaVersion), Parameters: params, SourceAssetID: j.Source.AssetID,
		SourceGeneration: j.Source.Generation, SourceCrc32c: int64(j.Source.CRC32C), ClipStartUs: j.Source.ClipStartUs, ClipEndUs: j.Source.ClipEndUs,
		Trigger: string(j.Trigger), MaxAttempts: int32(j.MaxAttempts), AvailableAt: j.AvailableAt, CreatedBy: optional(j.CreatedBy),
	}
	if c := j.Calibration; c != nil {
		zones, err := encodeZones(c.Zones)
		if err != nil {
			return sqlcgen.CreateJobParams{}, err
		}
		p.CalibrationID, p.CalibrationPlane, p.CalibrationZones = &c.ID, &c.Plane, zones
		p.CalibrationFrameWidth, p.CalibrationFrameHeight = int32p(c.FrameWidth), int32p(c.FrameHeight)
		p.CalibrationCropX, p.CalibrationCropY = int32p(c.Crop.X), int32p(c.Crop.Y)
		p.CalibrationCropWidth, p.CalibrationCropHeight = int32p(c.Crop.Width), int32p(c.Crop.Height)
		p.CalibrationTransform = append([]float64(nil), c.Transform[:]...)
	}
	return p, nil
}

func (s *Store) LockExpiredJobs(ctx context.Context, now time.Time) ([]domain.Job, error) {
	rows, err := s.queries.LockExpiredJobs(ctx, now)
	if err != nil {
		return nil, fmt.Errorf("lock expired jobs: %w", err)
	}
	return toJobs(rows)
}

func (s *Store) ClaimNext(ctx context.Context, capabilities []domain.Capability, now time.Time) (domain.Job, error) {
	row, err := s.queries.ClaimNext(ctx, sqlcgen.ClaimNextParams{Now: now, Capabilities: capabilityKeys(capabilities)})
	if err != nil {
		return domain.Job{}, translate(err, application.ErrNoJob, "claim job")
	}
	return toJob(row)
}

func (s *Store) UpdateJob(ctx context.Context, j domain.Job) (domain.Job, error) {
	row, err := s.queries.UpdateJob(ctx, sqlcgen.UpdateJobParams{
		ID: j.ID, Status: string(j.Status), Attempt: int32(j.Attempt), WorkerID: optional(j.WorkerID), LeaseExpiresAt: j.LeaseExpiresAt,
		AvailableAt: j.AvailableAt, ModelVersion: optional(j.ModelVersion), FailureReason: optional(j.FailureReason), FinishedAt: j.FinishedAt,
	})
	if err != nil {
		return domain.Job{}, translate(err, application.ErrJobNotFound, "update job")
	}
	return toJob(row)
}

func (s *Store) Job(ctx context.Context, id string) (domain.Job, error) {
	if !pgtx.ValidUUID(id) {
		return domain.Job{}, application.ErrJobNotFound
	}
	row, err := s.queries.GetJob(ctx, id)
	if err != nil {
		return domain.Job{}, translate(err, application.ErrJobNotFound, "get job")
	}
	return toJob(row)
}

func (s *Store) LockJob(ctx context.Context, id string) (domain.Job, error) {
	if !pgtx.ValidUUID(id) {
		return domain.Job{}, application.ErrJobNotFound
	}
	row, err := s.queries.LockJob(ctx, id)
	if err != nil {
		return domain.Job{}, translate(err, application.ErrJobNotFound, "lock job")
	}
	return toJob(row)
}

func (s *Store) Jobs(ctx context.Context, ids []string) ([]domain.Job, error) {
	rows, err := s.queries.ListJobs(ctx, validUUIDs(ids))
	if err != nil {
		return nil, fmt.Errorf("list jobs: %w", err)
	}
	return toJobs(rows)
}

func (s *Store) JobsByRequester(ctx context.Context, kind domain.RequesterKind, ids []string) ([]domain.Job, error) {
	rows, err := s.queries.ListJobsByRequester(ctx, sqlcgen.ListJobsByRequesterParams{RequesterKind: string(kind), RequesterIds: validUUIDs(ids)})
	if err != nil {
		return nil, fmt.Errorf("list jobs by requester: %w", err)
	}
	return toJobs(rows)
}

func (s *Store) CreateOutputUpload(ctx context.Context, u application.OutputUpload) error {
	err := s.queries.CreateOutputUpload(ctx, sqlcgen.CreateOutputUploadParams{
		JobID: u.JobID, Attempt: int32(u.Attempt), ObjectName: u.ObjectName, Kind: u.Kind, ContentType: u.ContentType,
		SizeBytes: u.SizeBytes, Crc32c: int64(u.CRC32C),
	})
	if err != nil {
		return translate(err, nil, "create output upload")
	}
	return nil
}

func (s *Store) OutputUploads(ctx context.Context, jobID string, attempt int) ([]application.OutputUpload, error) {
	rows, err := s.queries.OutputUploads(ctx, sqlcgen.OutputUploadsParams{JobID: jobID, Attempt: int32(attempt)})
	if err != nil {
		return nil, fmt.Errorf("output uploads: %w", err)
	}
	out := make([]application.OutputUpload, 0, len(rows))
	for _, r := range rows {
		out = append(out, application.OutputUpload{JobID: r.JobID, Attempt: int(r.Attempt), ObjectName: r.ObjectName, Kind: r.Kind,
			ContentType: r.ContentType, SizeBytes: r.SizeBytes, CRC32C: uint32(r.Crc32c)})
	}
	return out, nil
}

// Publish must run in the transaction that marks the job SUCCEEDED; a
// deferred constraint rejects a success without its video pair.
func (s *Store) Publish(ctx context.Context, j domain.Job, result domain.Result, outputs []application.VerifiedOutput, quality domain.Quality) error {
	attempt := int32(j.Attempt)
	for _, o := range outputs {
		err := s.queries.CreateOutput(ctx, sqlcgen.CreateOutputParams{
			JobID: j.ID, Attempt: attempt, ObjectName: o.ObjectName, Kind: o.Kind, Bucket: o.Bucket, Generation: o.Generation,
			SizeBytes: o.SizeBytes, Crc32c: int64(o.CRC32C), ContentType: o.ContentType,
		})
		if err != nil {
			return translate(err, nil, "create output")
		}
	}
	err := s.queries.CreatePair(ctx, sqlcgen.CreatePairParams{
		JobID: j.ID, Attempt: attempt, AnalyzedObjectName: result.Pair.AnalyzedObjectName, SourceOffsetUs: result.Pair.SourceOffsetUs,
		OutputOffsetUs: result.Pair.OutputOffsetUs, TimeMappingVersion: result.Pair.TimeMappingVersion,
	})
	if err != nil {
		return translate(err, nil, "create video pair")
	}
	for _, m := range result.Metrics {
		err := s.queries.CreateMetric(ctx, sqlcgen.CreateMetricParams{
			JobID: j.ID, Attempt: attempt, MetricKey: m.Key, Unit: m.Unit, Value: m.Value, MissingReason: optional(m.MissingReason),
		})
		if err != nil {
			return translate(err, nil, "create metric")
		}
	}
	for _, e := range result.Events {
		err := s.queries.CreateEvent(ctx, sqlcgen.CreateEventParams{
			JobID: j.ID, Attempt: attempt, EventType: e.Type, Label: optional(e.Label), Kind: e.Kind, StartUs: e.StartUs, EndUs: e.EndUs,
			Confidence: float32(e.Confidence),
		})
		if err != nil {
			return translate(err, nil, "create event")
		}
	}
	return s.RecordQC(ctx, j, application.QC{Quality: quality, Passed: true})
}

func (s *Store) RecordQC(ctx context.Context, j domain.Job, qc application.QC) error {
	err := s.queries.CreateQC(ctx, sqlcgen.CreateQCParams{
		JobID: j.ID, Attempt: int32(j.Attempt), Samples: int32(qc.Samples), Tracked: int32(qc.Tracked), Inferred: int32(qc.Inferred),
		LostRatio: qc.LostRatio, InferredRatio: qc.InferredRatio, MeanConfidence: qc.MeanConfidence, Passed: qc.Passed,
	})
	if err != nil {
		return translate(err, nil, "record QC")
	}
	return nil
}

// Published reads every stored result of the jobs with one query per kind.
func (s *Store) Published(ctx context.Context, jobIDs []string) (map[string]application.Published, error) {
	ids := validUUIDs(jobIDs)
	out := map[string]application.Published{}
	entry := func(id string) application.Published { return out[id] }
	metrics, err := s.queries.ListMetrics(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("list metrics: %w", err)
	}
	for _, m := range metrics {
		p := entry(m.JobID)
		p.Metrics = append(p.Metrics, domain.MetricValue{Key: m.MetricKey, Unit: m.Unit, Value: m.Value, MissingReason: value(m.MissingReason)})
		out[m.JobID] = p
	}
	events, err := s.queries.ListEvents(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("list events: %w", err)
	}
	for _, e := range events {
		p := entry(e.JobID)
		p.Events = append(p.Events, domain.Event{Type: e.EventType, Label: value(e.Label), Kind: e.Kind, StartUs: e.StartUs, EndUs: e.EndUs, Confidence: float64(e.Confidence)})
		out[e.JobID] = p
	}
	if err := s.addOutputs(ctx, ids, out); err != nil {
		return nil, err
	}
	qcs, err := s.queries.ListQC(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("list QC: %w", err)
	}
	for _, q := range qcs {
		p := entry(q.JobID)
		p.QC = &application.QC{Passed: q.Passed, Quality: domain.Quality{Samples: int(q.Samples), Tracked: int(q.Tracked), Inferred: int(q.Inferred),
			LostRatio: q.LostRatio, InferredRatio: q.InferredRatio, MeanConfidence: q.MeanConfidence}}
		out[q.JobID] = p
	}
	return out, nil
}

func (s *Store) addOutputs(ctx context.Context, ids []string, out map[string]application.Published) error {
	outputs, err := s.queries.ListOutputs(ctx, ids)
	if err != nil {
		return fmt.Errorf("list outputs: %w", err)
	}
	for _, o := range outputs {
		p := out[o.JobID]
		p.Artifacts = append(p.Artifacts, application.VerifiedOutput{
			OutputUpload: application.OutputUpload{JobID: o.JobID, Attempt: int(o.Attempt), ObjectName: o.ObjectName, Kind: o.Kind,
				ContentType: o.ContentType, SizeBytes: o.SizeBytes, CRC32C: uint32(o.Crc32c)},
			Bucket: o.Bucket, Generation: o.Generation,
		})
		out[o.JobID] = p
	}
	pairs, err := s.queries.ListPairs(ctx, ids)
	if err != nil {
		return fmt.Errorf("list pairs: %w", err)
	}
	for _, r := range pairs {
		p := out[r.JobID]
		p.Pair = &application.Pair{AnalyzedObjectName: r.AnalyzedObjectName, AnalyzedGeneration: r.AnalyzedGeneration, SourceOffsetUs: r.SourceOffsetUs,
			OutputOffsetUs: r.OutputOffsetUs, TimeMappingVersion: r.TimeMappingVersion}
		out[r.JobID] = p
	}
	return nil
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

func toJobs(rows []sqlcgen.MiskoAnalysisJob) ([]domain.Job, error) {
	out := make([]domain.Job, 0, len(rows))
	for _, row := range rows {
		j, err := toJob(row)
		if err != nil {
			return nil, err
		}
		out = append(out, j)
	}
	return out, nil
}

func toJob(r sqlcgen.MiskoAnalysisJob) (domain.Job, error) {
	params := map[string]float64{}
	if err := json.Unmarshal(r.Parameters, &params); err != nil {
		return domain.Job{}, fmt.Errorf("decode job parameters: %w", err)
	}
	calibration, err := toCalibration(r)
	if err != nil {
		return domain.Job{}, err
	}
	return domain.Job{
		ID: r.ID,
		Input: domain.Input{
			Requester: domain.Requester{Kind: domain.RequesterKind(r.RequesterKind), ID: r.RequesterID}, IdempotencyKey: value(r.IdempotencyKey),
			ParadigmKey: r.ParadigmKey, ParadigmVersion: int(r.ParadigmVersion), MetricEngineVersion: int(r.MetricEngineVersion),
			ResultSchemaVersion: int(r.ResultSchemaVersion), Parameters: params,
			Source: domain.Source{AssetID: r.SourceAssetID, Generation: r.SourceGeneration, CRC32C: uint32(r.SourceCrc32c), ClipStartUs: r.ClipStartUs,
				ClipEndUs: r.ClipEndUs},
			Calibration: calibration, Trigger: domain.Trigger(r.Trigger), CreatedBy: value(r.CreatedBy),
		},
		Status: domain.Status(r.Status), Attempt: int(r.Attempt), MaxAttempts: int(r.MaxAttempts), WorkerID: value(r.WorkerID),
		LeaseExpiresAt: utc(r.LeaseExpiresAt), AvailableAt: r.AvailableAt.UTC(), ModelVersion: value(r.ModelVersion),
		FailureReason: value(r.FailureReason), CreatedAt: r.CreatedAt.UTC(), FinishedAt: utc(r.FinishedAt),
	}, nil
}

// toCalibration reads the snapshot columns; the table guarantees they are all
// set or all NULL.
func toCalibration(r sqlcgen.MiskoAnalysisJob) (*domain.Calibration, error) {
	if r.CalibrationID == nil {
		return nil, nil
	}
	if len(r.CalibrationTransform) != 9 || r.CalibrationFrameWidth == nil || r.CalibrationFrameHeight == nil || r.CalibrationCropX == nil ||
		r.CalibrationCropY == nil || r.CalibrationCropWidth == nil || r.CalibrationCropHeight == nil {
		return nil, fmt.Errorf("job %s: incomplete calibration snapshot", r.ID)
	}
	zones, err := decodeZones(r.CalibrationZones)
	if err != nil {
		return nil, fmt.Errorf("job %s: %w", r.ID, err)
	}
	c := &domain.Calibration{
		ID: *r.CalibrationID, FrameWidth: int(*r.CalibrationFrameWidth), FrameHeight: int(*r.CalibrationFrameHeight),
		Crop:  domain.Rect{X: int(*r.CalibrationCropX), Y: int(*r.CalibrationCropY), Width: int(*r.CalibrationCropWidth), Height: int(*r.CalibrationCropHeight)},
		Plane: value(r.CalibrationPlane), Zones: zones,
	}
	copy(c.Transform[:], r.CalibrationTransform)
	return c, nil
}

func validUUIDs(ids []string) []string {
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if pgtx.ValidUUID(id) {
			out = append(out, id)
		}
	}
	return out
}

func nonNil(m map[string]float64) map[string]float64 {
	if m == nil {
		return map[string]float64{}
	}
	return m
}

func utc(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	u := t.UTC()
	return &u
}

func int32p(v int) *int32 {
	n := int32(v)
	return &n
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
