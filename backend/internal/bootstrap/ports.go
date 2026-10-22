package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"time"

	analysisapp "github.com/HappyHackingSpace/Misko/backend/internal/analysis/application"
	analysisdomain "github.com/HappyHackingSpace/Misko/backend/internal/analysis/domain"
	calibrationapp "github.com/HappyHackingSpace/Misko/backend/internal/calibration/application"
	environmentsapp "github.com/HappyHackingSpace/Misko/backend/internal/environments/application"
	mediaapp "github.com/HappyHackingSpace/Misko/backend/internal/media/application"
	mediadomain "github.com/HappyHackingSpace/Misko/backend/internal/media/domain"
	subjectanalysisapp "github.com/HappyHackingSpace/Misko/backend/internal/subjectanalysis/application"
	subjectanalysisdomain "github.com/HappyHackingSpace/Misko/backend/internal/subjectanalysis/domain"
	trialsapp "github.com/HappyHackingSpace/Misko/backend/internal/trials/application"
	trialsdomain "github.com/HappyHackingSpace/Misko/backend/internal/trials/domain"
)

// The adapters below translate between the analysis service's port and the
// interfaces its requesters declare, so no module imports another.

// subjectAnalysisPort is the analysis service as the experiment side sees it.
type subjectAnalysisPort struct{ analysis *analysisapp.Service }

func (p subjectAnalysisPort) Submit(ctx context.Context, r subjectanalysisdomain.JobRequest) (subjectanalysisdomain.Job, error) {
	job, err := p.analysis.Submit(ctx, analysisdomain.Request{
		Requester: analysisdomain.Requester{Kind: analysisdomain.RequesterTestSubject, ID: r.TestSubjectID}, ParadigmKey: r.ParadigmKey,
		ParadigmVersion: r.ParadigmVersion, Parameters: r.Parameters, SourceAssetID: r.SourceAssetID, ClipStartUs: r.ClipStartUs, ClipEndUs: r.ClipEndUs,
		CalibrationID: r.CalibrationID, Trigger: analysisdomain.Trigger(r.Trigger), IdempotencyKey: r.IdempotencyKey, CreatedBy: r.CreatedBy,
	})
	if refused(err) {
		return subjectanalysisdomain.Job{}, fmt.Errorf("%w: %v", subjectanalysisapp.ErrNotReady, err)
	}
	if err != nil {
		return subjectanalysisdomain.Job{}, err
	}
	return subjectJob(job), nil
}

func (p subjectAnalysisPort) Jobs(ctx context.Context, ids []string) ([]subjectanalysisdomain.Job, error) {
	jobs, err := p.analysis.Jobs(ctx, ids)
	if err != nil {
		return nil, err
	}
	out := make([]subjectanalysisdomain.Job, 0, len(jobs))
	for _, j := range jobs {
		out = append(out, subjectJob(j))
	}
	return out, nil
}

func (p subjectAnalysisPort) Metrics(ctx context.Context, ids []string) (map[string][]subjectanalysisdomain.Metric, error) {
	results, err := p.analysis.Results(ctx, ids)
	if err != nil {
		return nil, err
	}
	out := make(map[string][]subjectanalysisdomain.Metric, len(results))
	for _, r := range results {
		if r.Published == nil {
			continue
		}
		for _, m := range r.Published.Metrics {
			out[r.Job.ID] = append(out[r.Job.ID], subjectanalysisdomain.Metric{Key: m.Key, Unit: m.Unit, Value: m.Value, MissingReason: m.MissingReason})
		}
	}
	return out, nil
}

func subjectJob(j analysisdomain.Job) subjectanalysisdomain.Job {
	return subjectanalysisdomain.Job{
		ID: j.ID, Status: subjectanalysisdomain.JobStatus(j.Status), ParadigmKey: j.ParadigmKey, ParadigmVersion: j.ParadigmVersion,
		MetricEngineVersion: j.MetricEngineVersion, ResultSchemaVersion: j.ResultSchemaVersion, Parameters: j.Parameters,
		SourceAssetID: j.Source.AssetID, SourceGeneration: j.Source.Generation, SourceCRC32C: j.Source.CRC32C, ClipStartUs: j.Source.ClipStartUs,
		ClipEndUs: j.Source.ClipEndUs, CalibrationID: j.CalibrationID(), Attempt: j.Attempt, MaxAttempts: j.MaxAttempts, WorkerID: j.WorkerID,
		LeaseExpiresAt: j.LeaseExpiresAt, ModelVersion: j.ModelVersion, FailureReason: j.FailureReason, CreatedBy: j.CreatedBy,
		CreatedAt: j.CreatedAt, FinishedAt: j.FinishedAt,
	}
}

