package application

import (
	"context"
	"errors"
	"fmt"

	"github.com/HappyHackingSpace/Misko/backend/internal/analysis/domain"
)

// The in-process port. Callers are other modules that already authorized
// their user; they name themselves as an opaque requester and never see the
// worker protocol.

// MaxBatch bounds the ids one port call reads.
const MaxBatch = 500

// ErrTooManyIDs means a port call named more than MaxBatch ids.
var ErrTooManyIDs = errors.New("too many ids in one call")

// Submit snapshots a request into a new queued job: the paradigm contract,
// the verified source video and, when the paradigm needs one, the valid
// calibration with every required zone. A request whose idempotency key was
// seen before returns the job the first one created.
func (s *Service) Submit(ctx context.Context, req domain.Request) (domain.Job, error) {
	if err := req.Validate(); err != nil {
		return domain.Job{}, err
	}
	contract, err := s.catalog.Contract(req.ParadigmKey, req.ParadigmVersion)
	if errors.Is(err, ErrUnknownCapability) {
		return domain.Job{}, fmt.Errorf("%w: %v", domain.ErrNotAnalysable, err)
	}
	if err != nil {
		return domain.Job{}, err
	}
	video, err := s.sources.Source(ctx, req.SourceAssetID)
	if err != nil {
		return domain.Job{}, err
	}
	calibration, err := s.calibration(ctx, contract, req.CalibrationID)
	if err != nil {
		return domain.Job{}, err
	}
	job, err := domain.NewJob(req, contract, video, calibration, s.clock(), s.settings.MaxAttempts)
	if err != nil {
		return domain.Job{}, err
	}
	return s.store.CreateJob(ctx, job)
}

// calibration reads the calibration to snapshot when the contract needs one.
func (s *Service) calibration(ctx context.Context, contract domain.Contract, id string) (*domain.Calibration, error) {
	if !contract.CalibrationRequired || id == "" {
		return nil, nil
	}
	c, err := s.calibrations.Calibration(ctx, id)
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// Get returns one job, or ErrJobNotFound.
func (s *Service) Get(ctx context.Context, id string) (domain.Job, error) {
	return s.store.Job(ctx, id)
}

// Jobs returns the known jobs among ids, newest first.
func (s *Service) Jobs(ctx context.Context, ids []string) ([]domain.Job, error) {
	if len(ids) > MaxBatch {
		return nil, ErrTooManyIDs
	}
	if len(ids) == 0 {
		return []domain.Job{}, nil
	}
	return s.store.Jobs(ctx, ids)
}

// ByRequester returns every job the requesters asked for, newest first.
func (s *Service) ByRequester(ctx context.Context, kind domain.RequesterKind, ids []string) ([]domain.Job, error) {
	if !kind.Valid() {
		return nil, domain.ErrInvalidRequest
	}
	if len(ids) > MaxBatch {
		return nil, ErrTooManyIDs
	}
	if len(ids) == 0 {
		return []domain.Job{}, nil
	}
	return s.store.JobsByRequester(ctx, kind, ids)
}

// Result is a job with what it published. Published is nil until the job
// succeeded, except for a job QC failed, whose Published carries only QC.
type Result struct {
	Job       domain.Job
	Published *Published
}

// Results returns the known jobs among ids with their stored results, newest first.
func (s *Service) Results(ctx context.Context, ids []string) ([]Result, error) {
	jobs, err := s.Jobs(ctx, ids)
	if err != nil {
		return nil, err
	}
	return s.withResults(ctx, jobs)
}

func (s *Service) withResults(ctx context.Context, jobs []domain.Job) ([]Result, error) {
	ids := make([]string, 0, len(jobs))
	for _, j := range jobs {
		ids = append(ids, j.ID)
	}
	published := map[string]Published{}
	if len(ids) > 0 {
		var err error
		if published, err = s.store.Published(ctx, ids); err != nil {
			return nil, err
		}
	}
	out := make([]Result, 0, len(jobs))
	for _, j := range jobs {
		r := Result{Job: j}
		if p, ok := published[j.ID]; ok {
			r.Published = &p
		}
		out = append(out, r)
	}
	return out, nil
}
