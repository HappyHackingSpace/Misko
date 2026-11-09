package application

import (
	"context"
	"errors"
	"fmt"
	"time"

	access "github.com/HappyHackingSpace/Misko/backend/internal/access/domain"
	"github.com/HappyHackingSpace/Misko/backend/internal/analysis/domain"
)

var (
	// ErrOutputNotFound means the job published no output of the requested kind.
	ErrOutputNotFound = errors.New("the job has no published output of this kind")
	ErrNoArenaFloor   = errors.New("the paradigm has no arena floor to map occupancy on")
)

// Occupancy is the tracked time in each square cell of the arena floor, row
// by row from (MinXCm, MinYCm).
type Occupancy struct {
	MinXCm, MinYCm, CellCm float64
	Columns, Rows          int
	Seconds                []float64
}

// JobDetail is one job with its stored result, for every screen that shows
// analysis results.
func (s *Service) JobDetail(ctx context.Context, actor access.Actor, id string) (Result, error) {
	if err := actor.Require(access.Read); err != nil {
		return Result{}, err
	}
	job, err := s.store.Job(ctx, id)
	if err != nil {
		return Result{}, err
	}
	results, err := s.withResults(ctx, []domain.Job{job})
	if err != nil {
		return Result{}, err
	}
	return results[0], nil
}

type ReadLink struct {
	URL       string
	ExpiresAt time.Time
}

type VideoPairLinks struct {
	Job                      domain.Job
	Pair                     Pair
	OriginalURL, AnalyzedURL string
	ExpiresAt                time.Time
}

// VideoPair signs reads of the job's pinned original and its own analysed video.
func (s *Service) VideoPair(ctx context.Context, actor access.Actor, id string) (VideoPairLinks, error) {
	job, published, err := s.succeeded(ctx, actor, id)
	if err != nil {
		return VideoPairLinks{}, err
	}
	if published.Pair == nil {
		return VideoPairLinks{}, ErrJobNotSucceeded
	}
	video, err := s.sources.Source(ctx, job.Source.AssetID)
	if err != nil {
		return VideoPairLinks{}, err
	}
	p := *published.Pair
	expires := s.now().Add(s.settings.ReadTTL)
	original, err := s.objects.ReadURL(ctx, video.ObjectName, job.Source.Generation, expires)
	if err != nil {
		return VideoPairLinks{}, fmt.Errorf("sign original read: %w", err)
	}
	analyzed, err := s.objects.ReadURL(ctx, p.AnalyzedObjectName, p.AnalyzedGeneration, expires)
	if err != nil {
		return VideoPairLinks{}, fmt.Errorf("sign analysed read: %w", err)
	}
	return VideoPairLinks{Job: job, Pair: p, OriginalURL: original, AnalyzedURL: analyzed, ExpiresAt: expires}, nil
}

// OutputURL signs a read of one published output of a succeeded job, such as
// its trajectory for drawing the track.
func (s *Service) OutputURL(ctx context.Context, actor access.Actor, id, kind string) (ReadLink, error) {
	_, published, err := s.succeeded(ctx, actor, id)
	if err != nil {
		return ReadLink{}, err
	}
	for _, o := range published.Artifacts {
		if o.Kind != kind {
			continue
		}
		expires := s.now().Add(s.settings.ReadTTL)
		url, err := s.objects.ReadURL(ctx, o.ObjectName, o.Generation, expires)
		if err != nil {
			return ReadLink{}, fmt.Errorf("sign output read: %w", err)
		}
		return ReadLink{URL: url, ExpiresAt: expires}, nil
	}
	return ReadLink{}, ErrOutputNotFound
}

// Occupancy reads the published trajectory of a succeeded job and maps it
// onto the arena floor with the job's pinned parameters.
func (s *Service) Occupancy(ctx context.Context, actor access.Actor, id string) (Occupancy, error) {
	job, published, err := s.succeeded(ctx, actor, id)
	if err != nil {
		return Occupancy{}, err
	}
	for _, o := range published.Artifacts {
		if o.Kind != domain.Trajectory {
			continue
		}
		data, err := s.objects.Read(ctx, o.ObjectName, o.Generation, s.settings.MaxTrajectoryBytes)
		if err != nil {
			return Occupancy{}, fmt.Errorf("read trajectory: %w", err)
		}
		track, err := domain.ReadTrajectory(data, job)
		if err != nil {
			return Occupancy{}, err
		}
		return s.catalog.Occupancy(job.ParadigmKey, job.ParadigmVersion, job.Parameters, track)
	}
	return Occupancy{}, ErrOutputNotFound
}

// succeeded reads a job a user may sign reads for.
func (s *Service) succeeded(ctx context.Context, actor access.Actor, id string) (domain.Job, Published, error) {
	if err := actor.Require(access.Read); err != nil {
		return domain.Job{}, Published{}, err
	}
	if s.objects == nil {
		return domain.Job{}, Published{}, ErrStorageNotConfigured
	}
	job, err := s.store.Job(ctx, id)
	if err != nil {
		return domain.Job{}, Published{}, err
	}
	if job.Status != domain.Succeeded {
		return domain.Job{}, Published{}, ErrJobNotSucceeded
	}
	published, err := s.store.Published(ctx, []string{job.ID})
	if err != nil {
		return domain.Job{}, Published{}, err
	}
	return job, published[job.ID], nil
}
