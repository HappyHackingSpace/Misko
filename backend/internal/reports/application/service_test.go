package application

import (
	"context"
	"errors"
	access "github.com/HappyHackingSpace/Misko/backend/internal/access/domain"
	"github.com/HappyHackingSpace/Misko/backend/internal/reports/domain"
	"testing"
)

const id = "01a00000-0000-7000-8000-000000000001"

var viewer = access.Actor{UserID: id, Role: access.Viewer}

type fakeStore struct {
	filters []Filter
	metrics []domain.MetricRow
	events  []domain.EventRow
}

func (f *fakeStore) MetricRows(_ context.Context, filter Filter) ([]domain.MetricRow, error) {
	f.filters = append(f.filters, filter)
	return f.metrics, nil
}

func (f *fakeStore) CountMetricRows(_ context.Context, filter Filter) (int, error) {
	f.filters = append(f.filters, filter)
	return len(f.metrics), nil
}

func (f *fakeStore) EventRows(_ context.Context, filter Filter) ([]domain.EventRow, error) {
	f.filters = append(f.filters, filter)
	return f.events, nil
}

func (f *fakeStore) CountEventRows(_ context.Context, filter Filter) (int, error) {
	f.filters = append(f.filters, filter)
	return len(f.events), nil
}

func metric(test string, engine int) domain.MetricRow {
	value := 1.0
	return domain.MetricRow{Provenance: domain.Provenance{TestID: test, SubjectID: test, ParadigmKey: "OPEN_FIELD", ParadigmVersion: 1, MetricEngineVersion: engine}, MetricKey: "distance_cm", Unit: "cm", Value: &value}
}

func TestEveryReportNeedsReadPermission(t *testing.T) {
	store := &fakeStore{}
	s := New(store, 10)
	anonymous := access.Actor{}
	calls := map[string]func() error{
		"metrics":        func() error { _, err := s.Metrics(context.Background(), anonymous, Query{}); return err },
		"export metrics": func() error { _, err := s.ExportMetrics(context.Background(), anonymous, Query{}); return err },
		"events":         func() error { _, err := s.Events(context.Background(), anonymous, Query{}); return err },
		"export events":  func() error { _, err := s.ExportEvents(context.Background(), anonymous, Query{}); return err },
		"summary": func() error {
			_, err := s.Summary(context.Background(), anonymous, Query{ExperimentID: id, MetricKey: "distance_cm"})
			return err
		},
	}
	for name, call := range calls {
		if err := call(); err == nil {
			t.Errorf("%s: anonymous actor was allowed", name)
		}
	}
	if len(store.filters) != 0 {
		t.Fatal("the store was queried without permission")
	}
	if _, err := s.Metrics(context.Background(), viewer, Query{}); err != nil {
		t.Fatalf("viewer: %v", err)
	}
}

