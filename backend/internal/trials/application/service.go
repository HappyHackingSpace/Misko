// Package application contains paradigm trial use cases. Creating a trial,
// uploading its video, analysing it and deleting it need trial:run; reading
// trials and their analyses needs *:read. A trial keeps its own video (in the
// media module), may have its own calibration on that video (in the
// calibration module, never changing the environment) and asks the analysis
// service for jobs; it never reads another module's data except through the
// ports below.
package application

import (
	"context"
	"errors"
	"fmt"
	"time"

	access "github.com/HappyHackingSpace/Misko/backend/internal/access/domain"
	"github.com/HappyHackingSpace/Misko/backend/internal/trials/domain"
)

var (
	ErrTrialNotFound        = errors.New("paradigm trial not found")
	ErrRevisionNotFound     = errors.New("environment revision not found")
	ErrParadigmMismatch     = errors.New("the environment revision runs another paradigm version")
	ErrNoVideo              = errors.New("the trial has no video yet")
	ErrVideoNotVerified     = errors.New("the trial's video has not been verified")
	ErrNotReady             = errors.New("the trial needs a verified video and, when the paradigm requires one, a valid calibration with every required zone")
	ErrInvalidQuery         = errors.New("invalid trial query")
	ErrInvalidUpload        = errors.New("the upload needs a video content type, a size within the limit, a base64 CRC32C and a file name")
	ErrUploadIncomplete     = errors.New("the upload has not finished; finalize after it completes")
	ErrStorageNotConfigured = errors.New("video storage is not configured")
)

// Analysis is the analysis service's port as trials use it.
type Analysis interface {
	// Submit returns ErrNotReady when the service refuses the request's
	// inputs (the video, the calibration or the paradigm version).
	Submit(ctx context.Context, req domain.JobRequest) (domain.Job, error)
	// Results returns the known jobs among ids with their published results.
	Results(ctx context.Context, jobIDs []string) ([]domain.Result, error)
	// Latest returns the newest job of each trial that has one.
	Latest(ctx context.Context, trialIDs []string) (map[string]domain.Job, error)
}

// Videos is the media module's asset port.
type Videos interface {
	// StartUpload returns ErrInvalidUpload or ErrStorageNotConfigured.
	StartUpload(ctx context.Context, folder string, req domain.UploadRequest, createdBy string) (domain.Video, domain.SignedUpload, error)
	// Finalize returns ErrUploadIncomplete while the upload runs.
	Finalize(ctx context.Context, videoID string) (domain.Video, error)
	Video(ctx context.Context, videoID string) (domain.Video, error)
	// ReadURL returns ErrVideoNotVerified for an unverified video.
	ReadURL(ctx context.Context, videoID string) (string, time.Time, error)
}

// Calibrations is the calibration module's port.
type Calibrations interface {
	// TrialCalibration returns the valid calibration a trial is analysed with
	// now, or "" while it waits for one.
	TrialCalibration(ctx context.Context, trialID string) (string, error)
}

// Environments is the environments module's port.
type Environments interface {
	// Revision returns ErrRevisionNotFound for an unknown id.
	Revision(ctx context.Context, id string) (domain.Revision, error)
}

// Catalog checks ANALYSIS overrides against the paradigm catalog.
type Catalog interface {
	// CheckOverrides returns domain.ErrInvalidOverrides unless every override
	// is an ANALYSIS parameter of the version and the values are consistent
	// with the arena's parameters.
	CheckOverrides(key string, version int, parameters, overrides map[string]float64) error
}

// ListFilter selects trials. Search matches the name, ignoring case.
type ListFilter struct {
	Search, ParadigmKey string
	Limit, Offset       int
}

// ListItem is a trial with the name of its arena.
type ListItem struct {
	Trial           domain.Trial
	EnvironmentID   string
	EnvironmentName string
	RevisionNumber  int
}

type Store interface {
	Transaction(ctx context.Context, fn func(Store) error) error
	Create(ctx context.Context, t domain.Trial) (domain.Trial, error)
	// Trial returns ErrTrialNotFound for an unknown or deleted trial.
	Trial(ctx context.Context, id string) (domain.Trial, error)
	// LockTrial serializes uploads and analyses of one trial.
	LockTrial(ctx context.Context, id string) (domain.Trial, error)
	SetVideo(ctx context.Context, id, videoID string, clip domain.Clip) (domain.Trial, error)
	Delete(ctx context.Context, id string, at time.Time) error
	List(ctx context.Context, f ListFilter) ([]ListItem, int, error)
	// Candidates lists trials with a verified video and no automatic
	// analysis of it, most recently changed first.
	Candidates(ctx context.Context, limit int) ([]domain.Trial, error)
	// CreateLink reports false when an equal automatic link exists.
	CreateLink(ctx context.Context, l domain.Link) (bool, error)
	// Links lists a trial's analyses, newest first.
	Links(ctx context.Context, trialID string) ([]domain.Link, error)
}

