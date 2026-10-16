package application

import (
	"context"
	"errors"
	"testing"

	access "github.com/HappyHackingSpace/Misko/backend/internal/access/domain"
	"github.com/HappyHackingSpace/Misko/backend/internal/reports/domain"
)

const experimentID = "01a00000-0000-7000-8000-0000000000e1"

type fakeStore struct {
	rows  []domain.Measurement
	reads int
}

func (f *fakeStore) Experiment(_ context.Context, id string) (domain.Experiment, error) {
	if id != experimentID {
		return domain.Experiment{}, ErrExperimentNotFound
	}
	return domain.Experiment{ID: id, Code: "E1"}, nil
}

func (f *fakeStore) Groups(context.Context, string) ([]domain.Group, error) {
	return []domain.Group{{ID: "g", Role: domain.HealthyControl}}, nil
}

func (f *fakeStore) Timepoints(context.Context, string) ([]domain.Timepoint, error) {
	return []domain.Timepoint{{ID: "tp"}}, nil
}

func (f *fakeStore) Endpoints(context.Context, string) ([]domain.Endpoint, error) {
	return []domain.Endpoint{
		{ID: "01a00000-0000-7000-8000-0000000000a1", TimepointID: "tp", ParadigmKey: "MWM", MetricKey: "path_length_cm"},
		{ID: "01a00000-0000-7000-8000-0000000000a2", TimepointID: "tp", ParadigmKey: "MWM", MetricKey: "path_length_cm", Kind: "SECONDARY"},
	}, nil
}

func (f *fakeStore) Measurements(_ context.Context, _, _, _ string, limit int) ([]domain.Measurement, error) {
	f.reads++
	if len(f.rows) > limit {
		return f.rows[:limit], nil
	}
	return f.rows, nil
}

func TestReportReadsEachMetricOnceAndChecksScope(t *testing.T) {
	viewer := access.Actor{UserID: "v", Role: access.Viewer}
	store := &fakeStore{}
	s := New(store, 2)
	data, err := s.Report(context.Background(), viewer, experimentID, "")
	if err != nil || len(data.Report.Sections) != 2 || len(data.Measurements) != 2 || store.reads != 1 {
		t.Fatalf("report: %+v %v reads=%d", data, err, store.reads)
	}
	one, err := s.Report(context.Background(), viewer, experimentID, "01a00000-0000-7000-8000-0000000000a2")
	if err != nil || len(one.Report.Sections) != 1 || one.Report.Sections[0].Endpoint.Kind != "SECONDARY" {
		t.Fatalf("one endpoint: %+v %v", one, err)
	}
	for name, tc := range map[string]struct {
		actor                access.Actor
		experiment, endpoint string
		want                 error
	}{
		"anonymous":          {access.Actor{}, experimentID, "", access.ErrUnauthenticated},
		"malformed":          {viewer, "E1", "", ErrExperimentNotFound},
		"unknown":            {viewer, "01a00000-0000-7000-8000-0000000000ff", "", ErrExperimentNotFound},
		"malformed endpoint": {viewer, experimentID, "x", ErrInvalidQuery},
		"unknown endpoint":   {viewer, experimentID, "01a00000-0000-7000-8000-0000000000ff", ErrEndpointNotFound},
	} {
		if _, err := s.Report(context.Background(), tc.actor, tc.experiment, tc.endpoint); !errors.Is(err, tc.want) {
			t.Errorf("%s: %v want %v", name, err, tc.want)
		}
	}
	store.rows = make([]domain.Measurement, 3)
	if _, err := s.Report(context.Background(), viewer, experimentID, ""); !errors.Is(err, ErrExportTooLarge) {
		t.Fatalf("row limit: %v", err)
	}
}