func TestQueriesAreValidatedAndNormalized(t *testing.T) {
	for name, q := range map[string]Query{
		"malformed id":          {SubjectID: "subject-1"},
		"malformed disease id":  {DiseaseModelID: "1 OR 1=1"},
		"lowercase paradigm":    {ParadigmKey: "open_field"},
		"uppercase metric":      {MetricKey: "DISTANCE"},
		"negative version":      {ParadigmVersion: -1},
		"negative engine":       {MetricEngineVersion: -1},
		"unknown selection":     {Selection: "newest"},
		"unknown sort":          {Sort: "id"},
		"unknown order":         {Order: "up"},
		"negative page":         {Page: -1},
		"negative page size":    {PageSize: -1},
		"page beyond the limit": {Page: 1<<20 + 1},
	} {
		if _, err := New(&fakeStore{}, 10).Metrics(context.Background(), viewer, q); !errors.Is(err, ErrInvalidQuery) {
			t.Errorf("%s: %v", name, err)
		}
	}
	if _, err := New(&fakeStore{}, 10).Events(context.Background(), viewer, Query{Sort: "value"}); !errors.Is(err, ErrInvalidQuery) {
		t.Fatalf("metric sort on events: %v", err)
	}

	store := &fakeStore{}
	s := New(store, 10)
	page, err := s.Metrics(context.Background(), viewer, Query{ExperimentID: id, EventType: "immobile", PageSize: 500, Page: 3})
	f := store.filters[0]
	if err != nil || !f.LatestOnly || f.Sort != "scheduledAt" || f.Descending || f.Limit != 100 || f.Offset != 200 || f.EventType != "" || page.Page != 3 || page.PageSize != 100 {
		t.Fatalf("defaults: %+v %+v %v", f, page, err)
	}
	store.filters = nil
	if _, err := s.Events(context.Background(), viewer, Query{MetricKey: "distance_cm", Order: "desc", Sort: "eventType"}); err != nil {
		t.Fatal(err)
	}
	if f := store.filters[0]; f.MetricKey != "" || !f.Descending || f.Limit != 20 || f.Offset != 0 {
		t.Fatalf("event filter: %+v", f)
	}
	for q, latest := range map[Query]bool{{Selection: "all"}: false, {Selection: "latest"}: true, {RunID: id}: false} {
		store.filters = nil
		if _, err := s.Metrics(context.Background(), viewer, q); err != nil || store.filters[0].LatestOnly != latest {
			t.Errorf("%+v: latest=%v %v", q, store.filters[0].LatestOnly, err)
		}
	}
}

func TestExportsAndSummariesReadAtMostTheLimit(t *testing.T) {
	store := &fakeStore{metrics: []domain.MetricRow{metric("t1", 1), metric("t2", 1)}, events: []domain.EventRow{{}, {}}}
	s := New(store, 2)
	rows, err := s.ExportMetrics(context.Background(), viewer, Query{Page: 4, PageSize: 1})
	if err != nil || len(rows) != 2 || store.filters[0].Limit != 3 || store.filters[0].Offset != 0 {
		t.Fatalf("export: %d %+v %v", len(rows), store.filters, err)
	}
	store.metrics = append(store.metrics, metric("t3", 1))
	if _, err := s.ExportMetrics(context.Background(), viewer, Query{}); !errors.Is(err, ErrExportTooLarge) {
		t.Fatalf("metric export over the limit: %v", err)
	}
	if _, err := s.Summary(context.Background(), viewer, Query{ExperimentID: id, MetricKey: "distance_cm"}); !errors.Is(err, ErrExportTooLarge) {
		t.Fatalf("summary over the limit: %v", err)
	}
	store.events = append(store.events, domain.EventRow{})
	if _, err := s.ExportEvents(context.Background(), viewer, Query{}); !errors.Is(err, ErrExportTooLarge) {
		t.Fatalf("event export over the limit: %v", err)
	}
}

func TestSummaryScope(t *testing.T) {
	store := &fakeStore{metrics: []domain.MetricRow{metric("t1", 1)}}
	s := New(store, 10)
	for name, q := range map[string]Query{
		"no experiment": {MetricKey: "distance_cm"},
		"no metric":     {ExperimentID: id},
		"a single run":  {ExperimentID: id, MetricKey: "distance_cm", RunID: id},
		"every run":     {ExperimentID: id, MetricKey: "distance_cm", Selection: "all"},
	} {
		if _, err := s.Summary(context.Background(), viewer, q); !errors.Is(err, ErrSummaryScope) {
			t.Errorf("%s: %v", name, err)
		}
	}
	summary, err := s.Summary(context.Background(), viewer, Query{ExperimentID: id, MetricKey: "distance_cm", Sort: "bogus", Page: 9})
	if err != nil || len(summary.Groups) != 1 || !store.filters[0].LatestOnly || store.filters[0].Limit != 11 || store.filters[0].MetricKey != "distance_cm" {
		t.Fatalf("summary: %+v %+v %v", summary, store.filters, err)
	}
	store.metrics = append(store.metrics, metric("t1", 2))
	if _, err := s.Summary(context.Background(), viewer, Query{ExperimentID: id, MetricKey: "distance_cm"}); !errors.Is(err, domain.ErrIncompatibleVersions) {
		t.Fatalf("mixed engine versions: %v", err)
	}
}
