// Package reportshttp exposes read-only reports, their CSV and JSON exports and
// group comparisons.
package reportshttp

import (
	"encoding/csv"
	"errors"
	access "github.com/HappyHackingSpace/Misko/backend/internal/access/domain"
	"github.com/HappyHackingSpace/Misko/backend/internal/platform/httpjson"
	"github.com/HappyHackingSpace/Misko/backend/internal/reports/application"
	"github.com/HappyHackingSpace/Misko/backend/internal/reports/domain"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"
)

var failures = []httpjson.Failure{
	{Err: access.ErrUnauthenticated, Status: http.StatusUnauthorized, Code: "auth.unauthenticated"},
	{Err: access.ErrForbidden, Status: http.StatusForbidden, Code: "auth.forbidden"},
	{Err: application.ErrInvalidQuery, Status: http.StatusBadRequest, Code: "report.invalidQuery"},
	{Err: application.ErrSummaryScope, Status: http.StatusBadRequest, Code: "report.invalidSummary"},
	{Err: application.ErrExportTooLarge, Status: http.StatusBadRequest, Code: "report.exportTooLarge"},
	{Err: domain.ErrIncompatibleVersions, Status: http.StatusConflict, Code: "report.incompatibleVersions"},
}

type handler struct {
	service      *application.Service
	authenticate func(*http.Request) (access.Actor, error)
	logger       *slog.Logger
}

func Register(mux *http.ServeMux, service *application.Service, authenticate func(*http.Request) (access.Actor, error), logger *slog.Logger) {
	h := handler{service: service, authenticate: authenticate, logger: logger}
	mux.HandleFunc("GET /api/reports/metrics", h.with(h.metrics))
	mux.HandleFunc("GET /api/reports/metrics/export", h.with(h.exportMetrics))
	mux.HandleFunc("GET /api/reports/events", h.with(h.events))
	mux.HandleFunc("GET /api/reports/events/export", h.with(h.exportEvents))
	mux.HandleFunc("GET /api/reports/metric-summary", h.with(h.summary))
}

func (h handler) with(next func(http.ResponseWriter, *http.Request, access.Actor)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		actor, err := h.authenticate(r)
		if err != nil {
			h.fail(w, r, err)
			return
		}
		next(w, r, actor)
	}
}

func query(r *http.Request) application.Query {
	v := r.URL.Query()
	return application.Query{
		ExperimentID: v.Get("experimentId"), SubjectID: v.Get("subjectId"), GroupID: v.Get("groupId"), PhaseID: v.Get("phaseId"),
		TestID: v.Get("testId"), EnvironmentID: v.Get("environmentId"), VideoID: v.Get("videoId"), RunID: v.Get("runId"),
		DiseaseModelID: v.Get("diseaseModelId"), SubstanceID: v.Get("substanceId"), ParadigmKey: v.Get("paradigmKey"),
		MetricKey: v.Get("metricKey"), EventType: v.Get("eventType"),
		ParadigmVersion: httpjson.QueryInt(v.Get("paradigmVersion")), MetricEngineVersion: httpjson.QueryInt(v.Get("metricEngineVersion")),
		Selection: v.Get("selection"), Sort: v.Get("sort"), Order: v.Get("order"),
		Page: httpjson.QueryInt(v.Get("page")), PageSize: httpjson.QueryInt(v.Get("pageSize")),
	}
}

func (h handler) metrics(w http.ResponseWriter, r *http.Request, actor access.Actor) {
	page, err := h.service.Metrics(r.Context(), actor, query(r))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpjson.Write(w, http.StatusOK, pageJSON{mapSlice(page.Rows, toMetric), page.Total, page.Page, page.PageSize})
}

func (h handler) events(w http.ResponseWriter, r *http.Request, actor access.Actor) {
	page, err := h.service.Events(r.Context(), actor, query(r))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpjson.Write(w, http.StatusOK, pageJSON{mapSlice(page.Rows, toEvent), page.Total, page.Page, page.PageSize})
}

