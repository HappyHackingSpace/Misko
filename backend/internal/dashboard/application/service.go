// Package application computes the dashboard summary: counts, a short recent
// activity feed and the next planned tests. Everything here is a read; it
// never writes and needs only *:read. The one rule it applies itself, rather
// than reading from a column, is whether a candidate recording still needs a
// calibration — that comes from the paradigm catalog, the same source the
// single-recording calibration status uses, so the two never disagree.
package application

import (
	"context"

	access "github.com/HappyHackingSpace/Misko/backend/internal/access/domain"
	"github.com/HappyHackingSpace/Misko/backend/internal/dashboard/domain"
)

// CalibrationCandidate is a verified recording with no valid, unsuperseded
// calibration yet. Whether it actually needs one depends on its paradigm.
type CalibrationCandidate struct {
	ParadigmKey     string
	ParadigmVersion int
	Apparatus       map[string]float64
}

type Store interface {
	TestStatusCounts(ctx context.Context) (domain.StatusCounts, error)
	AnalysisStatusCounts(ctx context.Context) (queued, running, failed int, err error)
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
	tests, err := s.store.TestStatusCounts(ctx)
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
		Tests:              tests,
		AnalysisQueued:     queued,
		AnalysisRunning:    running,
		AnalysisFailed:     failed,
		CalibrationWaiting: waiting,
		RecentActivity:     activity,
		UpcomingTests:      upcoming,
	}, nil
}
