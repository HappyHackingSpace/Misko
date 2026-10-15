// Package reportshttp exposes the study report of an experiment and its CSV
// export.
package reportshttp

import (
	"bytes"
	"encoding/csv"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf16"

	access "github.com/HappyHackingSpace/Misko/backend/internal/access/domain"
	"github.com/HappyHackingSpace/Misko/backend/internal/platform/httpjson"
	"github.com/HappyHackingSpace/Misko/backend/internal/reports/application"
	"github.com/HappyHackingSpace/Misko/backend/internal/reports/domain"
)

var failures = []httpjson.Failure{
	{Err: access.ErrUnauthenticated, Status: http.StatusUnauthorized, Code: "auth.unauthenticated"},
	{Err: access.ErrForbidden, Status: http.StatusForbidden, Code: "auth.forbidden"},
	{Err: application.ErrInvalidQuery, Status: http.StatusBadRequest, Code: "report.invalidQuery"},
	{Err: application.ErrExperimentNotFound, Status: http.StatusNotFound, Code: "report.experimentNotFound"},
	{Err: application.ErrEndpointNotFound, Status: http.StatusNotFound, Code: "report.endpointNotFound"},
	{Err: application.ErrExportTooLarge, Status: http.StatusBadRequest, Code: "report.exportTooLarge"},
}

type handler struct {
	service      *application.Service
	authenticate func(*http.Request) (access.Actor, error)
	logger       *slog.Logger
}

