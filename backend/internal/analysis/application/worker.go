package application

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/HappyHackingSpace/Misko/backend/internal/analysis/domain"
)

// Claimed is a leased job with the contract of its paradigm version. The
// calibration a worker needs is the job's own snapshot.
type Claimed struct {
	Job      domain.Job
	Contract domain.Contract
}

// Claim leases the oldest queued job the worker can analyse.
func (s *Service) Claim(ctx context.Context, w domain.Worker) (Claimed, error) {
	if s.objects == nil {
		return Claimed{}, ErrStorageNotConfigured
	}
	now := s.clock()
	var out Claimed
	err := s.store.Transaction(ctx, func(tx Store) error {
		job, err := tx.ClaimNext(ctx, w.Capabilities, now)
		if err != nil {
			return err
		}
		claimed, err := job.Claim(w.ID, now, s.settings.Lease)
		if err != nil {
			return err
		}
		if out.Job, err = tx.UpdateJob(ctx, claimed); err != nil {
			return err
		}
		out.Contract, err = s.catalog.Contract(claimed.ParadigmKey, claimed.ParadigmVersion)
		return err
	})
	if err != nil {
		return Claimed{}, err
	}
	return out, nil
}

func (s *Service) Heartbeat(ctx context.Context, w domain.Worker, jobID string, attempt int) (domain.Job, error) {
	now := s.clock()
	var out domain.Job
	err := s.store.Transaction(ctx, func(tx Store) error {
		job, err := tx.LockJob(ctx, jobID)
		if err != nil {
			return err
		}
		beat, err := job.Heartbeat(w.ID, attempt, now, s.settings.Lease)
		if err != nil {
			return err
		}
		out, err = tx.UpdateJob(ctx, beat)
		return err
	})
	return out, err
}

// SourceURL signs a read of the job's pinned source generation for its worker.
func (s *Service) SourceURL(ctx context.Context, w domain.Worker, jobID string, attempt int) (string, time.Time, error) {
	if s.objects == nil {
		return "", time.Time{}, ErrStorageNotConfigured
	}
	job, err := s.store.Job(ctx, jobID)
	if err != nil {
		return "", time.Time{}, err
	}
	if err := job.CheckLease(w.ID, attempt); err != nil {
		return "", time.Time{}, err
	}
	video, err := s.sources.Source(ctx, job.Source.AssetID)
	if err != nil {
		return "", time.Time{}, err
	}
	expires := s.now().Add(s.settings.ReadTTL)
	url, err := s.objects.ReadURL(ctx, video.ObjectName, job.Source.Generation, expires)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("sign source read: %w", err)
	}
	return url, expires, nil
}

type OutputRequest struct {
	Attempt     int
	Kind        string
	FileName    string
	ContentType string
	SizeBytes   int64
	CRC32C      uint32
}

