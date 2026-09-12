// Package paradigmshttp exposes the read-only, versioned paradigm catalog.
// Only GET routes exist; definitions change through code, not the API.
package paradigmshttp

import (
	access "github.com/HappyHackingSpace/Misko/backend/internal/access/domain"
	"github.com/HappyHackingSpace/Misko/backend/internal/paradigms/application"
	"github.com/HappyHackingSpace/Misko/backend/internal/paradigms/domain"
	"github.com/HappyHackingSpace/Misko/backend/internal/platform/httpjson"
	"log/slog"
	"net/http"
	"strconv"
)

var failures = []httpjson.Failure{
	{Err: access.ErrUnauthenticated, Status: http.StatusUnauthorized, Code: "auth.unauthenticated"},
	{Err: access.ErrForbidden, Status: http.StatusForbidden, Code: "auth.forbidden"},
	{Err: domain.ErrUnknownParadigm, Status: http.StatusNotFound, Code: "paradigm.notFound"},
	{Err: domain.ErrUnknownVersion, Status: http.StatusNotFound, Code: "paradigm.versionNotFound"},
}

type handler struct {
	service      *application.Service
	authenticate func(*http.Request) (access.Actor, error)
	logger       *slog.Logger
}

func Register(mux *http.ServeMux, service *application.Service, authenticate func(*http.Request) (access.Actor, error), logger *slog.Logger) {
	h := handler{service: service, authenticate: authenticate, logger: logger}
	mux.HandleFunc("GET /api/paradigms", h.list)
	mux.HandleFunc("GET /api/paradigms/{key}", h.latest)
	mux.HandleFunc("GET /api/paradigms/{key}/versions/{version}", h.version)
}

func (h handler) list(w http.ResponseWriter, r *http.Request) {
	actor, err := h.authenticate(r)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	summaries, err := h.service.List(r.Context(), actor)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	data := make([]summaryJSON, 0, len(summaries))
	for _, s := range summaries {
		data = append(data, summaryJSON{s.Key, s.Name, string(s.Category), s.Species, s.Versions, s.LatestVersion, s.AutomatedAnalysis})
	}
	httpjson.Write(w, http.StatusOK, struct {
		Data []summaryJSON `json:"data"`
	}{data})
}

func (h handler) latest(w http.ResponseWriter, r *http.Request) {
	actor, err := h.authenticate(r)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	m, err := h.service.Latest(r.Context(), actor, r.PathValue("key"))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpjson.Write(w, http.StatusOK, toManifest(m))
}

func (h handler) version(w http.ResponseWriter, r *http.Request) {
	actor, err := h.authenticate(r)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	version, err := strconv.Atoi(r.PathValue("version"))
	if err != nil {
		version = -1
	}
	m, err := h.service.Version(r.Context(), actor, r.PathValue("key"), version)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpjson.Write(w, http.StatusOK, toManifest(m))
}

func (h handler) fail(w http.ResponseWriter, r *http.Request, err error) {
	httpjson.Fail(w, r, h.logger, err, failures)
}

type summaryJSON struct {
	Key               string   `json:"key"`
	Name              string   `json:"name"`
	Category          string   `json:"category"`
	Species           []string `json:"species"`
	Versions          []int    `json:"versions"`
	LatestVersion     int      `json:"latestVersion"`
	AutomatedAnalysis bool     `json:"automatedAnalysis"`
}

type parameterJSON struct {
	Key       string  `json:"key"`
	Label     string  `json:"label"`
	Unit      string  `json:"unit"`
	ValueType string  `json:"valueType"`
	Min       float64 `json:"min"`
	Max       float64 `json:"max"`
	Default   float64 `json:"default"`
}

type zoneJSON struct {
	Key        string `json:"key"`
	Role       string `json:"role"`
	Geometry   string `json:"geometry"`
	Calibrated bool   `json:"calibrated"`
	Required   bool   `json:"required"`
}

