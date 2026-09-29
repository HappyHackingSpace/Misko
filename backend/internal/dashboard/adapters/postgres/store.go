// Package postgres reads the dashboard summary straight from the tables the
// tests, analysis and calibration domains own. It never writes.
package postgres

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/HappyHackingSpace/Misko/backend/internal/dashboard/adapters/postgres/sqlcgen"
	"github.com/HappyHackingSpace/Misko/backend/internal/dashboard/application"
	"github.com/HappyHackingSpace/Misko/backend/internal/dashboard/domain"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct {
	queries *sqlcgen.Queries
}

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{queries: sqlcgen.New(pool)}
}

func (s *Store) TestStatusCounts(ctx context.Context) (domain.StatusCounts, error) {
	rows, err := s.queries.TestStatusCounts(ctx)
	if err != nil {
		return domain.StatusCounts{}, fmt.Errorf("test status counts: %w", err)
	}
	var counts domain.StatusCounts
	for _, row := range rows {
		switch row.Status {
		case "PLANNED":
			counts.Planned = int(row.Total)
		case "IN_PROGRESS":
			counts.InProgress = int(row.Total)
		case "COMPLETED":
			counts.Completed = int(row.Total)
		case "CANCELLED":
			counts.Cancelled = int(row.Total)
		}
	}
	return counts, nil
}

func (s *Store) AnalysisStatusCounts(ctx context.Context) (queued, running, failed int, err error) {
	rows, err := s.queries.AnalysisStatusCounts(ctx)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("analysis status counts: %w", err)
	}
	for _, row := range rows {
		switch row.Status {
		case "QUEUED":
			queued = int(row.Total)
		case "RUNNING":
			running = int(row.Total)
		case "FAILED":
			failed = int(row.Total)
		}
	}
	return queued, running, failed, nil
}

func (s *Store) RecentActivity(ctx context.Context, limit int) ([]domain.Activity, error) {
	rows, err := s.queries.RecentSucceededRuns(ctx, int32(limit))
	if err != nil {
		return nil, fmt.Errorf("recent activity: %w", err)
	}
	out := make([]domain.Activity, 0, len(rows))
	for _, row := range rows {
		activity := domain.Activity{
			TestID: row.TestID, SubjectCode: row.SubjectCode, ExperimentCode: row.ExperimentCode,
			ParadigmKey: row.ParadigmKey, ParadigmVersion: int(row.ParadigmVersion),
		}
		if row.FinishedAt != nil {
			activity.FinishedAt = row.FinishedAt.UTC()
		}
		out = append(out, activity)
	}
	return out, nil
}

func (s *Store) UpcomingTests(ctx context.Context, limit int) ([]domain.UpcomingTest, error) {
	rows, err := s.queries.UpcomingTests(ctx, int32(limit))
	if err != nil {
		return nil, fmt.Errorf("upcoming tests: %w", err)
	}
	out := make([]domain.UpcomingTest, 0, len(rows))
	for _, row := range rows {
		out = append(out, domain.UpcomingTest{
			ID: row.ID, SubjectCode: row.SubjectCode, ExperimentCode: row.ExperimentCode,
			ParadigmKey: row.ParadigmKey, ParadigmVersion: int(row.ParadigmVersion),
			ScheduledAt: row.ScheduledAt.UTC(),
		})
	}
	return out, nil
}

func (s *Store) CalibrationCandidates(ctx context.Context) ([]application.CalibrationCandidate, error) {
	rows, err := s.queries.RecordingsAwaitingCalibrationCheck(ctx)
	if err != nil {
		return nil, fmt.Errorf("calibration candidates: %w", err)
	}
	out := make([]application.CalibrationCandidate, 0, len(rows))
	for _, row := range rows {
		var apparatus map[string]float64
		if err := json.Unmarshal(row.Apparatus, &apparatus); err != nil {
			return nil, fmt.Errorf("decode apparatus: %w", err)
		}
		out = append(out, application.CalibrationCandidate{
			ParadigmKey: row.ParadigmKey, ParadigmVersion: int(row.ParadigmVersion), Apparatus: apparatus,
		})
	}
	return out, nil
}
