// Package cataloghttp exposes substances and disease models over HTTP.
package cataloghttp

import (
	access "github.com/HappyHackingSpace/Misko/backend/internal/access/domain"
	"github.com/HappyHackingSpace/Misko/backend/internal/catalog/application"
	"github.com/HappyHackingSpace/Misko/backend/internal/catalog/domain"
	"github.com/HappyHackingSpace/Misko/backend/internal/platform/httpjson"
	"log/slog"
	"net/http"
	"time"
)

var failures = []httpjson.Failure{
	{Err: access.ErrUnauthenticated, Status: http.StatusUnauthorized, Code: "auth.unauthenticated"},
	{Err: access.ErrForbidden, Status: http.StatusForbidden, Code: "auth.forbidden"},
	{Err: application.ErrSubstanceNotFound, Status: http.StatusNotFound, Code: "substance.notFound"},
	{Err: application.ErrDiseaseModelNotFound, Status: http.StatusNotFound, Code: "diseaseModel.notFound"},
	{Err: application.ErrNameTaken, Status: http.StatusConflict, Code: "catalog.nameTaken"},
	{Err: application.ErrSubstanceInUse, Status: http.StatusConflict, Code: "substance.inUse"},
	{Err: application.ErrDiseaseModelInUse, Status: http.StatusConflict, Code: "diseaseModel.inUse"},
	{Err: application.ErrNoChanges, Status: http.StatusBadRequest, Code: "common.noChanges"},
	{Err: application.ErrInvalidQuery, Status: http.StatusBadRequest, Code: "common.invalidQuery"},
	{Err: domain.ErrInvalidName, Status: http.StatusBadRequest, Code: "common.invalidName"},
	{Err: domain.ErrInvalidDescription, Status: http.StatusBadRequest, Code: "common.invalidDescription"},
	{Err: domain.ErrInvalidSubstanceKind, Status: http.StatusBadRequest, Code: "substance.invalidKind"},
	{Err: domain.ErrInvalidInductionType, Status: http.StatusBadRequest, Code: "diseaseModel.invalidInductionType"},
	{Err: domain.ErrInvalidConfirmationCriterion, Status: http.StatusBadRequest, Code: "diseaseModel.invalidConfirmationCriterion"},
}

type handler struct {
	service      *application.Service
	authenticate func(*http.Request) (access.Actor, error)
	logger       *slog.Logger
}

