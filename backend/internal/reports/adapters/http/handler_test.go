package reportshttp

import (
	"testing"

	"github.com/HappyHackingSpace/Misko/backend/internal/reports/application"
	"github.com/HappyHackingSpace/Misko/backend/internal/reports/domain"
)

func TestCellsNeverStartAFormula(t *testing.T) {
	for in, want := range map[string]string{"=SUM(A1)": "'=SUM(A1)", "+1": "+1", "-2.5": "-2.5", "@x": "'@x", "M-1": "M-1", "": ""} {
		if got := cell(in); got != want {
			t.Errorf("cell(%q) = %q want %q", in, got, want)
		}
	}
	micro := int64(2_500_000)
	if dose(&micro) != "2.5" || dose(nil) != "" {
		t.Errorf("dose: %q", dose(&micro))
	}
}

func TestAnimalRowsCoverTheEndpointAndItsBaseline(t *testing.T) {
	v := 12.5
	d := application.Data{
		Report: domain.Report{
			Experiment: domain.Experiment{Code: "=E1"},
			Groups:     []domain.Group{{ID: "g", Code: "G1", Role: domain.HealthyControl}},
			Timepoints: []domain.Timepoint{{ID: "b", Name: "Baseline"}, {ID: "t", Name: "Day 7", StudyDay: 7}, {ID: "x", Name: "Day 28"}},
			Sections:   []domain.Section{{Endpoint: domain.Endpoint{ID: "e", TimepointID: "t", BaselineTimepointID: "b", MetricKey: "distance_cm"}}},
		},
		Measurements: [][]domain.Measurement{{
			{GroupID: "g", TimepointID: "b", SubjectCode: "M1", ResultStatus: "ACCEPTED", HasMetric: true, Value: &v, Unit: "cm", MetricEngineVersion: 1},
			{GroupID: "g", TimepointID: "t", SubjectCode: "M1", ResultStatus: "PENDING"},
			{GroupID: "g", TimepointID: "x", SubjectCode: "M1"},
		}},
	}
	header, records := animalRecords(d)
	if len(records) != 2 {
		t.Fatalf("records: %v", records)
	}
	col := map[string]int{}
	for i, h := range header {
		col[h] = i
	}
	if records[0][col["experimentCode"]] != "'=E1" || records[0][col["baseline"]] != "true" || records[0][col["value"]] != "12.5" ||
		records[1][col["timepoint"]] != "Day 7" || records[1][col["value"]] != "" || records[1][col["resultStatus"]] != "PENDING" {
		t.Fatalf("records: %v", records)
	}
	for _, r := range records {
		if len(r) != len(header) {
			t.Fatalf("row width %d, header %d", len(r), len(header))
		}
	}
}
