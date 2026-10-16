// Package scoringhttp exposes observer scoring of one animal of a test: the
// paradigm's input events, the scoring sheet and its events, completion into
// a SCORED analysis run and reopening as a new sheet version.
package scoringhttp

import (
	access "github.com/HappyHackingSpace/Misko/backend/internal/access/domain"
	paradigms "github.com/HappyHackingSpace/Misko/backend/internal/paradigms/domain"
	"github.com/HappyHackingSpace/Misko/backend/internal/platform/httpjson"
	"github.com/HappyHackingSpace/Misko/backend/internal/scoring/application"
	"github.com/HappyHackingSpace/Misko/backend/internal/scoring/domain"
	"log/slog"
	"net/http"
	"time"
)

var failures = []httpjson.Failure{
	{Err: access.ErrUnauthenticated, Status: http.StatusUnauthorized, Code: "auth.unauthenticated"},
	{Err: access.ErrForbidden, Status: http.StatusForbidden, Code: "auth.forbidden"},
	{Err: application.ErrTestSubjectNotFound, Status: http.StatusNotFound, Code: "scoring.testSubjectNotFound"},
	{Err: application.ErrEventNotFound, Status: http.StatusNotFound, Code: "scoring.eventNotFound"},
	{Err: application.ErrNoVideo, Status: http.StatusConflict, Code: "scoring.noVideo"},
	{Err: application.ErrNotScorable, Status: http.StatusConflict, Code: "scoring.notScorable"},
	{Err: application.ErrConflict, Status: http.StatusConflict, Code: "scoring.conflict"},
	{Err: application.ErrRejectedByEngine, Status: http.StatusUnprocessableEntity, Code: "scoring.invalidEvents"},
	{Err: domain.ErrNotObserverScored, Status: http.StatusConflict, Code: "scoring.notObserverScored"},
	{Err: domain.ErrNotDraft, Status: http.StatusConflict, Code: "scoring.notDraft"},
	{Err: domain.ErrNotCompleted, Status: http.StatusConflict, Code: "scoring.notCompleted"},
	{Err: domain.ErrSingleEvent, Status: http.StatusConflict, Code: "scoring.singleEvent"},
	{Err: domain.ErrTooManyEvents, Status: http.StatusConflict, Code: "scoring.tooManyEvents"},
	{Err: domain.ErrUnknownEventType, Status: http.StatusBadRequest, Code: "scoring.unknownEventType"},
	{Err: domain.ErrInvalidEventTime, Status: http.StatusBadRequest, Code: "scoring.invalidEventTime"},
	{Err: domain.ErrInvalidLabel, Status: http.StatusBadRequest, Code: "scoring.invalidLabel"},
	{Err: domain.ErrEventOutsideClip, Status: http.StatusBadRequest, Code: "scoring.eventOutsideClip"},
	{Err: domain.ErrUnknownScoredType, Status: http.StatusBadRequest, Code: "scoring.unknownScoredType"},
	{Err: domain.ErrUnscoredEvent, Status: http.StatusBadRequest, Code: "scoring.unscoredEvent"},
	{Err: domain.ErrInvalidDuration, Status: http.StatusBadRequest, Code: "scoring.invalidDuration"},
	{Err: domain.ErrEventAfterDuration, Status: http.StatusBadRequest, Code: "scoring.eventAfterDuration"},
}

type handler struct {
	service      *application.Service
	authenticate func(*http.Request) (access.Actor, error)
	logger       *slog.Logger
}

func Register(mux *http.ServeMux, service *application.Service, authenticate func(*http.Request) (access.Actor, error), logger *slog.Logger) {
	h := handler{service: service, authenticate: authenticate, logger: logger}
	const base = "/api/tests/{id}/subjects/{testSubjectId}/scoring"
	mux.HandleFunc("GET "+base, h.with(h.read))
	mux.HandleFunc("POST "+base+"/events", h.with(h.addEvent))
	mux.HandleFunc("PATCH "+base+"/events/{eventId}", h.with(h.changeEvent))
	mux.HandleFunc("DELETE "+base+"/events/{eventId}", h.with(h.removeEvent))
	mux.HandleFunc("POST "+base+"/complete", h.with(h.complete))
	mux.HandleFunc("POST "+base+"/reopen", h.with(h.reopen))
}