type eventJSON struct {
	Type       string   `json:"type"`
	Kind       string   `json:"kind"`
	Labels     []string `json:"labels"`
	Single     bool     `json:"single"`
	Definition string   `json:"definition"`
}

type metricJSON struct {
	Key         string   `json:"key"`
	Label       string   `json:"label"`
	Unit        string   `json:"unit"`
	ValueType   string   `json:"valueType"`
	Definition  string   `json:"definition"`
	Formula     string   `json:"formula"`
	MissingWhen string   `json:"missingWhen"`
	Inputs      []string `json:"inputs"`
	Min         float64  `json:"min"`
	Max         float64  `json:"max"`
	Tolerance   float64  `json:"tolerance"`
}

type qcJSON struct {
	Key      string  `json:"key"`
	Operator string  `json:"operator"`
	Value    float64 `json:"value"`
	Unit     string  `json:"unit"`
}

type manifestJSON struct {
	Key                 string          `json:"key"`
	Name                string          `json:"name"`
	Category            string          `json:"category"`
	Species             []string        `json:"species"`
	TrialTypes          []string        `json:"trialTypes"`
	Version             int             `json:"version"`
	MetricEngineVersion int             `json:"metricEngineVersion"`
	ResultSchemaVersion int             `json:"resultSchemaVersion"`
	AutomatedAnalysis   bool            `json:"automatedAnalysis"`
	ApparatusParameters []parameterJSON `json:"apparatusParameters"`
	SessionParameters   []parameterJSON `json:"sessionParameters"`
	Zones               []zoneJSON      `json:"zones"`
	InputEvents         []eventJSON     `json:"inputEvents"`
	Events              []eventJSON     `json:"events"`
	Metrics             []metricJSON    `json:"metrics"`
	QC                  []qcJSON        `json:"qc"`
}

func toManifest(m application.Manifest) manifestJSON {
	p := m.Paradigm
	out := manifestJSON{
		Key: p.Key, Name: p.Name, Category: string(p.Category), Species: p.Species, TrialTypes: p.TrialTypes, Version: p.Version,
		MetricEngineVersion: m.MetricEngineVersion, ResultSchemaVersion: m.ResultSchemaVersion, AutomatedAnalysis: m.AutomatedAnalysis,
		ApparatusParameters: parameters(p.ApparatusParameters), SessionParameters: parameters(p.SessionParameters),
		Zones: []zoneJSON{}, InputEvents: events(p.InputEvents), Events: events(p.Events), Metrics: []metricJSON{}, QC: []qcJSON{},
	}
	for _, z := range p.Zones {
		out.Zones = append(out.Zones, zoneJSON{z.Key, z.Role, z.Geometry, z.Calibrated, z.Required})
	}
	for _, mt := range p.Metrics {
		out.Metrics = append(out.Metrics, metricJSON{mt.Key, mt.Label, string(mt.Unit), valueType(mt.Integer), mt.Definition, mt.Formula, mt.MissingWhen, mt.Inputs, mt.Min, mt.Max, mt.Tolerance})
	}
	for _, q := range p.QC {
		out.QC = append(out.QC, qcJSON{q.Key, q.Operator, q.Value, string(q.Unit)})
	}
	return out
}

func parameters(params []domain.Parameter) []parameterJSON {
	out := make([]parameterJSON, 0, len(params))
	for _, p := range params {
		out = append(out, parameterJSON{p.Key, p.Label, string(p.Unit), valueType(p.Integer), p.Min, p.Max, p.Default})
	}
	return out
}

func events(defs []domain.EventDefinition) []eventJSON {
	out := make([]eventJSON, 0, len(defs))
	for _, e := range defs {
		labels := e.Labels
		if labels == nil {
			labels = []string{}
		}
		out = append(out, eventJSON{e.Type, string(e.Kind), labels, e.Single, e.Definition})
	}
	return out
}

func valueType(integer bool) string {
	if integer {
		return "integer"
	}
	return "number"
}
