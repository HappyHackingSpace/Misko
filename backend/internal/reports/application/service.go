// Package application contains read-only report use cases. Every role with
// *:read may read reports; there is one laboratory per installation.
package application

import (
	"context"
	"errors"
	access "github.com/HappyHackingSpace/Misko/backend/internal/access/domain"
	"github.com/HappyHackingSpace/Misko/backend/internal/reports/domain"
	"regexp"
	"slices"
)

var (
	ErrInvalidQuery   = errors.New("invalid report query")
	ErrExportTooLarge = errors.New("the export exceeds the row limit; narrow the filters")
	ErrSummaryScope   = errors.New("a summary needs experimentId and metricKey and always uses the latest runs")
)

var (
	uuidPattern     = regexp.MustCompile(`^(?i)[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
	paradigmPattern = regexp.MustCompile(`^[A-Z][A-Z_]{0,39}$`)
	keyPattern      = regexp.MustCompile(`^[a-z][a-z0-9_]{0,79}$`)
)

// Query is a report request as received. Empty strings and zero versions do not
// filter. Selection is "latest" (default) or "all".
type Query struct {
	ExperimentID, SubjectID, GroupID, PhaseID, TestID string
	EnvironmentID, VideoID, RunID                     string
	DiseaseModelID, SubstanceID                       string
	ParadigmKey, MetricKey, EventType                 string
	ParadigmVersion, MetricEngineVersion              int
	Selection, Sort, Order                            string
	Page, PageSize                                    int
}

// Filter is a validated query. LatestOnly keeps the newest succeeded run of
// each test per metric engine version.
type Filter struct {
	ExperimentID, SubjectID, GroupID, PhaseID, TestID string
	EnvironmentID, VideoID, RunID                     string
	DiseaseModelID, SubstanceID                       string
	ParadigmKey, MetricKey, EventType                 string
	ParadigmVersion, MetricEngineVersion              int
	LatestOnly                                        bool
	Sort                                              string
	Descending                                        bool
	Limit, Offset                                     int
}

type Store interface {
	MetricRows(ctx context.Context, f Filter) ([]domain.MetricRow, error)
	CountMetricRows(ctx context.Context, f Filter) (int, error)
	EventRows(ctx context.Context, f Filter) ([]domain.EventRow, error)
	CountEventRows(ctx context.Context, f Filter) (int, error)
}

type Service struct {
	store       Store
	exportLimit int
}

// New returns the report service; exports and summaries read at most exportLimit rows.
func New(store Store, exportLimit int) *Service {
	return &Service{store: store, exportLimit: exportLimit}
}

type MetricPage struct {
	Rows                  []domain.MetricRow
	Total, Page, PageSize int
}

type EventPage struct {
	Rows                  []domain.EventRow
	Total, Page, PageSize int
}

var (
	metricSorts = []string{"scheduledAt", "subjectCode", "experimentCode", "metricKey", "value", "runFinishedAt"}
	eventSorts  = []string{"scheduledAt", "subjectCode", "eventType"}
)

func (s *Service) Metrics(ctx context.Context, actor access.Actor, q Query) (MetricPage, error) {
	if err := actor.Require(access.Read); err != nil {
		return MetricPage{}, err
	}
	q.EventType = ""
	f, page, err := validate(q, metricSorts)
	if err != nil {
		return MetricPage{}, err
	}
	rows, err := s.store.MetricRows(ctx, f)
	if err != nil {
		return MetricPage{}, err
	}
	total, err := s.store.CountMetricRows(ctx, f)
	if err != nil {
		return MetricPage{}, err
	}
	return MetricPage{Rows: rows, Total: total, Page: page, PageSize: f.Limit}, nil
}

// ExportMetrics returns every matching row in the page order, up to the export limit.
func (s *Service) ExportMetrics(ctx context.Context, actor access.Actor, q Query) ([]domain.MetricRow, error) {
	if err := actor.Require(access.Read); err != nil {
		return nil, err
	}
	q.EventType, q.Page, q.PageSize = "", 0, 0
	f, _, err := validate(q, metricSorts)
	if err != nil {
		return nil, err
	}
	f.Limit, f.Offset = s.exportLimit+1, 0
	rows, err := s.store.MetricRows(ctx, f)
	if err != nil {
		return nil, err
	}
	if len(rows) > s.exportLimit {
		return nil, ErrExportTooLarge
	}
	return rows, nil
}

func (s *Service) Events(ctx context.Context, actor access.Actor, q Query) (EventPage, error) {
	if err := actor.Require(access.Read); err != nil {
		return EventPage{}, err
	}
	q.MetricKey = ""
	f, page, err := validate(q, eventSorts)
	if err != nil {
		return EventPage{}, err
	}
	rows, err := s.store.EventRows(ctx, f)
	if err != nil {
		return EventPage{}, err
	}
	total, err := s.store.CountEventRows(ctx, f)
	if err != nil {
		return EventPage{}, err
	}
	return EventPage{Rows: rows, Total: total, Page: page, PageSize: f.Limit}, nil
}

func (s *Service) ExportEvents(ctx context.Context, actor access.Actor, q Query) ([]domain.EventRow, error) {
	if err := actor.Require(access.Read); err != nil {
		return nil, err
	}
	q.MetricKey, q.Page, q.PageSize = "", 0, 0
	f, _, err := validate(q, eventSorts)
	if err != nil {
		return nil, err
	}
	f.Limit, f.Offset = s.exportLimit+1, 0
	rows, err := s.store.EventRows(ctx, f)
	if err != nil {
		return nil, err
	}
	if len(rows) > s.exportLimit {
		return nil, ErrExportTooLarge
	}
	return rows, nil
}

// Summary compares the groups of one experiment on one metric using the latest
// run of each test. Results of different versions or units are refused.
func (s *Service) Summary(ctx context.Context, actor access.Actor, q Query) (domain.Summary, error) {
	if err := actor.Require(access.Read); err != nil {
		return domain.Summary{}, err
	}
	if q.ExperimentID == "" || q.MetricKey == "" || q.RunID != "" || (q.Selection != "" && q.Selection != "latest") {
		return domain.Summary{}, ErrSummaryScope
	}
	q.EventType, q.Sort, q.Order, q.Page, q.PageSize = "", "", "", 0, 0
	f, _, err := validate(q, metricSorts)
	if err != nil {
		return domain.Summary{}, err
	}
	f.Limit, f.Offset = s.exportLimit+1, 0
	rows, err := s.store.MetricRows(ctx, f)
	if err != nil {
		return domain.Summary{}, err
	}
	if len(rows) > s.exportLimit {
		return domain.Summary{}, ErrExportTooLarge
	}
	return domain.Summarize(q.MetricKey, rows)
}

func validate(q Query, sorts []string) (Filter, int, error) {
	f := Filter{
		ExperimentID: q.ExperimentID, SubjectID: q.SubjectID, GroupID: q.GroupID, PhaseID: q.PhaseID, TestID: q.TestID,
		EnvironmentID: q.EnvironmentID, VideoID: q.VideoID, RunID: q.RunID, DiseaseModelID: q.DiseaseModelID, SubstanceID: q.SubstanceID,
		ParadigmKey: q.ParadigmKey, MetricKey: q.MetricKey, EventType: q.EventType,
		ParadigmVersion: q.ParadigmVersion, MetricEngineVersion: q.MetricEngineVersion, Sort: q.Sort,
	}
	for _, id := range []string{f.ExperimentID, f.SubjectID, f.GroupID, f.PhaseID, f.TestID, f.EnvironmentID, f.VideoID, f.RunID, f.DiseaseModelID, f.SubstanceID} {
		if id != "" && !uuidPattern.MatchString(id) {
			return Filter{}, 0, ErrInvalidQuery
		}
	}
	if (f.ParadigmKey != "" && !paradigmPattern.MatchString(f.ParadigmKey)) || (f.MetricKey != "" && !keyPattern.MatchString(f.MetricKey)) ||
		(f.EventType != "" && !keyPattern.MatchString(f.EventType)) || f.ParadigmVersion < 0 || f.MetricEngineVersion < 0 {
		return Filter{}, 0, ErrInvalidQuery
	}
	switch q.Selection {
	case "", "latest":
		// An explicit run is reported even when a newer run of its test exists.
		f.LatestOnly = f.RunID == ""
	case "all":
	default:
		return Filter{}, 0, ErrInvalidQuery
	}
	if f.Sort == "" {
		f.Sort = "scheduledAt"
	}
	if !slices.Contains(sorts, f.Sort) {
		return Filter{}, 0, ErrInvalidQuery
	}
	switch q.Order {
	case "", "asc":
	case "desc":
		f.Descending = true
	default:
		return Filter{}, 0, ErrInvalidQuery
	}
	if q.Page < 0 || q.Page > 1<<20 || q.PageSize < 0 {
		return Filter{}, 0, ErrInvalidQuery
	}
	page, limit := max(q.Page, 1), min(q.PageSize, 100)
	if limit == 0 {
		limit = 20
	}
	f.Limit, f.Offset = limit, (page-1)*limit
	return f, page, nil
}