func (h handler) with(next func(http.ResponseWriter, *http.Request, access.Actor)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		actor, err := h.authenticate(r)
		if err != nil {
			h.error(w, r, err)
			return
		}
		next(w, r, actor)
	}
}

func (h handler) read(w http.ResponseWriter, r *http.Request, actor access.Actor) {
	view, err := h.service.Read(r.Context(), actor, r.PathValue("id"), r.PathValue("testSubjectId"))
	h.respond(w, r, http.StatusOK, func(lang string) any { return toView(view, lang) }, err)
}

type eventBody struct {
	Type    string `json:"type"`
	Label   string `json:"label"`
	StartUs int64  `json:"startUs"`
	EndUs   *int64 `json:"endUs"`
}

func (b eventBody) input() domain.EventInput {
	return domain.EventInput{Type: b.Type, Label: b.Label, StartUs: b.StartUs, EndUs: b.EndUs}
}

func (h handler) addEvent(w http.ResponseWriter, r *http.Request, actor access.Actor) {
	var body eventBody
	if !h.decode(w, r, &body) {
		return
	}
	o, err := h.service.AddEvent(r.Context(), actor, r.PathValue("id"), r.PathValue("testSubjectId"), body.input())
	h.respond(w, r, http.StatusCreated, func(string) any { return toEvent(o) }, err)
}

func (h handler) changeEvent(w http.ResponseWriter, r *http.Request, actor access.Actor) {
	var body eventBody
	if !h.decode(w, r, &body) {
		return
	}
	o, err := h.service.ChangeEvent(r.Context(), actor, r.PathValue("id"), r.PathValue("testSubjectId"), r.PathValue("eventId"), body.input())
	h.respond(w, r, http.StatusOK, func(string) any { return toEvent(o) }, err)
}

