package catalog

import (
	"slices"
	"testing"
)

func TestMetricsComeFromTheParadigmVersion(t *testing.T) {
	metrics, err := New().Metrics("MWM", 1)
	if err != nil || !slices.Contains(metrics, "escape_latency_s") {
		t.Fatalf("metrics=%v err=%v", metrics, err)
	}
	if _, err := New().Metrics("MWM", 999); err == nil {
		t.Fatal("unknown version accepted")
	}
	if _, err := New().Metrics("NOT_A_PARADIGM", 1); err == nil {
		t.Fatal("unknown paradigm accepted")
	}
}
