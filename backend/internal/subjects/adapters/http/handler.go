// Package subjectshttp exposes subjects and their body weights over HTTP.
package subjectshttp

import (
	access "github.com/HappyHackingSpace/Misko/backend/internal/access/domain"
	"github.com/HappyHackingSpace/Misko/backend/internal/platform/httpjson"
	"github.com/HappyHackingSpace/Misko/backend/internal/subjects/application"
	"github.com/HappyHackingSpace/Misko/backend/internal/subjects/domain"
	"log/slog"
	"net/http"
	"time"
)

var failures = []httpjson.Failure{
	{Err: access.ErrUnauthenticated, Status: http.StatusUnauthorized, Code: "auth.unauthenticated"},
	{Err: access.ErrForbidden, Status: http.StatusForbidden, Code: "auth.forbidden"},
	{Err: application.ErrNotFound, Status: http.StatusNotFound, Code: "subject.notFound"},
	{Err: application.ErrCodeTaken, Status: http.StatusConflict, Code: "subject.codeTaken"},
	{Err: application.ErrInUse, Status: http.StatusConflict, Code: "subject.inUse"},
	{Err: application.ErrNoChanges, Status: http.StatusBadRequest, Code: "common.noChanges"},
	{Err: application.ErrInvalidQuery, Status: http.StatusBadRequest, Code: "common.invalidQuery"},
	{Err: domain.ErrInvalidCode, Status: http.StatusBadRequest, Code: "subject.invalidCode"},
	{Err: domain.ErrInvalidSpecies, Status: http.StatusBadRequest, Code: "subject.invalidSpecies"},
	{Err: domain.ErrInvalidSex, Status: http.StatusBadRequest, Code: "subject.invalidSex"},
	{Err: domain.ErrInvalidWeight, Status: http.StatusBadRequest, Code: "weight.invalid"},
	{Err: domain.ErrMissingTime, Status: http.StatusBadRequest, Code: "common.invalidTime"},
	{Err: domain.ErrFutureTime, Status: http.StatusBadRequest, Code: "common.futureTime"},
}

type handler struct {
	service      *application.Service
	authenticate func(*http.Request) (access.Actor, error)
	logger       *slog.Logger
}

func Register(mux *http.ServeMux, service *application.Service, authenticate func(*http.Request) (access.Actor, error), logger *slog.Logger) {
	h := handler{service: service, authenticate: authenticate, logger: logger}
	mux.HandleFunc("GET /api/subjects", h.list)
	mux.HandleFunc("POST /api/subjects", h.create)
	mux.HandleFunc("GET /api/subjects/{id}", h.get)
	mux.HandleFunc("PATCH /api/subjects/{id}", h.update)
	mux.HandleFunc("DELETE /api/subjects/{id}", h.delete)
	mux.HandleFunc("GET /api/subjects/{id}/weights", h.listWeights)
	mux.HandleFunc("POST /api/subjects/{id}/weights", h.recordWeight)
}

func (h handler) list(w http.ResponseWriter, r *http.Request) {
	actor, ok := h.actor(w, r)
	if !ok {
		return
	}
	q := r.URL.Query()
	page, err := h.service.List(r.Context(), actor, application.Query{
		Search: q.Get("search"), Species: q.Get("species"), Sex: q.Get("sex"), Unallocated: q.Get("unallocated"), Sort: q.Get("sort"), Order: q.Get("order"),
		Page: httpjson.QueryInt(q.Get("page")), PageSize: httpjson.QueryInt(q.Get("pageSize")),
	})
	if err != nil {
		h.fail(w, r, err)
		return
	}
	data := make([]subjectJSON, 0, len(page.Subjects))
	for _, s := range page.Subjects {
		data = append(data, toJSON(s))
	}
	httpjson.Write(w, http.StatusOK, struct {
		Data     []subjectJSON `json:"data"`
		Total    int           `json:"total"`
		Page     int           `json:"page"`
		PageSize int           `json:"pageSize"`
	}{data, page.Total, page.Page, page.PageSize})
}

func (h handler) create(w http.ResponseWriter, r *http.Request) {
	actor, ok := h.actor(w, r)
	if !ok {
		return
	}
	var body struct {
		Code    string `json:"code"`
		Species string `json:"species"`
		Sex     string `json:"sex"`
	}
	if err := httpjson.Decode(w, r, &body); err != nil {
		h.fail(w, r, err)
		return
	}
	subject, err := h.service.Create(r.Context(), actor, application.Input(body))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpjson.Write(w, http.StatusCreated, toJSON(subject))
}