func (h handler) removeEvent(w http.ResponseWriter, r *http.Request, actor access.Actor) {
	if err := h.service.RemoveEvent(r.Context(), actor, r.PathValue("id"), r.PathValue("testSubjectId"), r.PathValue("eventId")); err != nil {
		h.error(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h handler) complete(w http.ResponseWriter, r *http.Request, actor access.Actor) {
	var body struct {
		ScoredEventTypes []string `json:"scoredEventTypes"`
		DurationUs       *int64   `json:"durationUs"`
	}
	if !h.decode(w, r, &body) {
		return
	}
	c, err := h.service.Complete(r.Context(), actor, r.PathValue("id"), r.PathValue("testSubjectId"),
		domain.CompleteInput{ScoredTypes: body.ScoredEventTypes, DurationUs: body.DurationUs})
	h.respond(w, r, http.StatusOK, func(lang string) any {
		out := completionJSON{viewJSON: toView(c.View, lang)}
		out.Run.ID, out.Run.Metrics = c.RunID, make([]metricJSON, 0, len(c.Computed.Metrics))
		for _, m := range c.Computed.Metrics {
			out.Run.Metrics = append(out.Run.Metrics, metricJSON{m.Key, m.Unit, m.Value, nullable(m.MissingReason)})
		}
		return out
	}, err)
}

func (h handler) reopen(w http.ResponseWriter, r *http.Request, actor access.Actor) {
	view, err := h.service.Reopen(r.Context(), actor, r.PathValue("id"), r.PathValue("testSubjectId"))
	h.respond(w, r, http.StatusCreated, func(lang string) any { return toView(view, lang) }, err)
}

func (h handler) decode(w http.ResponseWriter, r *http.Request, dst any) bool {
	if err := httpjson.Decode(w, r, dst); err != nil {
		h.error(w, r, err)
		return false
	}
	return true
}

// respond writes the body built in the request's language, or the error.
func (h handler) respond(w http.ResponseWriter, r *http.Request, status int, body func(lang string) any, err error) {
	if err != nil {
		h.error(w, r, err)
		return
	}
	lang := httpjson.Language(r, paradigms.Languages)
	w.Header().Set("Content-Language", lang)
	httpjson.Write(w, status, body(lang))
}

func (h handler) error(w http.ResponseWriter, r *http.Request, err error) {
	httpjson.Fail(w, r, h.logger, err, failures)
}

type inputEventJSON struct {
	Type       string   `json:"type"`
	Kind       string   `json:"kind"`
	Labels     []string `json:"labels"`
	Single     bool     `json:"single"`
	Label      string   `json:"label"`
	Definition string   `json:"definition"`
}

type videoJSON struct {
	RecordingID string `json:"recordingId"`
	ClipStartUs int64  `json:"clipStartUs"`
	ClipEndUs   *int64 `json:"clipEndUs"`
}

type sheetJSON struct {
	ID               string     `json:"id"`
	Version          int        `json:"version"`
	Status           string     `json:"status"`
	ScoredEventTypes []string   `json:"scoredEventTypes"`
	DurationUs       *int64     `json:"durationUs"`
	RecordingID      *string    `json:"recordingId"`
	RunID            *string    `json:"runId"`
	CreatedBy        string     `json:"createdBy"`
	CreatedAt        time.Time  `json:"createdAt"`
	CompletedBy      *string    `json:"completedBy"`
	CompletedAt      *time.Time `json:"completedAt"`
}

type eventJSON struct {
	ID        string    `json:"id"`
	Type      string    `json:"type"`
	Kind      string    `json:"kind"`
	Label     *string   `json:"label"`
	StartUs   int64     `json:"startUs"`
	EndUs     int64     `json:"endUs"`
	ScoredBy  string    `json:"scoredBy"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type paradigmJSON struct {
	Key     string `json:"key"`
	Version int    `json:"version"`
}

type viewJSON struct {
	ExperimentID  string           `json:"experimentId"`
	TestID        string           `json:"testId"`
	TestSubjectID string           `json:"testSubjectId"`
	Paradigm      paradigmJSON     `json:"paradigm"`
	InputEvents   []inputEventJSON `json:"inputEvents"`
	Video         *videoJSON       `json:"video"`
	Sheet         *sheetJSON       `json:"sheet"`
	Events        []eventJSON      `json:"events"`
}

type metricJSON struct {
	Key           string   `json:"key"`
	Unit          string   `json:"unit"`
	Value         *float64 `json:"value"`
	MissingReason *string  `json:"missingReason"`
}

type completionJSON struct {
	viewJSON
	Run struct {
		ID      string       `json:"id"`
		Metrics []metricJSON `json:"metrics"`
	} `json:"run"`
}

func toView(v application.View, lang string) viewJSON {
	out := viewJSON{
		ExperimentID: v.Subject.ExperimentID, TestID: v.Subject.TestID, TestSubjectID: v.Subject.TestSubjectID,
		Paradigm: paradigmJSON{v.Paradigm.Key, v.Paradigm.Version}, InputEvents: make([]inputEventJSON, 0, len(v.Paradigm.Events)),
		Events: make([]eventJSON, 0, len(v.Events)),
	}
	for _, e := range v.Paradigm.Events {
		labels := e.Labels
		if labels == nil {
			labels = []string{}
		}
		out.InputEvents = append(out.InputEvents, inputEventJSON{e.Type, string(e.Kind), labels, e.Single, text(e.Label, lang), text(e.Definition, lang)})
	}
	if v.Subject.Video != nil {
		out.Video = &videoJSON{v.Subject.Video.RecordingID, v.Subject.Video.StartUs, v.Subject.Video.EndUs}
	}
	if s := v.Sheet; s != nil {
		out.Sheet = &sheetJSON{
			ID: s.ID, Version: s.Version, Status: string(s.Status), ScoredEventTypes: s.ScoredTypes, DurationUs: s.DurationUs,
			RecordingID: nullable(s.RecordingID), RunID: nullable(s.RunID), CreatedBy: s.CreatedBy, CreatedAt: s.CreatedAt,
			CompletedBy: nullable(s.CompletedBy), CompletedAt: s.CompletedAt,
		}
	}
	for _, e := range v.Events {
		out.Events = append(out.Events, toEvent(e))
	}
	return out
}

func toEvent(o domain.Observation) eventJSON {
	return eventJSON{o.ID, o.Type, string(o.Kind), nullable(o.Label), o.StartUs, o.EndUs, o.ScoredBy, o.CreatedAt, o.UpdatedAt}
}

func text(t domain.Text, lang string) string {
	if lang == "tr" && t.TR != "" {
		return t.TR
	}
	return t.EN
}

func nullable(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
