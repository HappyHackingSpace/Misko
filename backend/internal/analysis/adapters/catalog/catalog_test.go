package catalog

import (
	"errors"
	"github.com/HappyHackingSpace/Misko/backend/internal/analysis/application"
	"github.com/HappyHackingSpace/Misko/backend/internal/analysis/domain"
	"slices"
	"testing"
)

func TestContractsComeFromThePublishedParadigmVersion(t *testing.T) {
	c, err := New().Contract("OPEN_FIELD", 1)
	if err != nil {
		t.Fatal(err)
	}
	if !c.CalibrationRequired || c.MetricEngineVersion != 1 || c.ResultSchemaVersion != 1 || len(c.Metrics) != 9 || len(c.Events) != 3 {
		t.Fatalf("OPEN_FIELD contract: %+v", c)
	}
	entries := c.Metrics[slices.IndexFunc(c.Metrics, func(m domain.MetricSpec) bool { return m.Key == "center_entries_count" })]
	if !entries.Integer || entries.Unit != "count" {
		t.Fatalf("integer metric: %+v", entries)
	}
	if !slices.Contains(c.MissingReasons, "NO_VALID_INTERVALS") || !slices.ContainsFunc(c.Events, func(e domain.EventSpec) bool { return e.Type == "center_entry" && e.Kind == "POINT" }) {
		t.Fatalf("reasons and events: %+v", c)
	}
	rotarod, err := New().Contract("ROTAROD", 1)
	if err != nil || rotarod.CalibrationRequired {
		t.Fatalf("ROTAROD: %+v %v", rotarod, err)
	}
	for _, tc := range []struct {
		key     string
		version int
	}{{"MAZE", 1}, {"OPEN_FIELD", 9}} {
		if _, err := New().Contract(tc.key, tc.version); !errors.Is(err, application.ErrUnknownCapability) {
			t.Errorf("%s v%d: %v", tc.key, tc.version, err)
		}
	}
}