type Service struct {
	store        Store
	analysis     Analysis
	videos       Videos
	calibrations Calibrations
	environments Environments
	catalog      Catalog
	now          func() time.Time
}

// Dependencies are the ports the service uses.
type Dependencies struct {
	Store        Store
	Analysis     Analysis
	Videos       Videos
	Calibrations Calibrations
	Environments Environments
	Catalog      Catalog
}

func New(deps Dependencies, now func() time.Time) *Service {
	return &Service{store: deps.Store, analysis: deps.Analysis, videos: deps.Videos, calibrations: deps.Calibrations,
		environments: deps.Environments, catalog: deps.Catalog, now: now}
}

type CreateInput struct {
	Name, ParadigmKey     string
	ParadigmVersion       int
	EnvironmentRevisionID string
	Notes                 string
}

// Create starts a trial of a paradigm version in an arena of that version.
func (s *Service) Create(ctx context.Context, actor access.Actor, in CreateInput) (domain.Trial, error) {
	if err := actor.Require(access.TrialRun); err != nil {
		return domain.Trial{}, err
	}
	t, err := domain.NewTrial(in.Name, in.ParadigmKey, in.ParadigmVersion, in.EnvironmentRevisionID, in.Notes, actor.UserID)
	if err != nil {
		return domain.Trial{}, err
	}
	revision, err := s.environments.Revision(ctx, in.EnvironmentRevisionID)
	if err != nil {
		return domain.Trial{}, err
	}
	if revision.ParadigmKey != in.ParadigmKey || revision.ParadigmVersion != in.ParadigmVersion {
		return domain.Trial{}, ErrParadigmMismatch
	}
	return s.store.Create(ctx, t)
}

type ListQuery struct {
	Search, ParadigmKey string
	Page, PageSize      int
}

// Summary is a trial in the list with its arena and its newest analysis.
type Summary struct {
	ListItem
	Latest *domain.Job
}

type Page struct {
	Items                 []Summary
	Total, Page, PageSize int
}

// List pages through the trials, newest first.
func (s *Service) List(ctx context.Context, actor access.Actor, q ListQuery) (Page, error) {
	if err := actor.Require(access.Read); err != nil {
		return Page{}, err
	}
	if q.Page < 0 || q.Page > 1<<20 || q.PageSize < 0 || len(q.Search) > 120 || len(q.ParadigmKey) > 40 {
		return Page{}, ErrInvalidQuery
	}
	page, size := max(q.Page, 1), min(q.PageSize, 200)
	if size == 0 {
		size = 50
	}
	items, total, err := s.store.List(ctx, ListFilter{Search: q.Search, ParadigmKey: q.ParadigmKey, Limit: size, Offset: (page - 1) * size})
	if err != nil {
		return Page{}, err
	}
	ids := make([]string, 0, len(items))
	for _, i := range items {
		ids = append(ids, i.Trial.ID)
	}
	latest, err := s.analysis.Latest(ctx, ids)
	if err != nil {
		return Page{}, err
	}
	out := Page{Items: make([]Summary, 0, len(items)), Total: total, Page: page, PageSize: size}
	for _, i := range items {
		summary := Summary{ListItem: i}
		if j, ok := latest[i.Trial.ID]; ok {
			summary.Latest = &j
		}
		out.Items = append(out.Items, summary)
	}
	return out, nil
}

// Detail is a trial with its arena, its current video, the calibration it
// would be analysed with now and every analysis, newest first.
type Detail struct {
	Trial         domain.Trial
	Revision      domain.Revision
	Video         *domain.Video
	CalibrationID string
	Analyses      []domain.Analysis
}

func (s *Service) Get(ctx context.Context, actor access.Actor, id string) (Detail, error) {
	if err := actor.Require(access.Read); err != nil {
		return Detail{}, err
	}
	t, err := s.store.Trial(ctx, id)
	if err != nil {
		return Detail{}, err
	}
	return s.detail(ctx, t)
}