func (h handler) exportMetrics(w http.ResponseWriter, r *http.Request, actor access.Actor) {
	format, ok := h.format(w, r)
	if !ok {
		return
	}
	rows, err := h.service.ExportMetrics(r.Context(), actor, query(r))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	if format == "json" {
		httpjson.Write(w, http.StatusOK, dataJSON{mapSlice(rows, toMetric)})
		return
	}
	header := append(append([]string{}, provenanceHeader...), "metricKey", "unit", "value", "missingReason")
	records := make([][]string, 0, len(rows))
	for _, m := range rows {
		value := ""
		if m.Value != nil {
			value = strconv.FormatFloat(*m.Value, 'g', -1, 64)
		}
		records = append(records, append(provenanceCells(m.Provenance), cell(m.MetricKey), cell(m.Unit), value, cell(m.MissingReason)))
	}
	h.csv(w, r, "misko-metrics.csv", header, records)
}

func (h handler) exportEvents(w http.ResponseWriter, r *http.Request, actor access.Actor) {
	format, ok := h.format(w, r)
	if !ok {
		return
	}
	rows, err := h.service.ExportEvents(r.Context(), actor, query(r))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	if format == "json" {
		httpjson.Write(w, http.StatusOK, dataJSON{mapSlice(rows, toEvent)})
		return
	}
	header := append(append([]string{}, provenanceHeader...), "eventId", "eventType", "kind", "startUs", "endUs", "confidence",
		"trialId", "trialNumber", "trialRepetition", "trialAttempt")
	records := make([][]string, 0, len(rows))
	for _, e := range rows {
		trial := []string{"", "", "", ""}
		if e.TrialID != "" {
			trial = []string{e.TrialID, strconv.Itoa(e.TrialNumber), strconv.Itoa(e.TrialRepetition), strconv.Itoa(e.TrialAttempt)}
		}
		records = append(records, append(append(provenanceCells(e.Provenance), e.EventID, cell(e.EventType), e.Kind,
			strconv.FormatInt(e.StartUs, 10), strconv.FormatInt(e.EndUs, 10), strconv.FormatFloat(e.Confidence, 'g', -1, 64)), trial...))
	}
	h.csv(w, r, "misko-events.csv", header, records)
}

func (h handler) summary(w http.ResponseWriter, r *http.Request, actor access.Actor) {
	s, err := h.service.Summary(r.Context(), actor, query(r))
	var incompatible domain.IncompatibleError
	if errors.As(err, &incompatible) {
		// The versions come from stored results and tell the client which filters to add.
		httpjson.Error(w, http.StatusConflict, "report.incompatibleVersions", incompatible.Error())
		return
	}
	if err != nil {
		h.fail(w, r, err)
		return
	}
	body := summaryJSON{MetricKey: s.MetricKey, Groups: []groupJSON{}}
	if s.Version != nil {
		body.Version = &versionJSON{s.Version.ParadigmKey, s.Version.ParadigmVersion, s.Version.MetricEngineVersion, s.Version.Unit}
	}
	for _, g := range s.Groups {
		body.Groups = append(body.Groups, groupJSON{
			GroupID: nullable(g.GroupID), GroupName: nullable(g.GroupName), GroupRole: nullable(g.GroupRole), Tests: g.Tests, Subjects: g.Subjects,
			MissingSubjects: g.MissingSubjects, MissingReasons: g.MissingReasons, Mean: g.Mean, SD: g.SD, SEM: g.SEM, Min: g.Min, Max: g.Max,
		})
	}
	httpjson.Write(w, http.StatusOK, body)
}

func (h handler) format(w http.ResponseWriter, r *http.Request) (string, bool) {
	switch f := r.URL.Query().Get("format"); f {
	case "", "csv":
		return "csv", true
	case "json":
		return f, true
	}
	h.fail(w, r, application.ErrInvalidQuery)
	return "", false
}

func (h handler) csv(w http.ResponseWriter, r *http.Request, name string, header []string, records [][]string) {
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	out := csv.NewWriter(w)
	if r.URL.Query().Get("delimiter") == "semicolon" {
		// Excel ignores the regional list separator when the file starts with a sep= line, so the
		// same file opens correctly in every locale. A BOM makes it read the text as UTF-8.
		_, _ = w.Write([]byte(csvPreamble))
		out.Comma = ';'
	}
	if err := out.Write(header); err == nil {
		err = out.WriteAll(records)
	}
	if err := out.Error(); err != nil {
		h.logger.ErrorContext(r.Context(), "csv export interrupted", "route", r.Pattern, "error", err)
	}
}

