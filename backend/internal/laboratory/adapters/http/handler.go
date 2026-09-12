// Package laboratoryhttp exposes the laboratory singleton over HTTP.
package laboratoryhttp

import (
	"errors"
	access "github.com/HappyHackingSpace/Misko/backend/internal/access/domain"
	"github.com/HappyHackingSpace/Misko/backend/internal/laboratory/application"
	"github.com/HappyHackingSpace/Misko/backend/internal/laboratory/domain"
	"github.com/HappyHackingSpace/Misko/backend/internal/platform/httpjson"
	"log/slog"
	"net/http"
	"time"
)

var failures = []httpjson.Failure{
	{Err: access.ErrUnauthenticated, Status: http.StatusUnauthorized, Code: "auth.unauthenticated"},
	{Err: access.ErrForbidden, Status: http.StatusForbidden, Code: "auth.forbidden"},
	{Err: application.ErrNotInitialized, Status: http.StatusNotFound, Code: "lab.notInitialized"},
	{Err: application.ErrNoChanges, Status: http.StatusBadRequest, Code: "common.noChanges"},
	{Err: domain.ErrInvalidName, Status: http.StatusBadRequest, Code: "lab.invalidName"},
	{Err: domain.ErrInvalidCode, Status: http.StatusBadRequest, Code: "lab.invalidCode"},
	{Err: domain.ErrInvalidTimezone, Status: http.StatusBadRequest, Code: "lab.invalidTimezone"},
}

type handler struct {
	service      *application.Service
	authenticate func(*http.Request) (access.Actor, error)
	logger       *slog.Logger
}

func Register(mux *http.ServeMux, service *application.Service, authenticate func(*http.Request) (access.Actor, error), logger *slog.Logger) {
	h := handler{service: service, authenticate: authenticate, logger: logger}
	mux.HandleFunc("GET /api/meta", h.meta)
	mux.HandleFunc("GET /api/lab", h.laboratory)
	mux.HandleFunc("PATCH /api/lab", h.update)
}

// meta is public branding for the sign-in screen; labName is null before setup.
func (h handler) meta(w http.ResponseWriter, r *http.Request) {
	name, err := h.service.PublicName(r.Context())
	var labName *string
	switch {
	case err == nil:
		labName = &name
	case !errors.Is(err, application.ErrNotInitialized):
		h.fail(w, r, err)
		return
	}
	httpjson.Write(w, http.StatusOK, struct {
		AppName string  `json:"appName"`
		LabName *string `json:"labName"`
	}{"Mişko", labName})
}

func (h handler) laboratory(w http.ResponseWriter, r *http.Request) {
	actor, err := h.authenticate(r)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	lab, err := h.service.Laboratory(r.Context(), actor)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpjson.Write(w, http.StatusOK, toJSON(lab))
}

// update applies provided fields only; an empty code clears it.
func (h handler) update(w http.ResponseWriter, r *http.Request) {
	actor, err := h.authenticate(r)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	var body struct {
		Name     *string `json:"name"`
		Code     *string `json:"code"`
		Timezone *string `json:"timezone"`
	}
	if err := httpjson.Decode(w, r, &body); err != nil {
		h.fail(w, r, err)
		return
	}
	lab, err := h.service.Update(r.Context(), actor, application.Update{Name: body.Name, Code: body.Code, Timezone: body.Timezone})
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpjson.Write(w, http.StatusOK, toJSON(lab))
}

func (h handler) fail(w http.ResponseWriter, r *http.Request, err error) {
	httpjson.Fail(w, r, h.logger, err, failures)
}

type laboratoryJSON struct {
	Name      string    `json:"name"`
	Code      *string   `json:"code"`
	Timezone  string    `json:"timezone"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

func toJSON(lab domain.Laboratory) laboratoryJSON {
	out := laboratoryJSON{Name: lab.Name, Timezone: lab.Timezone, CreatedAt: lab.CreatedAt, UpdatedAt: lab.UpdatedAt}
	if lab.Code != "" {
		out.Code = &lab.Code
	}
	return out
}
