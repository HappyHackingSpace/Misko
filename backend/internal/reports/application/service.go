// Package application contains read-only report use cases. Every role with
// *:read may read reports; there is one laboratory per installation.
package application

import (
	"context"
	"errors"
	"regexp"

	access "github.com/HappyHackingSpace/Misko/backend/internal/access/domain"
	"github.com/HappyHackingSpace/Misko/backend/internal/reports/domain"
)

var (
	ErrInvalidQuery       = errors.New("invalid report query")
	ErrExperimentNotFound = errors.New("experiment not found")
	ErrEndpointNotFound   = errors.New("endpoint not found in this experiment")
	ErrExportTooLarge     = errors.New("the report reads more rows than the limit allows")
)

var uuidPattern = regexp.MustCompile(`^(?i)[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

type Store interface {
	// Experiment returns ErrExperimentNotFound for an unknown experiment.
	Experiment(ctx context.Context, id string) (domain.Experiment, error)
	Groups(ctx context.Context, experimentID string) ([]domain.Group, error)
	Timepoints(ctx context.Context, experimentID string) ([]domain.Timepoint, error)
	Endpoints(ctx context.Context, experimentID string) ([]domain.Endpoint, error)
	// Measurements returns at most limit animals filmed for one paradigm in
	// the experiment with their value of one metric.
	Measurements(ctx context.Context, experimentID, paradigmKey, metricKey string, limit int) ([]domain.Measurement, error)
}

type Service struct {
	store    Store
	rowLimit int
}

// New returns the report service; one endpoint reads at most rowLimit measurements.
func New(store Store, rowLimit int) *Service {
	return &Service{store: store, rowLimit: rowLimit}
}

// Data is a report with the measurements each section was computed from, in
// section order, for exports.
type Data struct {
	Report       domain.Report
	Measurements [][]domain.Measurement
}

// Report computes the report of every endpoint of an experiment, or of one
// endpoint when endpointID is set.
func (s *Service) Report(ctx context.Context, actor access.Actor, experimentID, endpointID string) (Data, error) {
	if err := actor.Require(access.Read); err != nil {
		return Data{}, err
	}
	if !uuidPattern.MatchString(experimentID) {
		return Data{}, ErrExperimentNotFound
	}
	if endpointID != "" && !uuidPattern.MatchString(endpointID) {
		return Data{}, ErrInvalidQuery
	}
	experiment, err := s.store.Experiment(ctx, experimentID)
	if err != nil {
		return Data{}, err
	}
	groups, err := s.store.Groups(ctx, experimentID)
	if err != nil {
		return Data{}, err
	}
	timepoints, err := s.store.Timepoints(ctx, experimentID)
	if err != nil {
		return Data{}, err
	}
	endpoints, err := s.store.Endpoints(ctx, experimentID)
	if err != nil {
		return Data{}, err
	}
	out := Data{Report: domain.Report{Experiment: experiment, Groups: groups, Timepoints: timepoints, Sections: []domain.Section{}}}
	cache := map[[2]string][]domain.Measurement{}
	for _, e := range endpoints {
		if endpointID != "" && e.ID != endpointID {
			continue
		}
		key := [2]string{e.ParadigmKey, e.MetricKey}
		rows, ok := cache[key]
		if !ok {
			if rows, err = s.store.Measurements(ctx, experimentID, e.ParadigmKey, e.MetricKey, s.rowLimit+1); err != nil {
				return Data{}, err
			}
			if len(rows) > s.rowLimit {
				return Data{}, ErrExportTooLarge
			}
			cache[key] = rows
		}
		out.Report.Sections = append(out.Report.Sections, domain.Analyse(e, groups, timepoints, rows))
		out.Measurements = append(out.Measurements, rows)
	}
	if endpointID != "" && len(out.Report.Sections) == 0 {
		return Data{}, ErrEndpointNotFound
	}
	return out, nil
}
