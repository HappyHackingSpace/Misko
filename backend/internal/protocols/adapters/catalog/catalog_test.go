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
		"unknown version":          {"ROTAROD", 99, nil, application.ErrUnknownParadigmVersion},
		"apparatus value":          {"OPEN_FIELD", 1, map[string]float64{"arena_width_cm": 40}, application.ErrInvalidSession},
		"out of range":             {"ROTAROD", 1, map[string]float64{"start_rpm": -1}, application.ErrInvalidSession},
		"cross-parameter rule":     {"ROTAROD", 1, map[string]float64{"start_rpm": 30, "end_rpm": 10}, application.ErrInvalidSession},
		"unknown session variable": {"ROTAROD", 1, map[string]float64{"speed": 1}, application.ErrInvalidSession},
	} {
		if _, err := c.ResolveSession(tc.key, tc.version, tc.session); !errors.Is(err, tc.want) {
			t.Errorf("%s: err=%v want %v", name, err, tc.want)
		}
	}
	session, err := c.ResolveSession("ROTAROD", 1, map[string]float64{"start_rpm": 5, "end_rpm": 30})
	if err != nil || session["start_rpm"] != 5 || session["end_rpm"] != 30 || session["max_trial_duration_s"] == 0 {
		t.Fatalf("resolved: %v %v", session, err)
	}
}
