// Package subjectshttp exposes subjects over HTTP.
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
	{Err: domain.ErrInvalidStrain, Status: http.StatusBadRequest, Code: "subject.invalidStrain"},
	{Err: domain.ErrInvalidBirthDate, Status: http.StatusBadRequest, Code: "subject.invalidBirthDate"},
	{Err: domain.ErrInvalidNotes, Status: http.StatusBadRequest, Code: "subject.invalidNotes"},
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
}

func (h handler) list(w http.ResponseWriter, r *http.Request) {
	actor, ok := h.actor(w, r)
	if !ok {
		return
	}
	q := r.URL.Query()
	page, err := h.service.List(r.Context(), actor, application.Query{
		Search: q.Get("search"), Species: q.Get("species"), Sex: q.Get("sex"), Sort: q.Get("sort"), Order: q.Get("order"),
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
		Code      string `json:"code"`
		Species   string `json:"species"`
		Sex       string `json:"sex"`
		Strain    string `json:"strain"`
		BirthDate string `json:"birthDate"`
		Notes     string `json:"notes"`
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

// update changes provided fields; an empty strain, birthDate or notes clears it.
func (h handler) update(w http.ResponseWriter, r *http.Request) {
	actor, ok := h.actor(w, r)
	if !ok {
		return
	}
	var body struct {
		Code      *string `json:"code"`
		Species   *string `json:"species"`
		Sex       *string `json:"sex"`
		Strain    *string `json:"strain"`
		BirthDate *string `json:"birthDate"`
		Notes     *string `json:"notes"`
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
	ID        string    `json:"id"`
	Code      string    `json:"code"`
	Species   string    `json:"species"`
	Sex       string    `json:"sex"`
	Strain    *string   `json:"strain"`
	BirthDate *string   `json:"birthDate"`
	Notes     *string   `json:"notes"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

func toJSON(s domain.Subject) subjectJSON {
	out := subjectJSON{ID: s.ID, Code: s.Code, Species: string(s.Species), Sex: string(s.Sex), Strain: nullable(s.Strain), Notes: nullable(s.Notes), CreatedAt: s.CreatedAt, UpdatedAt: s.UpdatedAt}
	if s.BirthDate != nil {
		out.BirthDate = nullable(s.BirthDate.Format(time.DateOnly))
	}
	return out
}

func nullable(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