// trialsAnalysisPort is the analysis service as paradigm trials see it.
type trialsAnalysisPort struct{ analysis *analysisapp.Service }

func (p trialsAnalysisPort) Submit(ctx context.Context, r trialsdomain.JobRequest) (trialsdomain.Job, error) {
	job, err := p.analysis.Submit(ctx, analysisdomain.Request{
		Requester: analysisdomain.Requester{Kind: analysisdomain.RequesterTrial, ID: r.TrialID}, ParadigmKey: r.ParadigmKey,
		ParadigmVersion: r.ParadigmVersion, Parameters: r.Parameters, SourceAssetID: r.SourceAssetID, ClipStartUs: r.Clip.StartUs, ClipEndUs: r.Clip.EndUs,
		CalibrationID: r.CalibrationID, Trigger: analysisdomain.Trigger(r.Trigger), IdempotencyKey: r.IdempotencyKey, CreatedBy: r.CreatedBy,
	})
	if refused(err) {
		return trialsdomain.Job{}, fmt.Errorf("%w: %v", trialsapp.ErrNotReady, err)
	}
	if err != nil {
		return trialsdomain.Job{}, err
	}
	return trialJob(job), nil
}

func (p trialsAnalysisPort) Results(ctx context.Context, ids []string) ([]trialsdomain.Result, error) {
	results, err := p.analysis.Results(ctx, ids)
	if err != nil {
		return nil, err
	}
	out := make([]trialsdomain.Result, 0, len(results))
	for _, r := range results {
		res := trialsdomain.Result{Job: trialJob(r.Job)}
		if pub := r.Published; pub != nil {
			for _, m := range pub.Metrics {
				res.Metrics = append(res.Metrics, trialsdomain.Metric{Key: m.Key, Unit: m.Unit, Value: m.Value, MissingReason: m.MissingReason})
			}
			if q := pub.QC; q != nil {
				res.QC = &trialsdomain.QC{Samples: q.Samples, Tracked: q.Tracked, Inferred: q.Inferred, LostRatio: q.LostRatio,
					InferredRatio: q.InferredRatio, MeanConfidence: q.MeanConfidence, Passed: q.Passed}
			}
		}
		out = append(out, res)
	}
	return out, nil
}

// Latest reads every job of the trials, newest first, and keeps the first of each.
func (p trialsAnalysisPort) Latest(ctx context.Context, trialIDs []string) (map[string]trialsdomain.Job, error) {
	jobs, err := p.analysis.ByRequester(ctx, analysisdomain.RequesterTrial, trialIDs)
	if err != nil {
		return nil, err
	}
	out := map[string]trialsdomain.Job{}
	for _, j := range jobs {
		if _, seen := out[j.Requester.ID]; !seen {
			out[j.Requester.ID] = trialJob(j)
		}
	}
	return out, nil
}

func trialJob(j analysisdomain.Job) trialsdomain.Job {
	return trialsdomain.Job{ID: j.ID, Status: string(j.Status), ParadigmKey: j.ParadigmKey, ParadigmVersion: j.ParadigmVersion, Parameters: j.Parameters,
		SourceAssetID: j.Source.AssetID, CalibrationID: j.CalibrationID(), Attempt: j.Attempt, MaxAttempts: j.MaxAttempts, ModelVersion: j.ModelVersion,
		FailureReason: j.FailureReason, CreatedAt: j.CreatedAt, FinishedAt: j.FinishedAt}
}

// trialsVideos is the media module's asset port as paradigm trials see it.
type trialsVideos struct{ media *mediaapp.Service }

