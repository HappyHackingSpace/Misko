// Package environmentshttp exposes environments and their revisions over HTTP.
package environmentshttp

import (
	access "github.com/HappyHackingSpace/Misko/backend/internal/access/domain"
	"github.com/HappyHackingSpace/Misko/backend/internal/environments/application"
	"github.com/HappyHackingSpace/Misko/backend/internal/environments/domain"
	"github.com/HappyHackingSpace/Misko/backend/internal/platform/httpjson"
	"log/slog"
	"net/http"
	"strconv"
	"time"
)

var failures = []httpjson.Failure{
	{Err: access.ErrUnauthenticated, Status: http.StatusUnauthorized, Code: "auth.unauthenticated"},
	{Err: access.ErrForbidden, Status: http.StatusForbidden, Code: "auth.forbidden"},
	{Err: application.ErrEnvironmentNotFound, Status: http.StatusNotFound, Code: "environment.notFound"},
	{Err: application.ErrRevisionNotFound, Status: http.StatusNotFound, Code: "environment.revisionNotFound"},
	{Err: application.ErrNameTaken, Status: http.StatusConflict, Code: "environment.nameTaken"},
	{Err: application.ErrNoChanges, Status: http.StatusBadRequest, Code: "common.noChanges"},
	{Err: application.ErrInvalidQuery, Status: http.StatusBadRequest, Code: "common.invalidQuery"},
	{Err: application.ErrUnknownParadigm, Status: http.StatusBadRequest, Code: "environment.unknownParadigm"},
	{Err: application.ErrUnknownParadigmVersion, Status: http.StatusBadRequest, Code: "environment.unknownParadigmVersion"},
	{Err: application.ErrInvalidApparatus, Status: http.StatusBadRequest, Code: "environment.invalidApparatus"},
	{Err: domain.ErrInvalidName, Status: http.StatusBadRequest, Code: "common.invalidName"},
	{Err: domain.ErrInvalidNotes, Status: http.StatusBadRequest, Code: "common.invalidNotes"},
	{Err: domain.ErrInvalidParadigmKey, Status: http.StatusBadRequest, Code: "environment.invalidParadigmKey"},
	{Err: domain.ErrInvalidParadigmVersion, Status: http.StatusBadRequest, Code: "environment.invalidParadigmVersion"},
}

type handler struct {
	service      *application.Service
	authenticate func(*http.Request) (access.Actor, error)
	logger       *slog.Logger
}