func Register(mux *http.ServeMux, service *application.Service, authenticate func(*http.Request) (access.Actor, error), logger *slog.Logger) {
	h := handler{service: service, authenticate: authenticate, logger: logger}
	mux.HandleFunc("GET /api/experiments/{id}/report", h.with(h.report))
	mux.HandleFunc("GET /api/experiments/{id}/report/export", h.with(h.export))
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

func (h handler) report(w http.ResponseWriter, r *http.Request, actor access.Actor) {
	data, err := h.service.Report(r.Context(), actor, r.PathValue("id"), r.URL.Query().Get("endpointId"))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpjson.Write(w, http.StatusOK, toReport(data.Report))
}

// export writes one row per animal and test at the endpoint's timepoint and
// baseline (level=animals, the default) or one row per group and timepoint
// (level=groups).
func (h handler) export(w http.ResponseWriter, r *http.Request, actor access.Actor) {
	level := r.URL.Query().Get("level")
	if level != "" && level != "animals" && level != "groups" {
		h.fail(w, r, application.ErrInvalidQuery)
		return
	}
	data, err := h.service.Report(r.Context(), actor, r.PathValue("id"), r.URL.Query().Get("endpointId"))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	if level == "groups" {
		header, records := groupRecords(data.Report)
		h.csv(w, r, "misko-report-groups.csv", header, records)
		return
	}
	header, records := animalRecords(data)
	h.csv(w, r, "misko-report-animals.csv", header, records)
}

func (h handler) csv(w http.ResponseWriter, r *http.Request, name string, header []string, records [][]string) {
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	var sink io.Writer = w
	var buf bytes.Buffer
	excel := r.URL.Query().Get("excel") == "1"
	if excel {
		// Excel takes the separator from the OS regional settings and ignores a UTF-8 BOM next
		// to a sep= line, so the file is UTF-16 LE with a BOM and tab separators: it opens the
		// same way in every locale with Turkish characters intact.
		w.Header().Set("Content-Type", "text/csv; charset=utf-16le")
		sink = &buf
	}
	out := csv.NewWriter(sink)
	if excel {
		out.Comma = '\t'
	}
	if err := out.Write(header); err == nil {
		err = out.WriteAll(records)
	}
	if excel && out.Error() == nil {
		units := utf16.Encode([]rune(buf.String()))
		raw := make([]byte, 2, 2+2*len(units))
		raw[0], raw[1] = 0xFF, 0xFE
		for _, u := range units {
			raw = append(raw, byte(u), byte(u>>8))
		}
		_, _ = w.Write(raw)
	}
	if err := out.Error(); err != nil {
		h.logger.ErrorContext(r.Context(), "csv export interrupted", "route", r.Pattern, "error", err)
	}
}

func (h handler) fail(w http.ResponseWriter, r *http.Request, err error) {
	httpjson.Fail(w, r, h.logger, err, failures)
}

var animalHeader = []string{
	"experimentCode", "endpointId", "endpointKind", "paradigmKey", "metricKey", "timepoint", "studyDay", "baseline",
	"groupCode", "groupName", "groupRole", "diseaseModel", "substance", "dose", "doseUnit",
	"subjectCode", "allocationId", "testSubjectId", "testId", "arena", "scheduledAt", "excluded", "resultStatus", "resultRunId",
	"value", "unit", "missingReason", "paradigmVersion", "metricEngineVersion",
}

func animalRecords(d application.Data) ([]string, [][]string) {
	groups, timepoints := groupIndex(d.Report), timepointIndex(d.Report)
	var records [][]string
	for i, s := range d.Report.Sections {
		e := s.Endpoint
		for _, m := range d.Measurements[i] {
			if m.TimepointID != e.TimepointID && m.TimepointID != e.BaselineTimepointID {
				continue
			}
			g, tp := groups[m.GroupID], timepoints[m.TimepointID]
			value, engine := "", ""
			if m.Value != nil {
				value = number(*m.Value)
			}
			if m.HasMetric {
				engine = strconv.Itoa(m.MetricEngineVersion)
			}
			records = append(records, []string{
				cell(d.Report.Experiment.Code), e.ID, e.Kind, e.ParadigmKey, e.MetricKey, cell(tp.Name), strconv.Itoa(tp.StudyDay),
				strconv.FormatBool(m.TimepointID == e.BaselineTimepointID), cell(g.Code), cell(g.Name), g.Role, cell(g.DiseaseModelName),
				cell(g.SubstanceName), dose(g.DoseMicro), g.Unit, cell(m.SubjectCode), m.AllocationID, m.TestSubjectID, m.TestID, cell(m.Arena),
				m.ScheduledAt.UTC().Format(time.RFC3339Nano), strconv.FormatBool(m.Excluded), m.ResultStatus, m.ResultRunID,
				value, cell(m.Unit), cell(m.MissingReason), strconv.Itoa(m.ParadigmVersion), engine,
			})
		}
	}
	return animalHeader, records
}

var groupHeader = []string{
	"experimentCode", "endpointId", "endpointKind", "paradigmKey", "metricKey", "direction", "timepoint", "studyDay", "endpointTimepoint",
	"groupCode", "groupName", "groupRole", "diseaseModel", "substance", "dose", "doseUnit",
	"n", "mean", "sd", "sem", "min", "max", "tested", "pending", "awaitingAnalysis", "rejected", "missing", "excluded",
	"changeN", "changeMean", "changeSd", "changeSem", "comparable",
}

func groupRecords(r domain.Report) ([]string, [][]string) {
	groups, timepoints := groupIndex(r), timepointIndex(r)
	var records [][]string
	for _, s := range r.Sections {
		e := s.Endpoint
		for _, tc := range s.TimeCourse {
			stats := tc.Groups
			// The endpoint's own timepoint carries the change from baseline.
			if tc.TimepointID == e.TimepointID {
				stats = s.Groups
			}
			tp := timepoints[tc.TimepointID]
			for _, st := range stats {
				g := groups[st.GroupID]
				row := []string{
					cell(r.Experiment.Code), e.ID, e.Kind, e.ParadigmKey, e.MetricKey, e.Direction, cell(tp.Name), strconv.Itoa(tp.StudyDay),
					strconv.FormatBool(tc.TimepointID == e.TimepointID), cell(g.Code), cell(g.Name), g.Role, cell(g.DiseaseModelName),
					cell(g.SubstanceName), dose(g.DoseMicro), g.Unit,
					strconv.Itoa(st.N), optional(st.Mean), optional(st.SD), optional(st.SEM), optional(st.Min), optional(st.Max),
					strconv.Itoa(st.Tested), strconv.Itoa(st.Pending), strconv.Itoa(st.AwaitingAnalysis), strconv.Itoa(st.Rejected),
					strconv.Itoa(st.Missing), strconv.Itoa(st.Excluded),
				}
				if c := st.Change; c != nil {
					row = append(row, strconv.Itoa(c.N), optional(c.Mean), optional(c.SD), optional(c.SEM))
				} else {
					row = append(row, "", "", "", "")
				}
				records = append(records, append(row, strconv.FormatBool(s.Comparable)))
			}
		}
	}
	return groupHeader, records
}

func groupIndex(r domain.Report) map[string]domain.Group {
	out := make(map[string]domain.Group, len(r.Groups))
	for _, g := range r.Groups {
		out[g.ID] = g
	}
	return out
}

func timepointIndex(r domain.Report) map[string]domain.Timepoint {
	out := make(map[string]domain.Timepoint, len(r.Timepoints))
	for _, t := range r.Timepoints {
		out[t.ID] = t
	}
	return out
}

// cell keeps spreadsheet applications from evaluating stored text as a formula.
func cell(s string) string {
	if s != "" && strings.ContainsRune("=+-@\t\r", rune(s[0])) && !isNumber(s) {
		return "'" + s
	}
	return s
}

func isNumber(s string) bool {
	_, err := strconv.ParseFloat(s, 64)
	return err == nil
}

func number(v float64) string { return strconv.FormatFloat(v, 'g', -1, 64) }

func optional(v *float64) string {
	if v == nil {
		return ""
	}
	return number(*v)
}

// dose formats a dose stored in millionths of its unit.
func dose(micro *int64) string {
	if micro == nil {
		return ""
	}
	return strconv.FormatFloat(float64(*micro)/1e6, 'f', -1, 64)
}

type experimentJSON struct {
	ID      string `json:"id"`
	Code    string `json:"code"`
	Title   string `json:"title"`
	Status  string `json:"status"`
	Blinded bool   `json:"blinded"`
}

type treatmentJSON struct {
	SubstanceID   string `json:"substanceId"`
	SubstanceName string `json:"substanceName"`
	DoseMicro     int64  `json:"doseMicro"`
	Unit          string `json:"unit"`
}

type groupJSON struct {
	ID               string         `json:"id"`
	Code             string         `json:"code"`
	Name             string         `json:"name"`
	Role             string         `json:"role"`
	DiseaseModelID   *string        `json:"diseaseModelId"`
	DiseaseModelName *string        `json:"diseaseModelName"`
	Treatment        *treatmentJSON `json:"treatment"`
	Allocated        int            `json:"allocated"`
	Excluded         int            `json:"excluded"`
}

type timepointJSON struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	StudyDay int    `json:"studyDay"`
	Position int    `json:"position"`
}