// valid checks the declared kind, content type, size and file name.
func (r OutputRequest) valid(maxBytes int64) bool {
	name := strings.TrimSpace(r.FileName)
	video := r.Kind == domain.AnalyzedVideo && (r.ContentType == "video/mp4" || r.ContentType == "video/webm")
	data := (r.Kind == domain.Trajectory && r.ContentType == domain.TrajectoryContentType) ||
		(r.Kind == domain.Thumbnail && (r.ContentType == "image/jpeg" || r.ContentType == "image/png"))
	return (video || data) && r.SizeBytes >= 1 && r.SizeBytes <= maxBytes &&
		name != "" && len(name) <= 120 && !strings.ContainsAny(name, `/\`) && !strings.Contains(name, "..")
}

// RequestOutput records an output the attempt will upload and signs its upload.
func (s *Service) RequestOutput(ctx context.Context, w domain.Worker, jobID string, req OutputRequest) (OutputUpload, SignedRequest, error) {
	if s.objects == nil {
		return OutputUpload{}, SignedRequest{}, ErrStorageNotConfigured
	}
	job, err := s.store.Job(ctx, jobID)
	if err != nil {
		return OutputUpload{}, SignedRequest{}, err
	}
	if err := job.CheckLease(w.ID, req.Attempt); err != nil {
		return OutputUpload{}, SignedRequest{}, err
	}
	if !req.valid(s.settings.MaxOutputBytes) {
		return OutputUpload{}, SignedRequest{}, ErrInvalidOutput
	}
	u := OutputUpload{JobID: job.ID, Attempt: job.Attempt, ObjectName: job.OutputPrefix() + strings.TrimSpace(req.FileName), Kind: req.Kind,
		ContentType: req.ContentType, SizeBytes: req.SizeBytes, CRC32C: req.CRC32C}
	if err := s.store.CreateOutputUpload(ctx, u); err != nil {
		return OutputUpload{}, SignedRequest{}, err
	}
	signed, err := s.objects.UploadURL(ctx, u.ObjectName, u.ContentType, s.now().Add(s.settings.UploadTTL))
	if err != nil {
		return OutputUpload{}, SignedRequest{}, fmt.Errorf("sign output upload: %w", err)
	}
	return u, signed, nil
}

// SubmitResult verifies every output object in storage, computes the metrics
// from the trajectory and publishes the result, the outputs, the video pair
// and the QC in one transaction. A trajectory failing QC fails the attempt.
// A repeated delivery of the accepted attempt returns the job without
// publishing again; the listener hears about the success both times.
func (s *Service) SubmitResult(ctx context.Context, w domain.Worker, jobID string, sub domain.Submission) (domain.Job, error) {
	if s.objects == nil {
		return domain.Job{}, ErrStorageNotConfigured
	}
	sub.WorkerID = w.ID
	job, err := s.store.Job(ctx, jobID)
	if err != nil {
		return domain.Job{}, err
	}
	if accepted(job, w, sub.Attempt) {
		return job, s.notify(ctx, job)
	}
	if err := job.CheckLease(w.ID, sub.Attempt); err != nil {
		return domain.Job{}, err
	}
	result, err := domain.Validate(job, sub)
	if err != nil {
		return domain.Job{}, err
	}
	outputs, err := s.verifyOutputs(ctx, job, result)
	if err != nil {
		return domain.Job{}, err
	}
	track, err := s.readTrajectory(ctx, job, result, outputs)
	if err != nil {
		return domain.Job{}, err
	}
	contract, err := s.catalog.Contract(job.ParadigmKey, job.ParadigmVersion)
	if err != nil {
		return domain.Job{}, err
	}
	quality := track.Quality()
	if reason := domain.CheckQC(contract.QC, track); reason != "" {
		return s.failQC(ctx, w, jobID, sub.Attempt, reason, quality)
	}
	var zones []domain.Zone
	if job.Calibration != nil {
		zones = job.Calibration.Zones
	}
	computed, err := s.catalog.Evaluate(job.ParadigmKey, job.ParadigmVersion, job.Parameters, track, zones, result.DurationUs)
	if err != nil {
		return domain.Job{}, err
	}
	result.Metrics, result.Events = computed.Metrics, track.Events(computed)
	out, err := s.publish(ctx, w, jobID, sub.Attempt, result, outputs, quality)
	if err != nil {
		return domain.Job{}, err
	}
	return out, s.notify(ctx, out)
}

func accepted(job domain.Job, w domain.Worker, attempt int) bool {
	return job.Status == domain.Succeeded && job.WorkerID == w.ID && job.Attempt == attempt
}

// verifyOutputs checks every artifact against its declaration and the stored object.
func (s *Service) verifyOutputs(ctx context.Context, job domain.Job, result domain.Result) ([]VerifiedOutput, error) {
	uploads, err := s.store.OutputUploads(ctx, job.ID, job.Attempt)
	if err != nil {
		return nil, err
	}
	outputs := make([]VerifiedOutput, 0, len(result.Artifacts))
	for _, a := range result.Artifacts {
		i := indexUpload(uploads, a)
		if i < 0 {
			return nil, ErrUnknownOutput
		}
		attrs, err := s.objects.Attrs(ctx, a.ObjectName)
		if errors.Is(err, ErrObjectNotFound) {
			return nil, ErrOutputNotVerified
		}
		if err != nil {
			return nil, fmt.Errorf("read output attributes: %w", err)
		}
		u := uploads[i]
		if attrs.Generation <= 0 || attrs.Size != u.SizeBytes || attrs.CRC32C != u.CRC32C || attrs.ContentType != u.ContentType {
			return nil, ErrOutputNotVerified
		}
		outputs = append(outputs, VerifiedOutput{OutputUpload: u, Bucket: s.objects.Bucket(), Generation: attrs.Generation})
	}
	return outputs, nil
}

func (s *Service) publish(ctx context.Context, w domain.Worker, jobID string, attempt int, result domain.Result, outputs []VerifiedOutput, quality domain.Quality) (domain.Job, error) {
	now := s.clock()
	var out domain.Job
	err := s.store.Transaction(ctx, func(tx Store) error {
		locked, err := tx.LockJob(ctx, jobID)
		if err != nil {
			return err
		}
		if accepted(locked, w, attempt) {
			out = locked
			return nil
		}
		done, err := locked.Succeed(w.ID, attempt, result.ModelVersion, now)
		if err != nil {
			return err
		}
		if err := tx.Publish(ctx, done, result, outputs, quality); err != nil {
			return err
		}
		out, err = tx.UpdateJob(ctx, done)
		return err
	})
	return out, err
}

// failQC stores the quality of a failing trajectory and fails the job for good.
func (s *Service) failQC(ctx context.Context, w domain.Worker, jobID string, attempt int, reason string, quality domain.Quality) (domain.Job, error) {
	now := s.clock()
	var out domain.Job
	err := s.store.Transaction(ctx, func(tx Store) error {
		job, err := tx.LockJob(ctx, jobID)
		if err != nil {
			return err
		}
		failed, err := job.Fail(w.ID, attempt, reason, false, now)
		if err != nil {
			return err
		}
		if err := tx.RecordQC(ctx, job, QC{Quality: quality}); err != nil {
			return err
		}
		out, err = tx.UpdateJob(ctx, failed)
		return err
	})
	return out, err
}

func (s *Service) notify(ctx context.Context, job domain.Job) error {
	if s.listener == nil {
		return nil
	}
	if err := s.listener.JobSucceeded(ctx, job); err != nil {
		return fmt.Errorf("notify job %s succeeded: %w", job.ID, err)
	}
	return nil
}

// readTrajectory reads the verified trajectory generation. The published
// metrics are computed from exactly these bytes.
func (s *Service) readTrajectory(ctx context.Context, job domain.Job, result domain.Result, outputs []VerifiedOutput) (domain.Track, error) {
	for _, o := range outputs {
		if o.ObjectName != result.TrajectoryObject {
			continue
		}
		if o.ContentType != domain.TrajectoryContentType {
			return domain.Track{}, domain.ErrMissingTrajectory
		}
		if o.SizeBytes > s.settings.MaxTrajectoryBytes {
			return domain.Track{}, fmt.Errorf("%w: %d bytes exceed %d", domain.ErrInvalidTrajectory, o.SizeBytes, s.settings.MaxTrajectoryBytes)
		}
		data, err := s.objects.Read(ctx, o.ObjectName, o.Generation, s.settings.MaxTrajectoryBytes)
		if errors.Is(err, ErrObjectNotFound) {
			return domain.Track{}, ErrOutputNotVerified
		}
		if err != nil {
			return domain.Track{}, fmt.Errorf("read trajectory: %w", err)
		}
		return domain.ParseTrajectory(data, job, result.DurationUs)
	}
	return domain.Track{}, domain.ErrMissingTrajectory
}

func indexUpload(uploads []OutputUpload, a domain.ArtifactInput) int {
	for i, u := range uploads {
		if u.ObjectName == a.ObjectName && u.Kind == a.Kind {
			return i
		}
	}
	return -1
}

// Fail records a worker failure for its current attempt.
func (s *Service) Fail(ctx context.Context, w domain.Worker, jobID string, attempt int, reason string, retryable bool) (domain.Job, error) {
	now := s.clock()
	var out domain.Job
	err := s.store.Transaction(ctx, func(tx Store) error {
		job, err := tx.LockJob(ctx, jobID)
		if err != nil {
			return err
		}
		failed, err := job.Fail(w.ID, attempt, reason, retryable, now)
		if err != nil {
			return err
		}
		out, err = tx.UpdateJob(ctx, failed)
		return err
	})
	return out, err
}
