// Package application computes the dashboard summary: experiment, test,
// analysis and review counts, a short recent activity feed and the next
// planned tests. Everything here is a read; it never writes and needs only
// *:read. The one rule it applies itself, rather than reading from a column,
// is whether an animal filmed in a verified video still needs a calibration of
// its arena: that comes from the paradigm catalog, the same source the
// calibration context and the analysis scheduler use, so they never disagree.
package application

import (
	"context"

	access "github.com/HappyHackingSpace/Misko/backend/internal/access/domain"
	"github.com/HappyHackingSpace/Misko/backend/internal/dashboard/domain"
)

// CalibrationCandidate is an animal of a test with a verified current video
// whose arena has no valid effective calibration. Whether it actually needs
// one depends on its paradigm.
type CalibrationCandidate struct {
	ParadigmKey     string
	ParadigmVersion int
	Apparatus       map[string]float64
}

type Store interface {
	ExperimentStatusCounts(ctx context.Context) (domain.ExperimentCounts, error)
	TestStatusCounts(ctx context.Context) (domain.StatusCounts, error)
	AnalysisStatusCounts(ctx context.Context) (queued, running, failed int, err error)
	ResultStatusCounts(ctx context.Context) (domain.ResultCounts, error)
	RecentActivity(ctx context.Context, limit int) ([]domain.Activity, error)
	UpcomingTests(ctx context.Context, limit int) ([]domain.UpcomingTest, error)
	CalibrationCandidates(ctx context.Context) ([]CalibrationCandidate, error)
}

// Catalog reads calibration needs from the hardcoded paradigm definitions,
// the same question internal/calibration asks per recording.
type Catalog interface {
	CalibrationRequired(key string, version int, apparatus map[string]float64) (bool, error)
}

type Service struct {
	store   Store
	catalog Catalog
}

func New(store Store, catalog Catalog) *Service {
	return &Service{store: store, catalog: catalog}
}

// listLimit bounds the recent activity feed and the upcoming tests list: a
// dashboard card, not a report.
const listLimit = 5

func (s *Service) Summary(ctx context.Context, actor access.Actor) (domain.Summary, error) {
	if err := actor.Require(access.Read); err != nil {
		return domain.Summary{}, err
	}
	experiments, err := s.store.ExperimentStatusCounts(ctx)
	if err != nil {
		return domain.Summary{}, err
	}
	tests, err := s.store.TestStatusCounts(ctx)
	if err != nil {
		return domain.Summary{}, err
	}
	results, err := s.store.ResultStatusCounts(ctx)
	if err != nil {
		return domain.Summary{}, err
	}
	queued, running, failed, err := s.store.AnalysisStatusCounts(ctx)
	if err != nil {
		return domain.Summary{}, err
	}
	activity, err := s.store.RecentActivity(ctx, listLimit)
	if err != nil {
		return domain.Summary{}, err
	}
	upcoming, err := s.store.UpcomingTests(ctx, listLimit)
	if err != nil {
		return domain.Summary{}, err
	}
	candidates, err := s.store.CalibrationCandidates(ctx)
	if err != nil {
		return domain.Summary{}, err
	}
	waiting := 0
	for _, c := range candidates {
		// A candidate naming a paradigm version the catalog no longer carries
		// cannot be asked whether it needs calibration; it is left out of the
		// count rather than failing the whole dashboard over one stale row.
		required, err := s.catalog.CalibrationRequired(c.ParadigmKey, c.ParadigmVersion, c.Apparatus)
		if err != nil {
			continue
		}
		if required {
			waiting++
		}
	}
	return domain.Summary{
		Experiments:        experiments,
		Tests:              tests,
		Results:            results,
		AnalysisQueued:     queued,
		AnalysisRunning:    running,
		AnalysisFailed:     failed,
		CalibrationWaiting: waiting,
		RecentActivity:     activity,
		UpcomingTests:      upcoming,
	}, nil
}