func (h handler) fail(w http.ResponseWriter, r *http.Request, err error) {
	httpjson.Fail(w, r, h.logger, err, failures)
}

// cell keeps spreadsheet applications from evaluating stored text as a formula.
func cell(s string) string {
	if s != "" && strings.ContainsRune("=+-@\t\r", rune(s[0])) && !isNumber(s) {
		return "'" + s
	}
	return s
}

// csvPreamble is a UTF-8 BOM followed by the sep= hint Excel reads.
const csvPreamble = string(rune(0xFEFF)) + "sep=;\r\n"

func isNumber(s string) bool {
	_, err := strconv.ParseFloat(s, 64)
	return err == nil
}

var provenanceHeader = []string{
	"runId", "latest", "trigger", "modelVersion", "metricEngineVersion", "resultSchemaVersion", "calibrationId", "recordingId",
	"sourceVideoId", "runFinishedAt", "testId", "testStatus", "scheduledAt", "trialCount", "experimentId", "experimentCode",
	"subjectId", "subjectCode", "species", "sex", "phaseId", "phaseName", "groupId", "groupName", "groupRole",
	"paradigmKey", "paradigmVersion", "environmentId", "environmentName", "environmentRevision",
}

func provenanceCells(p domain.Provenance) []string {
	return []string{
		p.RunID, strconv.FormatBool(p.Latest), p.Trigger, cell(p.ModelVersion), strconv.Itoa(p.MetricEngineVersion), strconv.Itoa(p.ResultSchemaVersion),
		p.CalibrationID, p.RecordingID, p.SourceVideoID, timestamp(p.RunFinishedAt), p.TestID, p.TestStatus, timestamp(p.ScheduledAt),
		strconv.Itoa(p.TrialCount), p.ExperimentID, cell(p.ExperimentCode), p.SubjectID, cell(p.SubjectCode), p.Species, p.Sex,
		p.PhaseID, cell(p.PhaseName), p.GroupID, cell(p.GroupName), p.GroupRole, p.ParadigmKey, strconv.Itoa(p.ParadigmVersion),
		p.EnvironmentID, cell(p.EnvironmentName), strconv.Itoa(p.EnvironmentRevision),
	}
}

func timestamp(t time.Time) string { return t.UTC().Format(time.RFC3339Nano) }

type pageJSON struct {
	Data     any `json:"data"`
	Total    int `json:"total"`
	Page     int `json:"page"`
	PageSize int `json:"pageSize"`
}

type dataJSON struct {
	Data any `json:"data"`
}

type provenanceJSON struct {
	RunID               string    `json:"runId"`
	Latest              bool      `json:"latest"`
	Trigger             string    `json:"trigger"`
	ModelVersion        *string   `json:"modelVersion"`
	MetricEngineVersion int       `json:"metricEngineVersion"`
	ResultSchemaVersion int       `json:"resultSchemaVersion"`
	CalibrationID       *string   `json:"calibrationId"`
	RecordingID         string    `json:"recordingId"`
	SourceVideoID       string    `json:"sourceVideoId"`
	RunFinishedAt       time.Time `json:"runFinishedAt"`
	TestID              string    `json:"testId"`
	TestStatus          string    `json:"testStatus"`
	ScheduledAt         time.Time `json:"scheduledAt"`
	TrialCount          int       `json:"trialCount"`
	ExperimentID        string    `json:"experimentId"`
	ExperimentCode      string    `json:"experimentCode"`
	SubjectID           string    `json:"subjectId"`
	SubjectCode         string    `json:"subjectCode"`
	Species             string    `json:"species"`
	Sex                 string    `json:"sex"`
	PhaseID             *string   `json:"phaseId"`
	PhaseName           *string   `json:"phaseName"`
	GroupID             *string   `json:"groupId"`
	GroupName           *string   `json:"groupName"`
	GroupRole           *string   `json:"groupRole"`
	ParadigmKey         string    `json:"paradigmKey"`
	ParadigmVersion     int       `json:"paradigmVersion"`
	EnvironmentID       string    `json:"environmentId"`
	EnvironmentName     string    `json:"environmentName"`
	EnvironmentRevision int       `json:"environmentRevision"`
}

