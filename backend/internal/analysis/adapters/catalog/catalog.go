// Package catalog reads the analysis contract of a paradigm version from the
// hardcoded paradigm catalog and runs its metric engine. The Go engine is the
// only source of published metrics and events.
package catalog

import (
	"errors"
	"fmt"
	"github.com/HappyHackingSpace/Misko/backend/internal/analysis/application"
	"github.com/HappyHackingSpace/Misko/backend/internal/analysis/domain"
	paradigms "github.com/HappyHackingSpace/Misko/backend/internal/paradigms/domain"
	"slices"
)

type Catalog struct{}

func New() Catalog { return Catalog{} }

func (Catalog) Contract(key string, version int) (domain.Contract, error) {
	p, err := paradigms.Version(key, version)
	if errors.Is(err, paradigms.ErrUnknownParadigm) || errors.Is(err, paradigms.ErrUnknownVersion) {
		return domain.Contract{}, fmt.Errorf("%w: %s v%d", application.ErrUnknownCapability, key, version)
	}
	if err != nil {
		return domain.Contract{}, err
	}
	needs, err := paradigms.CalibrationRequirement(key, version, nil)
	if err != nil {
		return domain.Contract{}, err
	}
	c := domain.Contract{
		MetricEngineVersion: paradigms.MetricEngineVersion, ResultSchemaVersion: paradigms.ResultSchemaVersion,
		CalibrationRequired: needs.Required, MissingReasons: paradigms.MissingReasons(),
		// A trajectory supplies samples only: no scored observations and no
		// calibrated zone shapes.
		TrajectoryAnalyzable: len(p.InputEvents) == 0 && !slices.ContainsFunc(p.Zones, func(z paradigms.Zone) bool { return z.Calibrated && z.Required }),
	}
	for _, m := range p.Metrics {
		c.Metrics = append(c.Metrics, domain.MetricSpec{Key: m.Key, Unit: string(m.Unit), Integer: m.Integer, Min: m.Min, Max: m.Max})
	}
	for _, e := range p.Events {
		c.Events = append(c.Events, domain.EventSpec{Type: e.Type, Kind: string(e.Kind)})
	}
	for _, q := range p.QC {
		c.QC = append(c.QC, domain.QCRule{Key: q.Key, Operator: q.Operator, Value: q.Value})
	}
	return c, nil
}

// Evaluate computes metrics and events of a trajectory with the run's pinned
// parameters. Samples the engine rejects are ErrInvalidTrajectory.
func (Catalog) Evaluate(key string, version int, parameters map[string]float64, t domain.Track, durationUs int64) (domain.Computed, error) {
	samples := make([]paradigms.Sample, len(t.Samples))
	for i, s := range t.Samples {
		samples[i] = paradigms.Sample{TUs: s.TUs, X: s.X, Y: s.Y, Valid: s.Tracked}
	}
	r, err := paradigms.Evaluate(key, version, paradigms.Input{Parameters: parameters, Samples: samples, DurationUs: durationUs})
	if errors.Is(err, paradigms.ErrInvalidSample) {
		return domain.Computed{}, fmt.Errorf("%w: %v", domain.ErrInvalidTrajectory, err)
	}
	if err != nil {
		return domain.Computed{}, fmt.Errorf("evaluate %s v%d: %w", key, version, err)
	}
	var c domain.Computed
	for _, m := range r.Metrics {
		out := domain.MetricValue{Key: m.Key, Unit: string(m.Unit), MissingReason: m.Reason}
		if !m.Missing {
			value := m.Value
			out.Value, out.MissingReason = &value, ""
		}
		c.Metrics = append(c.Metrics, out)
	}
	for _, i := range r.Intervals {
		c.Intervals = append(c.Intervals, domain.Interval{Type: i.Type, StartUs: i.StartUs, EndUs: i.EndUs})
	}
	for _, p := range r.Points {
		c.Points = append(c.Points, domain.Point{Type: p.Type, AtUs: p.AtUs})
	}
	return c, nil
}
