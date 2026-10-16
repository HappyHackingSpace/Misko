package catalog

import (
	"errors"
	"testing"

	"github.com/HappyHackingSpace/Misko/backend/internal/scoring/application"
	"github.com/HappyHackingSpace/Misko/backend/internal/scoring/domain"
)

func TestTheEngineComputesScoredEvents(t *testing.T) {
	c := New()
	treadmill, err := c.Paradigm("TREADMILL", 1)
	if err != nil || !treadmill.ObserverScored || len(treadmill.Events) != 2 || !treadmill.Events[0].Single || treadmill.Events[1].Single || treadmill.Events[0].Label.TR == "" {
		t.Fatalf("treadmill: %+v %v", treadmill, err)
	}
	out, err := c.Evaluate(treadmill, map[string]float64{"accelerating": 0, "start_speed_cm_s": 10}, domain.Scoring{ScoredTypes: []string{"exhaustion"}, DurationUs: 1_800_000_000,
		Events: []domain.TrialEvent{{Type: "exhaustion", StartUs: 42_000_000, EndUs: 42_000_000}}})
	if err != nil || out.MetricEngineVersion < 1 || len(out.Metrics) != 4 || *out.Metrics[0].Value != 42 || len(out.Events) != 1 || out.Events[0].Kind != domain.Point {
		t.Fatalf("treadmill metrics: %+v %v", out, err)
	}
	if tracked, _ := c.Paradigm("MWM", 1); tracked.ObserverScored {
		t.Fatal("the water maze is tracked")
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
