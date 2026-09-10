// Package protocolshttp exposes the test protocols of an experiment and their
// versions over HTTP.
package protocolshttp

import (
	access "github.com/HappyHackingSpace/Misko/backend/internal/access/domain"
	"github.com/HappyHackingSpace/Misko/backend/internal/platform/httpjson"
	"github.com/HappyHackingSpace/Misko/backend/internal/protocols/application"
	"github.com/HappyHackingSpace/Misko/backend/internal/protocols/domain"
	"log/slog"
	"net/http"
	"strconv"
	"time"
)

var failures = []httpjson.Failure{
	{Err: access.ErrUnauthenticated, Status: http.StatusUnauthorized, Code: "auth.unauthenticated"},
	{Err: access.ErrForbidden, Status: http.StatusForbidden, Code: "auth.forbidden"},
	{Err: application.ErrExperimentNotFound, Status: http.StatusNotFound, Code: "experiment.notFound"},
	{Err: application.ErrProtocolNotFound, Status: http.StatusNotFound, Code: "protocol.notFound"},
	{Err: application.ErrVersionNotFound, Status: http.StatusNotFound, Code: "protocol.versionNotFound"},
	{Err: application.ErrEnvironmentRevisionNotFound, Status: http.StatusNotFound, Code: "environment.revisionNotFound"},
	{Err: application.ErrIncompatibleEnvironment, Status: http.StatusBadRequest, Code: "protocol.incompatibleEnvironment"},
	{Err: application.ErrTrialTypeNotSupported, Status: http.StatusBadRequest, Code: "protocol.trialTypeNotSupported"},
	{Err: application.ErrUnknownParadigm, Status: http.StatusBadRequest, Code: "protocol.unknownParadigm"},
	{Err: application.ErrUnknownParadigmVersion, Status: http.StatusBadRequest, Code: "protocol.unknownParadigmVersion"},
	{Err: application.ErrInvalidSession, Status: http.StatusBadRequest, Code: "protocol.invalidSession"},
	{Err: application.ErrNameTaken, Status: http.StatusConflict, Code: "protocol.nameTaken"},
	{Err: application.ErrNoChanges, Status: http.StatusBadRequest, Code: "common.noChanges"},
	{Err: domain.ErrInvalidName, Status: http.StatusBadRequest, Code: "common.invalidName"},
	{Err: domain.ErrInvalidDescription, Status: http.StatusBadRequest, Code: "common.invalidDescription"},
	{Err: domain.ErrInvalidNotes, Status: http.StatusBadRequest, Code: "common.invalidNotes"},
	{Err: domain.ErrNoSteps, Status: http.StatusBadRequest, Code: "protocol.noSteps"},
	{Err: domain.ErrTooManySteps, Status: http.StatusBadRequest, Code: "protocol.tooManySteps"},
	{Err: domain.ErrInvalidStepOrder, Status: http.StatusBadRequest, Code: "protocol.invalidStepOrder"},
	{Err: domain.ErrInvalidParadigmKey, Status: http.StatusBadRequest, Code: "protocol.invalidParadigmKey"},
	{Err: domain.ErrInvalidParadigmVersion, Status: http.StatusBadRequest, Code: "protocol.invalidParadigmVersion"},
	{Err: domain.ErrMissingEnvironment, Status: http.StatusBadRequest, Code: "protocol.missingEnvironment"},
	{Err: domain.ErrInvalidTrialType, Status: http.StatusBadRequest, Code: "protocol.invalidTrialType"},
	{Err: domain.ErrInvalidTrials, Status: http.StatusBadRequest, Code: "protocol.invalidTrials"},
	{Err: domain.ErrInvalidInterval, Status: http.StatusBadRequest, Code: "protocol.invalidInterval"},
}

type handler struct {
	service      *application.Service
	authenticate func(*http.Request) (access.Actor, error)
	logger       *slog.Logger
}