func (s *Service) detail(ctx context.Context, t domain.Trial) (Detail, error) {
	out := Detail{Trial: t}
	var err error
	if out.Revision, err = s.environments.Revision(ctx, t.EnvironmentRevisionID); err != nil {
		return Detail{}, err
	}
	if t.VideoAssetID != "" {
		video, err := s.videos.Video(ctx, t.VideoAssetID)
		if err != nil {
			return Detail{}, err
		}
		out.Video = &video
	}
	if out.CalibrationID, err = s.calibrations.TrialCalibration(ctx, t.ID); err != nil {
		return Detail{}, err
	}
	if out.Analyses, err = s.analyses(ctx, t.ID); err != nil {
		return Detail{}, err
	}
	return out, nil
}

// analyses joins the trial's links with the jobs' results.
func (s *Service) analyses(ctx context.Context, trialID string) ([]domain.Analysis, error) {
	links, err := s.store.Links(ctx, trialID)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(links))
	for _, l := range links {
		ids = append(ids, l.JobID)
	}
	results, err := s.analysis.Results(ctx, ids)
	if err != nil {
		return nil, err
	}
	byJob := make(map[string]domain.Result, len(results))
	for _, r := range results {
		byJob[r.Job.ID] = r
	}
	out := make([]domain.Analysis, 0, len(links))
	for _, l := range links {
		r, ok := byJob[l.JobID]
		if !ok {
			return nil, fmt.Errorf("analysis job %s of trial %s is unknown to the analysis service", l.JobID, trialID)
		}
		out = append(out, domain.Analysis{Link: l, Result: r})
	}
	return out, nil
}

// Upload is a started video upload of a trial.
type Upload struct {
	Trial   domain.Trial
	Video   domain.Video
	Request domain.SignedUpload
}

// StartUpload declares the trial's new video and signs its resumable upload.
// The new video becomes the trial's current one; earlier analyses keep the
// video they analysed.
func (s *Service) StartUpload(ctx context.Context, actor access.Actor, id string, req domain.UploadRequest) (Upload, error) {
	if err := actor.Require(access.TrialRun); err != nil {
		return Upload{}, err
	}
	if err := req.Clip.Validate(); err != nil {
		return Upload{}, err
	}
	var out Upload
	err := s.store.Transaction(ctx, func(tx Store) error {
		t, err := tx.LockTrial(ctx, id)
		if err != nil {
			return err
		}
		video, signed, err := s.videos.StartUpload(ctx, "trials/"+t.ID, req, actor.UserID)
		if err != nil {
			return err
		}
		if t, err = tx.SetVideo(ctx, t.ID, video.ID, req.Clip); err != nil {
			return err
		}
		out = Upload{Trial: t, Video: video, Request: signed}
		return nil
	})
	return out, err
}

// FinalizeUpload verifies the trial's current video and, once it is
// verified, submits the automatic analysis when the trial is ready. A trial
// still waiting for its calibration is analysed by a later tick.
func (s *Service) FinalizeUpload(ctx context.Context, actor access.Actor, id string) (Detail, error) {
	if err := actor.Require(access.TrialRun); err != nil {
		return Detail{}, err
	}
	t, err := s.store.Trial(ctx, id)
	if err != nil {
		return Detail{}, err
	}
	if t.VideoAssetID == "" {
		return Detail{}, ErrNoVideo
	}
	video, err := s.videos.Finalize(ctx, t.VideoAssetID)
	if err != nil {
		return Detail{}, err
	}
	if video.Status == domain.VideoVerified {
		if _, err := s.submitAutomatic(ctx, t); err != nil && !errors.Is(err, ErrNotReady) {
			return Detail{}, err
		}
	}
	return s.detail(ctx, t)
}

// VideoURL signs a read of the trial's verified video, for calibrating on it
// and for playback.
func (s *Service) VideoURL(ctx context.Context, actor access.Actor, id string) (string, time.Time, error) {
	if err := actor.Require(access.Read); err != nil {
		return "", time.Time{}, err
	}
	t, err := s.store.Trial(ctx, id)
	if err != nil {
		return "", time.Time{}, err
	}
	if t.VideoAssetID == "" {
		return "", time.Time{}, ErrNoVideo
	}
	return s.videos.ReadURL(ctx, t.VideoAssetID)
}