func (h handler) get(w http.ResponseWriter, r *http.Request) {
	actor, ok := h.actor(w, r)
	if !ok {
		return
	}
	subject, err := h.service.Subject(r.Context(), actor, r.PathValue("id"))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpjson.Write(w, http.StatusOK, toJSON(subject))
}

// update changes the provided fields.
func (h handler) update(w http.ResponseWriter, r *http.Request) {
	actor, ok := h.actor(w, r)
	if !ok {
		return
	}
	var body struct {
		Code    *string `json:"code"`
		Species *string `json:"species"`
		Sex     *string `json:"sex"`
	}
	if err := httpjson.Decode(w, r, &body); err != nil {
		h.fail(w, r, err)
		return
	}
	subject, err := h.service.Update(r.Context(), actor, r.PathValue("id"), application.Patch(body))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpjson.Write(w, http.StatusOK, toJSON(subject))
}

func (h handler) delete(w http.ResponseWriter, r *http.Request) {
	actor, ok := h.actor(w, r)
	if !ok {
		return
	}
	if err := h.service.Delete(r.Context(), actor, r.PathValue("id")); err != nil {
		h.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h handler) listWeights(w http.ResponseWriter, r *http.Request) {
	actor, ok := h.actor(w, r)
	if !ok {
		return
	}
	weights, err := h.service.Weights(r.Context(), actor, r.PathValue("id"))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	data := make([]weightJSON, 0, len(weights))
	for _, weight := range weights {
		data = append(data, toWeightJSON(weight))
	}
	httpjson.Write(w, http.StatusOK, struct {
		Data []weightJSON `json:"data"`
	}{data})
}

func (h handler) recordWeight(w http.ResponseWriter, r *http.Request) {
	actor, ok := h.actor(w, r)
	if !ok {
		return
	}
	var body struct {
		Grams      string    `json:"grams"`
		MeasuredAt time.Time `json:"measuredAt"`
	}
	if err := httpjson.Decode(w, r, &body); err != nil {
		h.fail(w, r, err)
		return
	}
	weight, err := h.service.RecordWeight(r.Context(), actor, r.PathValue("id"), application.WeightInput(body))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpjson.Write(w, http.StatusCreated, toWeightJSON(weight))
}

func (h handler) actor(w http.ResponseWriter, r *http.Request) (access.Actor, bool) {
	actor, err := h.authenticate(r)
	if err != nil {
		h.fail(w, r, err)
		return access.Actor{}, false
	}
	return actor, true
}

func (h handler) fail(w http.ResponseWriter, r *http.Request, err error) {
	httpjson.Fail(w, r, h.logger, err, failures)
}

type subjectJSON struct {
	ID         string          `json:"id"`
	Code       string          `json:"code"`
	Species    string          `json:"species"`
	Sex        string          `json:"sex"`
	Allocation *allocationJSON `json:"allocation"`
	CreatedAt  time.Time       `json:"createdAt"`
	UpdatedAt  time.Time       `json:"updatedAt"`
}

type allocationJSON struct {
	ExperimentID   string `json:"experimentId"`
	ExperimentCode string `json:"experimentCode"`
	GroupID        string `json:"groupId"`
	GroupCode      string `json:"groupCode"`
	GroupRole      string `json:"groupRole"`
	Excluded       bool   `json:"excluded"`
}

func toJSON(s domain.Subject) subjectJSON {
	out := subjectJSON{ID: s.ID, Code: s.Code, Species: string(s.Species), Sex: string(s.Sex), CreatedAt: s.CreatedAt, UpdatedAt: s.UpdatedAt}
	if a := s.Allocation; a != nil {
		out.Allocation = &allocationJSON{a.ExperimentID, a.ExperimentCode, a.GroupID, a.GroupCode, a.GroupRole, a.Excluded}
	}
	return out
}

type weightJSON struct {
	ID         string    `json:"id"`
	SubjectID  string    `json:"subjectId"`
	Grams      string    `json:"grams"`
	MeasuredAt time.Time `json:"measuredAt"`
	RecordedBy string    `json:"recordedBy"`
	CreatedAt  time.Time `json:"createdAt"`
}

func toWeightJSON(w domain.Weight) weightJSON {
	return weightJSON{w.ID, w.SubjectID, domain.FormatGrams(w.Milligrams), w.MeasuredAt, w.RecordedBy, w.CreatedAt}
}