func Register(mux *http.ServeMux, service *application.Service, authenticate func(*http.Request) (access.Actor, error), logger *slog.Logger) {
	h := handler{service: service, authenticate: authenticate, logger: logger}
	mux.HandleFunc("GET /api/environments", h.with(h.list))
	mux.HandleFunc("POST /api/environments", h.with(h.create))
	mux.HandleFunc("GET /api/environments/{id}", h.with(h.get))
	mux.HandleFunc("PATCH /api/environments/{id}", h.with(h.update))
	mux.HandleFunc("GET /api/environments/{id}/revisions", h.with(h.revisions))
	mux.HandleFunc("POST /api/environments/{id}/revisions", h.with(h.addRevision))
	mux.HandleFunc("GET /api/environments/{id}/revisions/{number}", h.with(h.revision))
	mux.HandleFunc("GET /api/environment-revisions", h.with(h.allRevisions))
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

type revisionBody struct {
	ParadigmVersion int                `json:"paradigmVersion"`
	Apparatus       map[string]float64 `json:"apparatus"`
	Notes           string             `json:"notes"`
}

func (h handler) list(w http.ResponseWriter, r *http.Request, actor access.Actor) {
	environments, err := h.service.List(r.Context(), actor, r.URL.Query().Get("paradigmKey"))
	h.respond(w, r, http.StatusOK, dataJSON{mapSlice(environments, environmentJSON)}, err)
}

func (h handler) create(w http.ResponseWriter, r *http.Request, actor access.Actor) {
	var body struct {
		Name        string       `json:"name"`
		ParadigmKey string       `json:"paradigmKey"`
		Notes       string       `json:"notes"`
		Revision    revisionBody `json:"revision"`
	}
	if !h.decode(w, r, &body) {
		return
	}
	created, err := h.service.Create(r.Context(), actor, application.EnvironmentInput{
		Name: body.Name, ParadigmKey: body.ParadigmKey, Notes: body.Notes, Revision: application.RevisionInput(body.Revision),
	})
	h.respond(w, r, http.StatusCreated, struct {
		Environment environmentJSONBody `json:"environment"`
		Revision    revisionJSONBody    `json:"revision"`
	}{environmentJSON(created.Environment), revisionJSON(created.Revision)}, err)
}

func (h handler) get(w http.ResponseWriter, r *http.Request, actor access.Actor) {
	e, err := h.service.Get(r.Context(), actor, r.PathValue("id"))
	h.respond(w, r, http.StatusOK, environmentJSON(e), err)
}

func (h handler) update(w http.ResponseWriter, r *http.Request, actor access.Actor) {
	var body struct {
		Name  *string `json:"name"`
		Notes *string `json:"notes"`
	}
	if !h.decode(w, r, &body) {
		return
	}
	e, err := h.service.Update(r.Context(), actor, r.PathValue("id"), application.Patch(body))
	h.respond(w, r, http.StatusOK, environmentJSON(e), err)
}

func (h handler) revisions(w http.ResponseWriter, r *http.Request, actor access.Actor) {
	revisions, err := h.service.Revisions(r.Context(), actor, r.PathValue("id"))
	h.respond(w, r, http.StatusOK, dataJSON{mapSlice(revisions, revisionJSON)}, err)
}

func (h handler) allRevisions(w http.ResponseWriter, r *http.Request, actor access.Actor) {
	revisions, err := h.service.AllRevisions(r.Context(), actor)
	h.respond(w, r, http.StatusOK, dataJSON{mapSlice(revisions, revisionSummaryJSON)}, err)
}

func (h handler) addRevision(w http.ResponseWriter, r *http.Request, actor access.Actor) {
	var body revisionBody
	if !h.decode(w, r, &body) {
		return
	}
	revision, err := h.service.AddRevision(r.Context(), actor, r.PathValue("id"), application.RevisionInput(body))
	h.respond(w, r, http.StatusCreated, revisionJSON(revision), err)
}

func (h handler) revision(w http.ResponseWriter, r *http.Request, actor access.Actor) {
	number, err := strconv.Atoi(r.PathValue("number"))
	if err != nil {
		number = 0
	}
	revision, err := h.service.Revision(r.Context(), actor, r.PathValue("id"), number)
	h.respond(w, r, http.StatusOK, revisionJSON(revision), err)
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

type environmentJSONBody struct {
	ID             string    `json:"id"`
	Name           string    `json:"name"`
	ParadigmKey    string    `json:"paradigmKey"`
	Notes          *string   `json:"notes"`
	LatestRevision int       `json:"latestRevision"`
	CreatedAt      time.Time `json:"createdAt"`
	UpdatedAt      time.Time `json:"updatedAt"`
}

func environmentJSON(e domain.Environment) environmentJSONBody {
	return environmentJSONBody{e.ID, e.Name, e.ParadigmKey, nullable(e.Notes), e.LatestRevision, e.CreatedAt, e.UpdatedAt}
}

type revisionJSONBody struct {
	ID              string             `json:"id"`
	EnvironmentID   string             `json:"environmentId"`
	Number          int                `json:"number"`
	ParadigmKey     string             `json:"paradigmKey"`
	ParadigmVersion int                `json:"paradigmVersion"`
	Apparatus       map[string]float64 `json:"apparatus"`
	Notes           *string            `json:"notes"`
	CreatedBy       string             `json:"createdBy"`
	CreatedAt       time.Time          `json:"createdAt"`
}

type revisionSummaryJSONBody struct {
	ID              string `json:"id"`
	EnvironmentID   string `json:"environmentId"`
	EnvironmentName string `json:"environmentName"`
	ParadigmKey     string `json:"paradigmKey"`
	Number          int    `json:"number"`
	ParadigmVersion int    `json:"paradigmVersion"`
}

func revisionSummaryJSON(r domain.RevisionSummary) revisionSummaryJSONBody {
	return revisionSummaryJSONBody(r)
}

func revisionJSON(r domain.Revision) revisionJSONBody {
	apparatus := r.Apparatus
	if apparatus == nil {
		apparatus = map[string]float64{}
	}
	return revisionJSONBody{r.ID, r.EnvironmentID, r.Number, r.ParadigmKey, r.ParadigmVersion, apparatus, nullable(r.Notes), r.CreatedBy, r.CreatedAt}
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