func Register(mux *http.ServeMux, service *application.Service, authenticate func(*http.Request) (access.Actor, error), logger *slog.Logger) {
	h := handler{service: service, authenticate: authenticate, logger: logger}
	mux.HandleFunc("GET /api/experiments/{id}/protocols", h.with(h.list))
	mux.HandleFunc("POST /api/experiments/{id}/protocols", h.with(h.create))
	mux.HandleFunc("GET /api/experiments/{id}/protocols/{protocolId}", h.with(h.get))
	mux.HandleFunc("PATCH /api/experiments/{id}/protocols/{protocolId}", h.with(h.update))
	mux.HandleFunc("GET /api/experiments/{id}/protocols/{protocolId}/versions", h.with(h.versions))
	mux.HandleFunc("POST /api/experiments/{id}/protocols/{protocolId}/versions", h.with(h.addVersion))
	mux.HandleFunc("GET /api/experiments/{id}/protocols/{protocolId}/versions/{number}", h.with(h.version))
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

type stepBody struct {
	Position              int                `json:"position"`
	ParadigmKey           string             `json:"paradigmKey"`
	ParadigmVersion       int                `json:"paradigmVersion"`
	EnvironmentRevisionID string             `json:"environmentRevisionId"`
	TrialType             string             `json:"trialType"`
	Trials                int                `json:"trials"`
	InterTrialIntervalS   int                `json:"interTrialIntervalS"`
	Session               map[string]float64 `json:"session"`
	Notes                 string             `json:"notes"`
}

type versionBody struct {
	Notes string     `json:"notes"`
	Steps []stepBody `json:"steps"`
}

func (b versionBody) input() application.VersionInput {
	in := application.VersionInput{Notes: b.Notes}
	for _, s := range b.Steps {
		in.Steps = append(in.Steps, application.StepInput(s))
	}
	return in
}

func (h handler) list(w http.ResponseWriter, r *http.Request, actor access.Actor) {
	protocols, err := h.service.List(r.Context(), actor, r.PathValue("id"))
	h.respond(w, r, http.StatusOK, dataJSON{mapSlice(protocols, protocolJSON)}, err)
}

func (h handler) create(w http.ResponseWriter, r *http.Request, actor access.Actor) {
	var body struct {
		Name        string      `json:"name"`
		Description string      `json:"description"`
		Version     versionBody `json:"version"`
	}
	if !h.decode(w, r, &body) {
		return
	}
	created, err := h.service.Create(r.Context(), actor, r.PathValue("id"), application.ProtocolInput{
		Name: body.Name, Description: body.Description, Version: body.Version.input(),
	})
	h.respond(w, r, http.StatusCreated, struct {
		Protocol protocolJSONBody `json:"protocol"`
		Version  versionJSONBody  `json:"version"`
	}{protocolJSON(created.Protocol), versionJSON(created.Version)}, err)
}

func (h handler) get(w http.ResponseWriter, r *http.Request, actor access.Actor) {
	p, err := h.service.Get(r.Context(), actor, r.PathValue("id"), r.PathValue("protocolId"))
	h.respond(w, r, http.StatusOK, protocolJSON(p), err)
}

func (h handler) update(w http.ResponseWriter, r *http.Request, actor access.Actor) {
	var body struct {
		Name        *string `json:"name"`
		Description *string `json:"description"`
	}
	if !h.decode(w, r, &body) {
		return
	}
	p, err := h.service.Update(r.Context(), actor, r.PathValue("id"), r.PathValue("protocolId"), application.Patch(body))
	h.respond(w, r, http.StatusOK, protocolJSON(p), err)
}

func (h handler) versions(w http.ResponseWriter, r *http.Request, actor access.Actor) {
	versions, err := h.service.Versions(r.Context(), actor, r.PathValue("id"), r.PathValue("protocolId"))
	h.respond(w, r, http.StatusOK, dataJSON{mapSlice(versions, versionJSON)}, err)
}

func (h handler) addVersion(w http.ResponseWriter, r *http.Request, actor access.Actor) {
	var body versionBody
	if !h.decode(w, r, &body) {
		return
	}
	v, err := h.service.AddVersion(r.Context(), actor, r.PathValue("id"), r.PathValue("protocolId"), body.input())
	h.respond(w, r, http.StatusCreated, versionJSON(v), err)
}

func (h handler) version(w http.ResponseWriter, r *http.Request, actor access.Actor) {
	number, err := strconv.Atoi(r.PathValue("number"))
	if err != nil {
		number = 0
	}
	v, err := h.service.Version(r.Context(), actor, r.PathValue("id"), r.PathValue("protocolId"), number)
	h.respond(w, r, http.StatusOK, versionJSON(v), err)
}

func (h handler) decode(w http.ResponseWriter, r *http.Request, dst any) bool {
	if err := httpjson.Decode(w, r, dst); err != nil {
		h.fail(w, r, err)
		return false
	}
	return true
}

func (h handler) respond(w http.ResponseWriter, r *http.Request, status int, body any, err error) {
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpjson.Write(w, status, body)
}

func (h handler) fail(w http.ResponseWriter, r *http.Request, err error) {
	httpjson.Fail(w, r, h.logger, err, failures)
}

type dataJSON struct {
	Data any `json:"data"`
}

type protocolJSONBody struct {
	ID            string    `json:"id"`
	ExperimentID  string    `json:"experimentId"`
	Name          string    `json:"name"`
	Description   *string   `json:"description"`
	LatestVersion int       `json:"latestVersion"`
	CreatedAt     time.Time `json:"createdAt"`
	UpdatedAt     time.Time `json:"updatedAt"`
}

func protocolJSON(p domain.Protocol) protocolJSONBody {
	return protocolJSONBody{p.ID, p.ExperimentID, p.Name, nullable(p.Description), p.LatestVersion, p.CreatedAt, p.UpdatedAt}
}

type stepJSONBody struct {
	Position              int                `json:"position"`
	ParadigmKey           string             `json:"paradigmKey"`
	ParadigmVersion       int                `json:"paradigmVersion"`
	EnvironmentID         string             `json:"environmentId"`
	EnvironmentRevision   int                `json:"environmentRevision"`
	EnvironmentRevisionID string             `json:"environmentRevisionId"`
	TrialType             string             `json:"trialType"`
	Trials                int                `json:"trials"`
	InterTrialIntervalS   int                `json:"interTrialIntervalS"`
	Session               map[string]float64 `json:"session"`
	Notes                 *string            `json:"notes"`
}

type versionJSONBody struct {
	ID           string         `json:"id"`
	ExperimentID string         `json:"experimentId"`
	ProtocolID   string         `json:"protocolId"`
	Number       int            `json:"number"`
	Notes        *string        `json:"notes"`
	CreatedBy    string         `json:"createdBy"`
	CreatedAt    time.Time      `json:"createdAt"`
	Steps        []stepJSONBody `json:"steps"`
}

func versionJSON(v domain.Version) versionJSONBody {
	out := versionJSONBody{v.ID, v.ExperimentID, v.ProtocolID, v.Number, nullable(v.Notes), v.CreatedBy, v.CreatedAt, make([]stepJSONBody, 0, len(v.Steps))}
	for _, s := range v.Steps {
		session := s.Session
		if session == nil {
			session = map[string]float64{}
		}
		out.Steps = append(out.Steps, stepJSONBody{
			s.Position, s.ParadigmKey, s.ParadigmVersion, s.EnvironmentID, s.EnvironmentRevision, s.EnvironmentRevisionID,
			s.TrialType, s.Trials, s.InterTrialIntervalS, session, nullable(s.Notes),
		})
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