// Analyse submits a new manual analysis of the trial's current video with
// the arena's parameters and the given ANALYSIS overrides. Earlier analyses
// stay for comparison.
func (s *Service) Analyse(ctx context.Context, actor access.Actor, id string, overrides map[string]float64) (domain.Analysis, error) {
	if err := actor.Require(access.TrialRun); err != nil {
		return domain.Analysis{}, err
	}
	if err := domain.CheckOverrides(overrides); err != nil {
		return domain.Analysis{}, err
	}
	var out domain.Analysis
	err := s.store.Transaction(ctx, func(tx Store) error {
		t, err := tx.LockTrial(ctx, id)
		if err != nil {
			return err
		}
		link, job, err := s.submit(ctx, t, domain.Manual, overrides, actor.UserID)
		if err != nil {
			return err
		}
		if _, err := tx.CreateLink(ctx, link); err != nil {
			return err
		}
		link.CreatedAt = job.CreatedAt
		out = domain.Analysis{Link: link, Result: domain.Result{Job: job}}
		return nil
	})
	return out, err
}

// Delete hides a trial. Its analyses never change and stay readable through
// the analysis service.
func (s *Service) Delete(ctx context.Context, actor access.Actor, id string) error {
	if err := actor.Require(access.TrialRun); err != nil {
		return err
	}
	return s.store.Transaction(ctx, func(tx Store) error {
		t, err := tx.LockTrial(ctx, id)
		if err != nil {
			return err
		}
		return tx.Delete(ctx, t.ID, s.now().UTC().Truncate(time.Microsecond))
	})
}

// TickReport counts what a tick did. Skipped holds trials that failed for
// another reason than waiting for a calibration.
type TickReport struct {
	Submitted int
	Skipped   []error
}

// candidateLimit bounds the trials one tick looks at.
const candidateLimit = 200

// Tick submits the automatic analysis of every trial whose verified video has
// none yet and that is ready, such as one calibrated after its upload.
func (s *Service) Tick(ctx context.Context) (TickReport, error) {
	var report TickReport
	candidates, err := s.store.Candidates(ctx, candidateLimit)
	if err != nil {
		return TickReport{}, err
	}
	for _, t := range candidates {
		created, err := s.submitAutomatic(ctx, t)
		switch {
		case errors.Is(err, ErrNotReady):
		case err != nil:
			report.Skipped = append(report.Skipped, fmt.Errorf("trial %s: %w", t.ID, err))
		case created:
			report.Submitted++
		}
	}
	return report, nil
}

func (s *Service) submitAutomatic(ctx context.Context, t domain.Trial) (bool, error) {
	link, _, err := s.submit(ctx, t, domain.Automatic, nil, "")
	if err != nil {
		return false, err
	}
	return s.store.CreateLink(ctx, link)
}

// submit asks the analysis service for a job of the trial's current video.
func (s *Service) submit(ctx context.Context, t domain.Trial, trigger domain.Trigger, overrides map[string]float64, by string) (domain.Link, domain.Job, error) {
	if err := t.Active(); err != nil {
		return domain.Link{}, domain.Job{}, err
	}
	if t.VideoAssetID == "" {
		return domain.Link{}, domain.Job{}, ErrNoVideo
	}
	revision, err := s.environments.Revision(ctx, t.EnvironmentRevisionID)
	if err != nil {
		return domain.Link{}, domain.Job{}, err
	}
	if err := s.catalog.CheckOverrides(t.ParadigmKey, t.ParadigmVersion, revision.Parameters, overrides); err != nil {
		return domain.Link{}, domain.Job{}, err
	}
	calibration, err := s.calibrations.TrialCalibration(ctx, t.ID)
	if err != nil {
		return domain.Link{}, domain.Job{}, err
	}
	req := domain.JobRequest{TrialID: t.ID, ParadigmKey: t.ParadigmKey, ParadigmVersion: t.ParadigmVersion,
		Parameters: domain.Merge(revision.Parameters, overrides), SourceAssetID: t.VideoAssetID, Clip: t.Clip, CalibrationID: calibration,
		Trigger: trigger, CreatedBy: by}
	if trigger == domain.Automatic {
		req.IdempotencyKey = t.AutomaticKey()
	}
	job, err := s.analysis.Submit(ctx, req)
	if err != nil {
		return domain.Link{}, domain.Job{}, err
	}
	link := domain.Link{JobID: job.ID, TrialID: t.ID, VideoAssetID: t.VideoAssetID, CalibrationID: job.CalibrationID, Trigger: trigger,
		Overrides: overrides, CreatedBy: by}
	return link, job, nil
}
