package catalog

import (
	"errors"
	"testing"

	"github.com/HappyHackingSpace/Misko/backend/internal/scoring/application"
	"github.com/HappyHackingSpace/Misko/backend/internal/scoring/domain"
)

func TestTheEngineComputesScoredEvents(t *testing.T) {
	c := New()
	rotarod, err := c.Paradigm("ROTAROD", 1)
	if err != nil || !rotarod.ObserverScored || len(rotarod.Events) != 1 || !rotarod.Events[0].Single || rotarod.Events[0].Label.TR == "" {
		t.Fatalf("rotarod: %+v %v", rotarod, err)
	}
	out, err := c.Evaluate(rotarod, map[string]float64{"accelerating": 0, "start_rpm": 10}, domain.Scoring{ScoredTypes: []string{"fall"}, DurationUs: 300_000_000,
		Events: []domain.TrialEvent{{Type: "fall", StartUs: 42_000_000, EndUs: 42_000_000}}})
	if err != nil || out.MetricEngineVersion < 1 || len(out.Metrics) != 3 || *out.Metrics[0].Value != 42 || len(out.Events) != 1 || out.Events[0].Kind != domain.Point {
		t.Fatalf("rotarod metrics: %+v %v", out, err)
	}
	if tracked, _ := c.Paradigm("OPEN_FIELD", 1); tracked.ObserverScored {
		t.Fatal("open field is tracked")
	}
	pole, _ := c.Paradigm("POLE", 1)
	_, err = c.Evaluate(pole, nil, domain.Scoring{ScoredTypes: pole.Types(), Events: []domain.TrialEvent{
		{Type: "base_reached", StartUs: 1, EndUs: 1}, {Type: "turn_complete", StartUs: 2, EndUs: 2}}})
	if !errors.Is(err, application.ErrRejectedByEngine) {
		t.Fatalf("pole order: %v", err)
	}
	if _, err := c.Paradigm("NOPE", 1); err == nil {
		t.Fatal("unknown paradigm")
	}
}