type endpointJSON struct {
	ID                  string  `json:"id"`
	Kind                string  `json:"kind"`
	ParadigmKey         string  `json:"paradigmKey"`
	MetricKey           string  `json:"metricKey"`
	Direction           string  `json:"direction"`
	TimepointID         string  `json:"timepointId"`
	BaselineTimepointID *string `json:"baselineTimepointId"`
}

type versionJSON struct {
	ParadigmVersion     int    `json:"paradigmVersion"`
	MetricEngineVersion int    `json:"metricEngineVersion"`
	Unit                string `json:"unit"`
}

type descriptivesJSON struct {
	N    int      `json:"n"`
	Mean *float64 `json:"mean"`
	SD   *float64 `json:"sd"`
	SEM  *float64 `json:"sem"`
	Min  *float64 `json:"min"`
	Max  *float64 `json:"max"`
}

type groupStatsJSON struct {
	GroupID string `json:"groupId"`
	descriptivesJSON
	Tested           int               `json:"tested"`
	Pending          int               `json:"pending"`
	AwaitingAnalysis int               `json:"awaitingAnalysis"`
	Rejected         int               `json:"rejected"`
	Missing          int               `json:"missing"`
	Excluded         int               `json:"excluded"`
	Change           *descriptivesJSON `json:"change"`
}

type comparisonJSON struct {
	Kind                    string   `json:"kind"`
	GroupID                 string   `json:"groupId"`
	ReferenceGroupID        string   `json:"referenceGroupId"`
	HealthyReferenceGroupID *string  `json:"healthyReferenceGroupId"`
	Difference              *float64 `json:"difference"`
	PercentDifference       *float64 `json:"percentDifference"`
	Effect                  *float64 `json:"effect"`
	RescuePercent           *float64 `json:"rescuePercent"`
}

