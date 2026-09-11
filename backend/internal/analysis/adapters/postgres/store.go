// Package postgres implements analysis persistence with sqlc-generated queries.
// The runs table is the job queue: claims use FOR UPDATE SKIP LOCKED.
package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/HappyHackingSpace/Misko/backend/internal/analysis/adapters/postgres/sqlcgen"
	"github.com/HappyHackingSpace/Misko/backend/internal/analysis/application"
	"github.com/HappyHackingSpace/Misko/backend/internal/analysis/domain"
	"github.com/HappyHackingSpace/Misko/backend/internal/platform/pgtx"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"maps"
	"strconv"
	"time"
)

// constraintErrors maps named constraints to the error a violation means.
var constraintErrors = map[string]error{
	"analysis_workers_name_key":    application.ErrWorkerNameTaken,
	"analysis_runs_transition":     domain.ErrStaleAttempt,
	"analysis_outputs_fenced":      domain.ErrStaleAttempt,
	"analysis_output_uploads_pkey": application.ErrInvalidOutput,
	"analysis_events_trial_fkey":   domain.ErrUnknownTrial,
	"analysis_runs_recording_fkey": application.ErrRecordingNotFound,
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
		out = domain.Worker{ID: row.ID, Name: row.Name, ModelVersion: row.ModelVersion, DisabledAt: utc(row.DisabledAt), CreatedBy: row.CreatedBy,
			CreatedAt: row.CreatedAt.UTC(), Capabilities: append([]domain.Capability{}, w.Capabilities...)}
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
		out = append(out, domain.Worker{ID: r.ID, Name: r.Name, ModelVersion: r.ModelVersion, DisabledAt: utc(r.DisabledAt), CreatedBy: r.CreatedBy,
			CreatedAt: r.CreatedAt.UTC(), Capabilities: byWorker[r.ID]})
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
	return domain.Worker{ID: row.ID, Name: row.Name, ModelVersion: row.ModelVersion, DisabledAt: utc(row.DisabledAt), CreatedBy: row.CreatedBy,
		CreatedAt: row.CreatedAt.UTC(), Capabilities: caps}, nil
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
	return domain.Worker{ID: row.ID, Name: row.Name, ModelVersion: row.ModelVersion, CreatedBy: row.CreatedBy, CreatedAt: row.CreatedAt.UTC(), Capabilities: caps}, nil
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

func (s *Store) ReadyRecordings(ctx context.Context) ([]application.Candidate, error) {
	rows, err := s.queries.ReadyRecordings(ctx)
	if err != nil {
		return nil, fmt.Errorf("ready recordings: %w", err)
	}
	out := make([]application.Candidate, 0, len(rows))
	for _, r := range rows {
		params, err := parameters(r.Apparatus, r.Session)
		if err != nil {
			return nil, err
		}
		out = append(out, application.Candidate{
			ExperimentID: r.ExperimentID, TestID: r.TestID, RecordingID: r.RecordingID, SourceAssetID: r.SourceAssetID,
			SourceGeneration: r.SourceGeneration, SourceCRC32C: uint32(r.SourceCrc32c), ClipStartUs: r.ClipStartUs, ClipEndUs: r.ClipEndUs,
			CalibrationID: r.CalibrationID, ParadigmKey: r.ParadigmKey, ParadigmVersion: int(r.ParadigmVersion),
			EnvironmentRevisionID: r.EnvironmentRevisionID, ProtocolVersionID: r.ProtocolVersionID, Parameters: params, VideoVerified: true,
		})
	}
	return out, nil
}

func (s *Store) Candidate(ctx context.Context, testID, recordingID string) (application.Candidate, error) {
	if !pgtx.ValidUUID(testID) || !pgtx.ValidUUID(recordingID) {
		return application.Candidate{}, application.ErrRecordingNotFound
	}
	r, err := s.queries.GetCandidate(ctx, sqlcgen.GetCandidateParams{TestID: testID, ID: recordingID})
	if err != nil {
		return application.Candidate{}, translate(err, application.ErrRecordingNotFound, "get candidate")
	}
	params, err := parameters(r.Apparatus, r.Session)
	if err != nil {
		return application.Candidate{}, err
	}
	return application.Candidate{
		ExperimentID: r.ExperimentID, TestID: r.TestID, RecordingID: r.RecordingID, SourceAssetID: r.SourceAssetID,
		SourceGeneration: r.SourceGeneration, SourceCRC32C: uint32(r.SourceCrc32c), ClipStartUs: r.ClipStartUs, ClipEndUs: r.ClipEndUs,
		CalibrationID: r.CalibrationID, ParadigmKey: r.ParadigmKey, ParadigmVersion: int(r.ParadigmVersion),
		EnvironmentRevisionID: r.EnvironmentRevisionID, ProtocolVersionID: r.ProtocolVersionID, Parameters: params,
		VideoVerified: r.VideoStatus == "VERIFIED" && r.TestStatus != "CANCELLED",
	}, nil
}

func (s *Store) CreateAutomaticRun(ctx context.Context, r domain.Run) (bool, error) {
	params, err := json.Marshal(nonNil(r.Parameters))
	if err != nil {
		return false, fmt.Errorf("encode parameters: %w", err)
	}
	n, err := s.queries.CreateAutomaticRun(ctx, sqlcgen.CreateAutomaticRunParams{
		ExperimentID: r.ExperimentID, TestID: r.TestID, RecordingID: r.RecordingID, SourceAssetID: r.SourceAssetID,
		SourceGeneration: r.SourceGeneration, SourceCrc32c: int64(r.SourceCRC32C), ClipStartUs: r.ClipStartUs, ClipEndUs: r.ClipEndUs,
		CalibrationID: optional(r.CalibrationID), ParadigmKey: r.ParadigmKey, ParadigmVersion: int32(r.ParadigmVersion),
		MetricEngineVersion: int32(r.MetricEngineVersion), ResultSchemaVersion: int32(r.ResultSchemaVersion),
		EnvironmentRevisionID: r.EnvironmentRevisionID, ProtocolVersionID: r.ProtocolVersionID, Parameters: params,
		MaxAttempts: int32(r.MaxAttempts), AvailableAt: r.AvailableAt,
	})
	if err != nil {
		return false, translate(err, nil, "create automatic run")
	}
	return n == 1, nil
}

func (s *Store) CreateRun(ctx context.Context, r domain.Run) (domain.Run, error) {
	params, err := json.Marshal(nonNil(r.Parameters))
	if err != nil {
		return domain.Run{}, fmt.Errorf("encode parameters: %w", err)
	}
	row, err := s.queries.CreateManualRun(ctx, sqlcgen.CreateManualRunParams{
		ExperimentID: r.ExperimentID, TestID: r.TestID, RecordingID: r.RecordingID, SourceAssetID: r.SourceAssetID,
		SourceGeneration: r.SourceGeneration, SourceCrc32c: int64(r.SourceCRC32C), ClipStartUs: r.ClipStartUs, ClipEndUs: r.ClipEndUs,
		CalibrationID: optional(r.CalibrationID), ParadigmKey: r.ParadigmKey, ParadigmVersion: int32(r.ParadigmVersion),
		MetricEngineVersion: int32(r.MetricEngineVersion), ResultSchemaVersion: int32(r.ResultSchemaVersion),
		EnvironmentRevisionID: r.EnvironmentRevisionID, ProtocolVersionID: r.ProtocolVersionID, Parameters: params,
		MaxAttempts: int32(r.MaxAttempts), AvailableAt: r.AvailableAt, CreatedBy: optional(r.CreatedBy),
	})
	if err != nil {
		return domain.Run{}, translate(err, nil, "create run")
	}
	return toRun(row)
}

func (s *Store) LockExpiredRuns(ctx context.Context, now time.Time) ([]domain.Run, error) {
	rows, err := s.queries.LockExpiredRuns(ctx, now)
	if err != nil {
		return nil, fmt.Errorf("lock expired runs: %w", err)
	}
	return toRuns(rows)
}

func (s *Store) ClaimNext(ctx context.Context, capabilities []domain.Capability, now time.Time) (domain.Run, error) {
	keys := make([]string, 0, len(capabilities))
	for _, c := range capabilities {
		keys = append(keys, c.ParadigmKey+"@"+strconv.Itoa(c.ParadigmVersion))
	}
	row, err := s.queries.ClaimNext(ctx, sqlcgen.ClaimNextParams{Now: now, Capabilities: keys})
	if err != nil {
		return domain.Run{}, translate(err, application.ErrNoRun, "claim run")
	}
	return toRun(row)
}

func (s *Store) UpdateRun(ctx context.Context, r domain.Run) (domain.Run, error) {
	row, err := s.queries.UpdateRun(ctx, sqlcgen.UpdateRunParams{
		ID: r.ID, Status: string(r.Status), Attempt: int32(r.Attempt), WorkerID: optional(r.WorkerID), LeaseExpiresAt: r.LeaseExpiresAt,
		AvailableAt: r.AvailableAt, ModelVersion: optional(r.ModelVersion), FailureReason: optional(r.FailureReason), FinishedAt: r.FinishedAt,
	})
	if err != nil {
		return domain.Run{}, translate(err, application.ErrRunNotFound, "update run")
	}
	return toRun(row)
}

func (s *Store) Run(ctx context.Context, id string) (domain.Run, error) {
	if !pgtx.ValidUUID(id) {
		return domain.Run{}, application.ErrRunNotFound
	}
	row, err := s.queries.GetRun(ctx, id)
	if err != nil {
		return domain.Run{}, translate(err, application.ErrRunNotFound, "get run")
	}
	return toRun(row)
}

func (s *Store) LockRun(ctx context.Context, id string) (domain.Run, error) {
	if !pgtx.ValidUUID(id) {
		return domain.Run{}, application.ErrRunNotFound
	}
	row, err := s.queries.LockRun(ctx, id)
	if err != nil {
		return domain.Run{}, translate(err, application.ErrRunNotFound, "lock run")
	}
	return toRun(row)
}

func (s *Store) Runs(ctx context.Context, testID string) ([]domain.Run, error) {
	if !pgtx.ValidUUID(testID) {
		return []domain.Run{}, nil
	}
	rows, err := s.queries.ListRuns(ctx, testID)
	if err != nil {
		return nil, fmt.Errorf("list runs: %w", err)
	}
	return toRuns(rows)
}

func (s *Store) TrialIDs(ctx context.Context, testID string) ([]string, error) {
	ids, err := s.queries.TrialIDs(ctx, testID)
	if err != nil {
		return nil, fmt.Errorf("trial ids: %w", err)
	}
	return ids, nil
}

func (s *Store) SourceObject(ctx context.Context, assetID string) (string, error) {
	name, err := s.queries.SourceObject(ctx, assetID)
	if err != nil {
		return "", fmt.Errorf("source object: %w", err)
	}
	return name, nil
}

func (s *Store) CreateOutputUpload(ctx context.Context, u application.OutputUpload) error {
	err := s.queries.CreateOutputUpload(ctx, sqlcgen.CreateOutputUploadParams{
		RunID: u.RunID, Attempt: int32(u.Attempt), ObjectName: u.ObjectName, Kind: u.Kind, ContentType: u.ContentType,
		SizeBytes: u.SizeBytes, Crc32c: int64(u.CRC32C),
	})
	if err != nil {
		return translate(err, nil, "create output upload")
	}
	return nil
}

func (s *Store) OutputUploads(ctx context.Context, runID string, attempt int) ([]application.OutputUpload, error) {
	rows, err := s.queries.OutputUploads(ctx, sqlcgen.OutputUploadsParams{RunID: runID, Attempt: int32(attempt)})
	if err != nil {
		return nil, fmt.Errorf("output uploads: %w", err)
	}
	out := make([]application.OutputUpload, 0, len(rows))
	for _, r := range rows {
		out = append(out, application.OutputUpload{RunID: r.RunID, Attempt: int(r.Attempt), ObjectName: r.ObjectName, Kind: r.Kind,
			ContentType: r.ContentType, SizeBytes: r.SizeBytes, CRC32C: uint32(r.Crc32c)})
	}
	return out, nil
}

// Publish must run in the transaction that marks the run SUCCEEDED; a deferred
// constraint rejects a success without its video pair.
func (s *Store) Publish(ctx context.Context, run domain.Run, result domain.Result, outputs []application.VerifiedOutput) error {
	attempt := int32(run.Attempt)
	analyzed := ""
	for _, o := range outputs {
		var assetID *string
		if o.Kind == domain.AnalyzedVideo {
			generation := o.Generation
			id, err := s.queries.CreateAnalyzedAsset(ctx, sqlcgen.CreateAnalyzedAssetParams{
				Bucket: o.Bucket, ObjectName: o.ObjectName, ContentType: o.ContentType, SizeBytes: o.SizeBytes, Crc32c: int64(o.CRC32C),
				Generation: &generation, VerifiedAt: run.FinishedAt, CreatedBy: run.WorkerID,
			})
			if err != nil {
				return translate(err, nil, "create analyzed video")
			}
			analyzed, assetID = id, &id
		}
		err := s.queries.CreateArtifact(ctx, sqlcgen.CreateArtifactParams{
			RunID: run.ID, Attempt: attempt, ObjectName: o.ObjectName, Kind: o.Kind, Bucket: o.Bucket, Generation: o.Generation,
			SizeBytes: o.SizeBytes, Crc32c: int64(o.CRC32C), ContentType: o.ContentType, VideoAssetID: assetID,
		})
		if err != nil {
			return translate(err, nil, "create artifact")
		}
	}
	err := s.queries.CreatePair(ctx, sqlcgen.CreatePairParams{
		RunID: run.ID, Attempt: attempt, SourceAssetID: run.SourceAssetID, SourceGeneration: run.SourceGeneration, AnalyzedAssetID: analyzed,
		SourceOffsetUs: result.Pair.SourceOffsetUs, OutputOffsetUs: result.Pair.OutputOffsetUs, TimeMappingVersion: result.Pair.TimeMappingVersion,
	})
	if err != nil {
		return translate(err, nil, "create video pair")
	}
	for _, m := range result.Metrics {
		err := s.queries.CreateMetric(ctx, sqlcgen.CreateMetricParams{
			RunID: run.ID, Attempt: attempt, MetricKey: m.Key, Unit: m.Unit, Value: m.Value, MissingReason: optional(m.MissingReason),
		})
		if err != nil {
			return translate(err, nil, "create metric")
		}
	}
	for _, e := range result.Events {
		err := s.queries.CreateEvent(ctx, sqlcgen.CreateEventParams{
			RunID: run.ID, Attempt: attempt, TestID: run.TestID, EventType: e.Type, Kind: e.Kind, StartUs: e.StartUs, EndUs: e.EndUs,
			Confidence: float32(e.Confidence), TrialID: optional(e.TrialID),
		})
		if err != nil {
			return translate(err, nil, "create event")
		}
	}
	return nil
}

func (s *Store) Published(ctx context.Context, runID string) (application.Published, error) {
	var out application.Published
	metrics, err := s.queries.ListMetrics(ctx, runID)
	if err != nil {
		return out, fmt.Errorf("list metrics: %w", err)
	}
	for _, m := range metrics {
		out.Metrics = append(out.Metrics, domain.MetricValue{Key: m.MetricKey, Unit: m.Unit, Value: m.Value, MissingReason: value(m.MissingReason)})
	}
	events, err := s.queries.ListEvents(ctx, runID)
	if err != nil {
		return out, fmt.Errorf("list events: %w", err)
	}
	for _, e := range events {
		out.Events = append(out.Events, domain.Event{Type: e.EventType, Kind: e.Kind, StartUs: e.StartUs, EndUs: e.EndUs, Confidence: float64(e.Confidence), TrialID: value(e.TrialID)})
	}
	artifacts, err := s.queries.ListArtifacts(ctx, runID)
	if err != nil {
		return out, fmt.Errorf("list artifacts: %w", err)
	}
	for _, a := range artifacts {
		out.Artifacts = append(out.Artifacts, application.VerifiedOutput{
			OutputUpload: application.OutputUpload{RunID: a.RunID, Attempt: int(a.Attempt), ObjectName: a.ObjectName, Kind: a.Kind,
				ContentType: a.ContentType, SizeBytes: a.SizeBytes, CRC32C: uint32(a.Crc32c)},
			Bucket: a.Bucket, Generation: a.Generation,
		})
	}
	pair, err := s.queries.GetPair(ctx, runID)
	if errors.Is(err, pgx.ErrNoRows) {
		return out, nil
	}
	if err != nil {
		return out, fmt.Errorf("get pair: %w", err)
	}
	p := application.Pair{
		SourceAssetID: pair.SourceAssetID, SourceGeneration: pair.SourceGeneration, SourceObjectName: pair.SourceObjectName,
		AnalyzedAssetID: pair.AnalyzedAssetID, AnalyzedObjectName: pair.AnalyzedObjectName, SourceOffsetUs: pair.SourceOffsetUs,
		OutputOffsetUs: pair.OutputOffsetUs, TimeMappingVersion: pair.TimeMappingVersion,
	}
	if pair.AnalyzedGeneration != nil {
		p.AnalyzedGeneration = *pair.AnalyzedGeneration
	}
	out.Pair = &p
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

func toRuns(rows []sqlcgen.MiskoAnalysisRun) ([]domain.Run, error) {
	out := make([]domain.Run, 0, len(rows))
	for _, row := range rows {
		r, err := toRun(row)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, nil
}

func toRun(r sqlcgen.MiskoAnalysisRun) (domain.Run, error) {
	params := map[string]float64{}
	if err := json.Unmarshal(r.Parameters, &params); err != nil {
		return domain.Run{}, fmt.Errorf("decode run parameters: %w", err)
	}
	return domain.Run{
		ID: r.ID,
		Source: domain.Source{
			ExperimentID: r.ExperimentID, TestID: r.TestID, RecordingID: r.RecordingID, SourceAssetID: r.SourceAssetID,
			SourceGeneration: r.SourceGeneration, SourceCRC32C: uint32(r.SourceCrc32c), ClipStartUs: r.ClipStartUs, ClipEndUs: r.ClipEndUs,
			CalibrationID: value(r.CalibrationID), ParadigmKey: r.ParadigmKey, ParadigmVersion: int(r.ParadigmVersion),
			EnvironmentRevisionID: r.EnvironmentRevisionID, ProtocolVersionID: r.ProtocolVersionID, Parameters: params,
		},
		MetricEngineVersion: int(r.MetricEngineVersion), ResultSchemaVersion: int(r.ResultSchemaVersion), Trigger: domain.Trigger(r.Trigger),
		Status: domain.Status(r.Status), Attempt: int(r.Attempt), MaxAttempts: int(r.MaxAttempts), WorkerID: value(r.WorkerID),
		LeaseExpiresAt: utc(r.LeaseExpiresAt), AvailableAt: r.AvailableAt.UTC(), ModelVersion: value(r.ModelVersion),
		FailureReason: value(r.FailureReason), CreatedBy: value(r.CreatedBy), CreatedAt: r.CreatedAt.UTC(), FinishedAt: utc(r.FinishedAt),
	}, nil
}

// parameters snapshots the environment's apparatus and the step's session values.
func parameters(apparatus, session []byte) (map[string]float64, error) {
	out := map[string]float64{}
	if err := json.Unmarshal(apparatus, &out); err != nil {
		return nil, fmt.Errorf("decode apparatus: %w", err)
	}
	s := map[string]float64{}
	if err := json.Unmarshal(session, &s); err != nil {
		return nil, fmt.Errorf("decode session: %w", err)
	}
	maps.Copy(out, s)
	return out, nil
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