func (v trialsVideos) StartUpload(ctx context.Context, folder string, r trialsdomain.UploadRequest, createdBy string) (trialsdomain.Video, trialsdomain.SignedUpload, error) {
	asset, signed, err := v.media.StartAssetUpload(ctx, folder, mediadomain.UploadRequest{FileName: r.FileName, ContentType: r.ContentType,
		SizeBytes: r.SizeBytes, CRC32C: r.CRC32C, ClipStartUs: r.Clip.StartUs, ClipEndUs: r.Clip.EndUs}, createdBy)
	if err != nil {
		return trialsdomain.Video{}, trialsdomain.SignedUpload{}, videoError(err)
	}
	return trialVideo(asset), trialsdomain.SignedUpload(signed), nil
}

func (v trialsVideos) Finalize(ctx context.Context, id string) (trialsdomain.Video, error) {
	asset, err := v.media.FinalizeAsset(ctx, id)
	if err != nil {
		return trialsdomain.Video{}, videoError(err)
	}
	return trialVideo(asset), nil
}

func (v trialsVideos) Video(ctx context.Context, id string) (trialsdomain.Video, error) {
	asset, err := v.media.Asset(ctx, id)
	if err != nil {
		return trialsdomain.Video{}, videoError(err)
	}
	return trialVideo(asset), nil
}

func (v trialsVideos) ReadURL(ctx context.Context, id string) (string, time.Time, error) {
	link, err := v.media.AssetReadURL(ctx, id)
	if err != nil {
		return "", time.Time{}, videoError(err)
	}
	return link.URL, link.ExpiresAt, nil
}

func trialVideo(a mediadomain.Asset) trialsdomain.Video {
	return trialsdomain.Video{ID: a.ID, FileName: a.FileName, ContentType: a.ContentType, SizeBytes: a.SizeBytes, Status: string(a.Status),
		RejectionReason: a.RejectionReason, CreatedAt: a.CreatedAt, VerifiedAt: a.VerifiedAt}
}

// videoError translates the media module's errors into the trials module's.
func videoError(err error) error {
	switch {
	case errors.Is(err, mediaapp.ErrStorageNotConfigured):
		return trialsapp.ErrStorageNotConfigured
	case errors.Is(err, mediaapp.ErrUploadIncomplete):
		return trialsapp.ErrUploadIncomplete
	case errors.Is(err, mediaapp.ErrNotVerified):
		return trialsapp.ErrVideoNotVerified
	case errors.Is(err, mediaapp.ErrAssetNotFound):
		return trialsapp.ErrNoVideo
	case errors.Is(err, mediadomain.ErrInvalidClip):
		return trialsdomain.ErrInvalidClip
	case errors.Is(err, mediadomain.ErrInvalidContentType), errors.Is(err, mediadomain.ErrInvalidSize), errors.Is(err, mediadomain.ErrInvalidChecksum),
		errors.Is(err, mediadomain.ErrInvalidFileName):
		return fmt.Errorf("%w: %v", trialsapp.ErrInvalidUpload, err)
	}
	return err
}

// trialsCalibrations is the calibration module's port as trials see it.
type trialsCalibrations struct{ calibration *calibrationapp.Service }

func (c trialsCalibrations) TrialCalibration(ctx context.Context, trialID string) (string, error) {
	id, err := c.calibration.TrialCalibration(ctx, trialID)
	if errors.Is(err, calibrationapp.ErrTrialNotFound) {
		return "", trialsapp.ErrTrialNotFound
	}
	return id, err
}

// trialsEnvironments is the environments module's port as trials see it.
type trialsEnvironments struct{ environments *environmentsapp.Service }

func (e trialsEnvironments) Revision(ctx context.Context, id string) (trialsdomain.Revision, error) {
	r, err := e.environments.RevisionByID(ctx, id)
	if errors.Is(err, environmentsapp.ErrRevisionNotFound) {
		return trialsdomain.Revision{}, trialsapp.ErrRevisionNotFound
	}
	if err != nil {
		return trialsdomain.Revision{}, err
	}
	return trialsdomain.Revision{ID: r.ID, EnvironmentID: r.EnvironmentID, EnvironmentName: r.EnvironmentName, Number: r.Number,
		ParadigmKey: r.ParadigmKey, ParadigmVersion: r.ParadigmVersion, Parameters: r.Parameters}, nil
}
