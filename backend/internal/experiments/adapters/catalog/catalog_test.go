package catalog

import (
	"slices"
	"testing"
)

func TestMetricsComeFromTheParadigmVersion(t *testing.T) {
	metrics, err := New().Metrics("OPEN_FIELD", 1)
	if err != nil || !slices.Contains(metrics, "distance_cm") {
		t.Fatalf("metrics=%v err=%v", metrics, err)
	}
	if _, err := New().Metrics("OPEN_FIELD", 999); err == nil {
		t.Fatal("unknown version accepted")
	}
	if _, err := New().Metrics("NOT_A_PARADIGM", 1); err == nil {
		t.Fatal("unknown paradigm accepted")
	}
}
