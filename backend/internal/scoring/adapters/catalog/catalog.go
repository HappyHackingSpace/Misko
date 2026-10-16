// Package catalog reads the input events of a paradigm version from the
// hardcoded paradigm catalog and runs its metric engine on scored events.
package catalog

import (
	"errors"
	"fmt"
	paradigms "github.com/HappyHackingSpace/Misko/backend/internal/paradigms/domain"
	"github.com/HappyHackingSpace/Misko/backend/internal/scoring/application"
	"github.com/HappyHackingSpace/Misko/backend/internal/scoring/domain"
	"slices"
)

type Catalog struct{}

func New() Catalog { return Catalog{} }

func (Catalog) Paradigm(key string, version int) (domain.Paradigm, error) {
	p, err := paradigms.Version(key, version)
	if err != nil {
		return domain.Paradigm{}, fmt.Errorf("paradigm %s v%d: %w", key, version, err)
	}
	out := domain.Paradigm{Key: p.Key, Version: p.Version, ObserverScored: p.ObserverScored()}
	for _, e := range p.InputEvents {
		out.Events = append(out.Events, domain.EventDefinition{
			Type: e.Type, Kind: domain.Kind(e.Kind), Labels: slices.Clone(e.Labels), Single: e.Single,
			Label: domain.Text{EN: e.Label.EN, TR: e.Label.TR}, Definition: domain.Text{EN: e.Definition.EN, TR: e.Definition.TR},
		})
	}
	return out, nil
}

// Evaluate computes metrics and events from scored events in microseconds from
// the clip start, with the arena and session parameters.
func (Catalog) Evaluate(p domain.Paradigm, parameters map[string]float64, s domain.Scoring) (domain.Computed, error) {
	events := make([]paradigms.ObservedEvent, len(s.Events))
	for i, e := range s.Events {
		events[i] = paradigms.ObservedEvent{Type: e.Type, Label: e.Label, StartUs: e.StartUs, EndUs: e.EndUs}
	}
	r, err := paradigms.Evaluate(p.Key, p.Version, paradigms.Input{Parameters: parameters, Events: events, ScoredEvents: s.ScoredTypes, DurationUs: s.DurationUs})
	if errors.Is(err, paradigms.ErrInvalidEvent) {
		return domain.Computed{}, fmt.Errorf("%w: %v", application.ErrRejectedByEngine, err)
	}
	if err != nil {
		return domain.Computed{}, fmt.Errorf("evaluate %s v%d: %w", p.Key, p.Version, err)
	}
	c := domain.Computed{MetricEngineVersion: r.MetricEngineVersion, ResultSchemaVersion: r.ResultSchemaVersion}
	for _, m := range r.Metrics {
		out := domain.MetricValue{Key: m.Key, Unit: string(m.Unit), MissingReason: m.Reason}
		if !m.Missing {
			value := m.Value
			out.Value, out.MissingReason = &value, ""
		}
		c.Metrics = append(c.Metrics, out)
	}
	for _, i := range r.Intervals {
		c.Events = append(c.Events, domain.DerivedEvent{Type: i.Type, Kind: domain.Interval, StartUs: i.StartUs, EndUs: i.EndUs})
	}
	for _, p := range r.Points {
		c.Events = append(c.Events, domain.DerivedEvent{Type: p.Type, Kind: domain.Point, StartUs: p.AtUs, EndUs: p.AtUs})
	}
	return c, nil
}