type dosePointJSON struct {
	GroupID       string   `json:"groupId"`
	DoseMicro     int64    `json:"doseMicro"`
	N             int      `json:"n"`
	Mean          *float64 `json:"mean"`
	RescuePercent *float64 `json:"rescuePercent"`
}

type doseResponseJSON struct {
	SubstanceID   string          `json:"substanceId"`
	SubstanceName string          `json:"substanceName"`
	Unit          string          `json:"unit"`
	Points        []dosePointJSON `json:"points"`
}

type timepointStatsJSON struct {
	TimepointID string           `json:"timepointId"`
	Groups      []groupStatsJSON `json:"groups"`
}

type sectionJSON struct {
	Endpoint     endpointJSON         `json:"endpoint"`
	Versions     []versionJSON        `json:"versions"`
	Comparable   bool                 `json:"comparable"`
	Groups       []groupStatsJSON     `json:"groups"`
	Comparisons  []comparisonJSON     `json:"comparisons"`
	DoseResponse []doseResponseJSON   `json:"doseResponse"`
	TimeCourse   []timepointStatsJSON `json:"timeCourse"`
}

type reportJSON struct {
	Experiment experimentJSON  `json:"experiment"`
	Groups     []groupJSON     `json:"groups"`
	Timepoints []timepointJSON `json:"timepoints"`
	Sections   []sectionJSON   `json:"sections"`
}

func toReport(r domain.Report) reportJSON {
	e := r.Experiment
	out := reportJSON{Experiment: experimentJSON{e.ID, e.Code, e.Title, e.Status, e.Blinded}}
	out.Groups = mapSlice(r.Groups, func(g domain.Group) groupJSON {
		j := groupJSON{ID: g.ID, Code: g.Code, Name: g.Name, Role: g.Role, DiseaseModelID: nullable(g.DiseaseModelID),
			DiseaseModelName: nullable(g.DiseaseModelName), Allocated: g.Allocated, Excluded: g.Excluded}
		if g.DoseMicro != nil {
			j.Treatment = &treatmentJSON{g.SubstanceID, g.SubstanceName, *g.DoseMicro, g.Unit}
		}
		return j
	})
	out.Timepoints = mapSlice(r.Timepoints, func(t domain.Timepoint) timepointJSON { return timepointJSON(t) })
	out.Sections = mapSlice(r.Sections, toSection)
	return out
}

func toSection(s domain.Section) sectionJSON {
	e := s.Endpoint
	return sectionJSON{
		Endpoint:   endpointJSON{e.ID, e.Kind, e.ParadigmKey, e.MetricKey, e.Direction, e.TimepointID, nullable(e.BaselineTimepointID)},
		Versions:   mapSlice(s.Versions, func(v domain.Version) versionJSON { return versionJSON(v) }),
		Comparable: s.Comparable,
		Groups:     mapSlice(s.Groups, toStats),
		Comparisons: mapSlice(s.Comparisons, func(c domain.Comparison) comparisonJSON {
			return comparisonJSON{c.Kind, c.GroupID, c.ReferenceGroupID, nullable(c.HealthyReferenceGroupID), c.Difference, c.PercentDifference, c.Effect, c.RescuePercent}
		}),
		DoseResponse: mapSlice(s.DoseResponse, func(d domain.DoseResponse) doseResponseJSON {
			return doseResponseJSON{d.SubstanceID, d.SubstanceName, d.Unit, mapSlice(d.Points, func(p domain.DosePoint) dosePointJSON { return dosePointJSON(p) })}
		}),
		TimeCourse: mapSlice(s.TimeCourse, func(t domain.TimepointStats) timepointStatsJSON {
			return timepointStatsJSON{t.TimepointID, mapSlice(t.Groups, toStats)}
		}),
	}
}

func toStats(s domain.GroupStats) groupStatsJSON {
	out := groupStatsJSON{GroupID: s.GroupID, descriptivesJSON: descriptivesJSON(s.Descriptives), Tested: s.Tested, Pending: s.Pending,
		AwaitingAnalysis: s.AwaitingAnalysis, Rejected: s.Rejected, Missing: s.Missing, Excluded: s.Excluded}
	if s.Change != nil {
		c := descriptivesJSON(*s.Change)
		out.Change = &c
	}
	return out
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
