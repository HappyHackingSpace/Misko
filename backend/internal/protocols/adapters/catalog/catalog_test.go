package catalog

import (
	"errors"
	paradigms "github.com/HappyHackingSpace/Misko/backend/internal/paradigms/domain"
	"github.com/HappyHackingSpace/Misko/backend/internal/protocols/application"
	"testing"
)

// Every paradigm version must accept an empty session: the apparatus defaults
// that stand in for an arena have to satisfy the version's own rules.
func TestEveryParadigmVersionResolvesAnEmptySession(t *testing.T) {
	c := New()
	for _, key := range paradigms.Keys() {
		versions, err := paradigms.Versions(key)
		if err != nil {
			t.Fatal(err)
		}
		for _, version := range versions {
			p, _ := paradigms.Version(key, version)
			session, err := c.ResolveSession(key, version, nil)
			if err != nil {
				t.Errorf("%s v%d: %v", key, version, err)
				continue
			}
			if len(session) != len(p.In(paradigms.SessionScope)) {
				t.Errorf("%s v%d: session %v misses defaults", key, version, session)
			}
		}
	}
}

func TestSessionValuesAreValidated(t *testing.T) {
	c := New()
	for name, tc := range map[string]struct {
		key     string
		version int
		session map[string]float64
		want    error
	}{
		"unknown paradigm":         {"MAZE", 1, nil, application.ErrUnknownParadigm},
		"unknown version":          {"TREADMILL", 99, nil, application.ErrUnknownParadigmVersion},
		"apparatus value":          {"TREADMILL", 1, map[string]float64{"lane_length_cm": 40}, application.ErrInvalidSession},
		"analysis value":           {"MWM", 1, map[string]float64{"max_sample_gap_s": 1}, application.ErrInvalidSession},
		"out of range":             {"TREADMILL", 1, map[string]float64{"start_speed_cm_s": -1}, application.ErrInvalidSession},
		"cross-parameter rule":     {"TREADMILL", 1, map[string]float64{"start_speed_cm_s": 30, "end_speed_cm_s": 10}, application.ErrInvalidSession},
		"unknown session variable": {"TREADMILL", 1, map[string]float64{"speed": 1}, application.ErrInvalidSession},
	} {
		if _, err := c.ResolveSession(tc.key, tc.version, tc.session); !errors.Is(err, tc.want) {
			t.Errorf("%s: err=%v want %v", name, err, tc.want)
		}
	}
	session, err := c.ResolveSession("TREADMILL", 1, map[string]float64{"start_speed_cm_s": 5, "end_speed_cm_s": 30})
	if err != nil || session["start_speed_cm_s"] != 5 || session["end_speed_cm_s"] != 30 || session["max_trial_duration_s"] == 0 {
		t.Fatalf("resolved: %v %v", session, err)
	}
}
