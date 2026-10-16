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
	"strings"
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
		// A trajectory supplies samples and the pinned calibration supplies
		// calibrated zone shapes. Observer-scored paradigms compute nothing
		// from samples and stay worker-free; optional scored events of a
		// tracked paradigm are left missing as unscored.
		TrajectoryAnalyzable: tracked(p),
		ObserverScored:       p.ObserverScored(),
	}
	for _, z := range p.Zones {
		if z.Calibrated && z.Required {
			c.RequiredZones = append(c.RequiredZones, z.Key)
		}
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

// tracked reports whether any metric of the paradigm reads tracked samples.
func tracked(p paradigms.Paradigm) bool {
	return slices.ContainsFunc(p.Metrics, func(m paradigms.MetricDefinition) bool {
		return slices.ContainsFunc(m.Inputs, func(input string) bool { return strings.HasPrefix(input, "samples") })
	})
}

// Evaluate computes metrics and events of a trajectory with the run's pinned
// parameters and the centimetre shapes of its calibration's zones. Samples the
// engine rejects are ErrInvalidTrajectory.
func (Catalog) Evaluate(key string, version int, parameters map[string]float64, t domain.Track, zones []domain.Zone, durationUs int64) (domain.Computed, error) {
	samples := make([]paradigms.Sample, len(t.Samples))
	for i, s := range t.Samples {
		samples[i] = paradigms.Sample{TUs: s.TUs, X: s.X, Y: s.Y, Valid: s.Tracked}
	}
	var shapes map[string][]paradigms.Shape
	for _, z := range zones {
		if shapes == nil {
			shapes = map[string][]paradigms.Shape{}
		}
		shapes[z.Key] = append(shapes[z.Key], toShape(z.Centimeters))
	}
	r, err := paradigms.Evaluate(key, version, paradigms.Input{Parameters: parameters, Samples: samples, Zones: shapes, DurationUs: durationUs})
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

func toShape(s domain.Shape) paradigms.Shape {
	out := paradigms.Shape{}
	if s.Circle != nil {
		out.Circle = &paradigms.Circle{X: s.Circle.X, Y: s.Circle.Y, Radius: s.Circle.Radius}
	}
	for _, v := range s.Polygon {
		out.Polygon = append(out.Polygon, paradigms.Vertex{X: v.X, Y: v.Y})
	}
	return out
}