func toProvenance(p domain.Provenance) provenanceJSON {
	return provenanceJSON{
		RunID: p.RunID, Latest: p.Latest, Trigger: p.Trigger, ModelVersion: nullable(p.ModelVersion), MetricEngineVersion: p.MetricEngineVersion,
		ResultSchemaVersion: p.ResultSchemaVersion, CalibrationID: nullable(p.CalibrationID), RecordingID: p.RecordingID, SourceVideoID: p.SourceVideoID,
		RunFinishedAt: p.RunFinishedAt, TestID: p.TestID, TestStatus: p.TestStatus, ScheduledAt: p.ScheduledAt, TrialCount: p.TrialCount,
		ExperimentID: p.ExperimentID, ExperimentCode: p.ExperimentCode, SubjectID: p.SubjectID, SubjectCode: p.SubjectCode, Species: p.Species, Sex: p.Sex,
		PhaseID: nullable(p.PhaseID), PhaseName: nullable(p.PhaseName), GroupID: nullable(p.GroupID), GroupName: nullable(p.GroupName),
		GroupRole: nullable(p.GroupRole), ParadigmKey: p.ParadigmKey, ParadigmVersion: p.ParadigmVersion, EnvironmentID: p.EnvironmentID,
		EnvironmentName: p.EnvironmentName, EnvironmentRevision: p.EnvironmentRevision,
	}
}

type metricJSON struct {
	provenanceJSON
	MetricKey     string   `json:"metricKey"`
	Unit          string   `json:"unit"`
	Value         *float64 `json:"value"`
	MissingReason *string  `json:"missingReason"`
}

func toMetric(m domain.MetricRow) metricJSON {
	return metricJSON{toProvenance(m.Provenance), m.MetricKey, m.Unit, m.Value, nullable(m.MissingReason)}
}

type eventJSON struct {
	provenanceJSON
	EventID         string  `json:"eventId"`
	EventType       string  `json:"eventType"`
	Kind            string  `json:"kind"`
	StartUs         int64   `json:"startUs"`
	EndUs           int64   `json:"endUs"`
	Confidence      float64 `json:"confidence"`
	TrialID         *string `json:"trialId"`
	TrialNumber     *int    `json:"trialNumber"`
	TrialRepetition *int    `json:"trialRepetition"`
	TrialAttempt    *int    `json:"trialAttempt"`
}

func toEvent(e domain.EventRow) eventJSON {
	out := eventJSON{provenanceJSON: toProvenance(e.Provenance), EventID: e.EventID, EventType: e.EventType, Kind: e.Kind, StartUs: e.StartUs, EndUs: e.EndUs, Confidence: e.Confidence}
	if e.TrialID != "" {
		out.TrialID, out.TrialNumber, out.TrialRepetition, out.TrialAttempt = &e.TrialID, &e.TrialNumber, &e.TrialRepetition, &e.TrialAttempt
	}
	return out
}

type versionJSON struct {
	ParadigmKey         string `json:"paradigmKey"`
	ParadigmVersion     int    `json:"paradigmVersion"`
	MetricEngineVersion int    `json:"metricEngineVersion"`
	Unit                string `json:"unit"`
}

type groupJSON struct {
	GroupID         *string        `json:"groupId"`
	GroupName       *string        `json:"groupName"`
	GroupRole       *string        `json:"groupRole"`
	Tests           int            `json:"tests"`
	Subjects        int            `json:"subjects"`
	MissingSubjects int            `json:"missingSubjects"`
	MissingReasons  map[string]int `json:"missingReasons"`
	Mean            *float64       `json:"mean"`
	SD              *float64       `json:"sd"`
	SEM             *float64       `json:"sem"`
	Min             *float64       `json:"min"`
	Max             *float64       `json:"max"`
}

type summaryJSON struct {
	MetricKey string       `json:"metricKey"`
	Version   *versionJSON `json:"version"`
	Groups    []groupJSON  `json:"groups"`
}

func mapSlice[T, U any](items []T, convert func(T) U) []U {
	out := make([]U, 0, len(items))
	for _, item := range items {
		out = append(out, convert(item))
	}
	return out
}

func nullable(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