func Register(mux *http.ServeMux, service *application.Service, authenticate func(*http.Request) (access.Actor, error), logger *slog.Logger) {
	h := handler{service: service, authenticate: authenticate, logger: logger}
	mux.HandleFunc("GET /api/catalog/substances", h.with(h.listSubstances))
	mux.HandleFunc("POST /api/catalog/substances", h.with(h.createSubstance))
	mux.HandleFunc("GET /api/catalog/substances/{id}", h.with(h.getSubstance))
	mux.HandleFunc("PATCH /api/catalog/substances/{id}", h.with(h.updateSubstance))
	mux.HandleFunc("DELETE /api/catalog/substances/{id}", h.with(h.deleteSubstance))
	mux.HandleFunc("GET /api/catalog/disease-models", h.with(h.listDiseaseModels))
	mux.HandleFunc("POST /api/catalog/disease-models", h.with(h.createDiseaseModel))
	mux.HandleFunc("GET /api/catalog/disease-models/{id}", h.with(h.getDiseaseModel))
	mux.HandleFunc("PATCH /api/catalog/disease-models/{id}", h.with(h.updateDiseaseModel))
	mux.HandleFunc("DELETE /api/catalog/disease-models/{id}", h.with(h.deleteDiseaseModel))
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

func (h handler) listSubstances(w http.ResponseWriter, r *http.Request, actor access.Actor) {
	q := r.URL.Query()
	substances, err := h.service.Substances(r.Context(), actor, application.SubstanceQuery{Search: q.Get("search"), Kind: q.Get("kind")})
	h.respond(w, r, http.StatusOK, dataJSON{mapSlice(substances, substanceJSON)}, err)
}

func (h handler) getSubstance(w http.ResponseWriter, r *http.Request, actor access.Actor) {
	substance, err := h.service.Substance(r.Context(), actor, r.PathValue("id"))
	h.respond(w, r, http.StatusOK, substanceJSON(substance), err)
}

func (h handler) createSubstance(w http.ResponseWriter, r *http.Request, actor access.Actor) {
	var body struct {
		Name        string `json:"name"`
		Kind        string `json:"kind"`
		Description string `json:"description"`
	}
	if !h.decode(w, r, &body) {
		return
	}
	substance, err := h.service.CreateSubstance(r.Context(), actor, application.SubstanceInput(body))
	h.respond(w, r, http.StatusCreated, substanceJSON(substance), err)
}

func (h handler) updateSubstance(w http.ResponseWriter, r *http.Request, actor access.Actor) {
	var body struct {
		Name        *string `json:"name"`
		Kind        *string `json:"kind"`
		Description *string `json:"description"`
	}
	if !h.decode(w, r, &body) {
		return
	}
	substance, err := h.service.UpdateSubstance(r.Context(), actor, r.PathValue("id"), application.SubstancePatch(body))
	h.respond(w, r, http.StatusOK, substanceJSON(substance), err)
}

func (h handler) deleteSubstance(w http.ResponseWriter, r *http.Request, actor access.Actor) {
	h.noContent(w, r, h.service.DeleteSubstance(r.Context(), actor, r.PathValue("id")))
}

func (h handler) listDiseaseModels(w http.ResponseWriter, r *http.Request, actor access.Actor) {
	q := r.URL.Query()
	models, err := h.service.DiseaseModels(r.Context(), actor, application.DiseaseModelQuery{Search: q.Get("search"), InductionType: q.Get("inductionType")})
	h.respond(w, r, http.StatusOK, dataJSON{mapSlice(models, diseaseModelJSON)}, err)
}

func (h handler) getDiseaseModel(w http.ResponseWriter, r *http.Request, actor access.Actor) {
	model, err := h.service.DiseaseModel(r.Context(), actor, r.PathValue("id"))
	h.respond(w, r, http.StatusOK, diseaseModelJSON(model), err)
}

func (h handler) createDiseaseModel(w http.ResponseWriter, r *http.Request, actor access.Actor) {
	var body struct {
		Name                  string `json:"name"`
		InductionType         string `json:"inductionType"`
		ConfirmationCriterion string `json:"confirmationCriterion"`
		Description           string `json:"description"`
	}
	if !h.decode(w, r, &body) {
		return
	}
	model, err := h.service.CreateDiseaseModel(r.Context(), actor, application.DiseaseModelInput(body))
	h.respond(w, r, http.StatusCreated, diseaseModelJSON(model), err)
}

func (h handler) updateDiseaseModel(w http.ResponseWriter, r *http.Request, actor access.Actor) {
	var body struct {
		Name                  *string `json:"name"`
		InductionType         *string `json:"inductionType"`
		ConfirmationCriterion *string `json:"confirmationCriterion"`
		Description           *string `json:"description"`
	}
	if !h.decode(w, r, &body) {
		return
	}
	model, err := h.service.UpdateDiseaseModel(r.Context(), actor, r.PathValue("id"), application.DiseaseModelPatch(body))
	h.respond(w, r, http.StatusOK, diseaseModelJSON(model), err)
}

func (h handler) deleteDiseaseModel(w http.ResponseWriter, r *http.Request, actor access.Actor) {
	h.noContent(w, r, h.service.DeleteDiseaseModel(r.Context(), actor, r.PathValue("id")))
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

func (h handler) noContent(w http.ResponseWriter, r *http.Request, err error) {
	if err != nil {
		h.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h handler) fail(w http.ResponseWriter, r *http.Request, err error) {
	httpjson.Fail(w, r, h.logger, err, failures)
}

type dataJSON struct {
	Data any `json:"data"`
}

type substanceJSONBody struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Kind        string    `json:"kind"`
	Description *string   `json:"description"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

func substanceJSON(s domain.Substance) substanceJSONBody {
	return substanceJSONBody{s.ID, s.Name, string(s.Kind), nullable(s.Description), s.CreatedAt, s.UpdatedAt}
}

type diseaseModelJSONBody struct {
	ID                    string    `json:"id"`
	Name                  string    `json:"name"`
	InductionType         string    `json:"inductionType"`
	ConfirmationCriterion *string   `json:"confirmationCriterion"`
	Description           *string   `json:"description"`
	CreatedAt             time.Time `json:"createdAt"`
	UpdatedAt             time.Time `json:"updatedAt"`
}

func diseaseModelJSON(m domain.DiseaseModel) diseaseModelJSONBody {
	return diseaseModelJSONBody{m.ID, m.Name, string(m.InductionType), nullable(m.ConfirmationCriterion), nullable(m.Description), m.CreatedAt, m.UpdatedAt}
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
