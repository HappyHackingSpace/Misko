// Package trialshttp exposes paradigm trials over HTTP: create, list, read
// and delete trials, upload and verify a trial's video and analyse it again
// with changed ANALYSIS thresholds. Calibrating on the trial's video is served
// by the calibration module under /api/trials/{id}/calibrations, and a job's
// events and videos by the analysis service's job routes.
package trialshttp

import (
	"log/slog"
	"net/http"
	"time"

	access "github.com/HappyHackingSpace/Misko/backend/internal/access/domain"
	"github.com/HappyHackingSpace/Misko/backend/internal/platform/httpjson"
	"github.com/HappyHackingSpace/Misko/backend/internal/trials/application"
	"github.com/HappyHackingSpace/Misko/backend/internal/trials/domain"
)

var failures = []httpjson.Failure{
	{Err: access.ErrUnauthenticated, Status: http.StatusUnauthorized, Code: "auth.unauthenticated"},
	{Err: access.ErrForbidden, Status: http.StatusForbidden, Code: "auth.forbidden"},
	{Err: application.ErrTrialNotFound, Status: http.StatusNotFound, Code: "trial.notFound"},
	{Err: application.ErrRevisionNotFound, Status: http.StatusNotFound, Code: "environment.revisionNotFound"},
	{Err: application.ErrParadigmMismatch, Status: http.StatusBadRequest, Code: "trial.paradigmMismatch"},
	{Err: application.ErrNoVideo, Status: http.StatusConflict, Code: "trial.noVideo"},
	{Err: application.ErrVideoNotVerified, Status: http.StatusConflict, Code: "trial.videoNotVerified"},
	{Err: application.ErrNotReady, Status: http.StatusConflict, Code: "trial.notReady"},
	{Err: application.ErrInvalidQuery, Status: http.StatusBadRequest, Code: "trial.invalidQuery"},
	{Err: application.ErrInvalidUpload, Status: http.StatusBadRequest, Code: "trial.invalidUpload"},
	{Err: application.ErrUploadIncomplete, Status: http.StatusConflict, Code: "media.uploadIncomplete"},
	{Err: application.ErrStorageNotConfigured, Status: http.StatusServiceUnavailable, Code: "media.storageNotConfigured"},
	{Err: domain.ErrInvalidName, Status: http.StatusBadRequest, Code: "trial.invalidName"},
	{Err: domain.ErrInvalidNotes, Status: http.StatusBadRequest, Code: "trial.invalidNotes"},
	{Err: domain.ErrInvalidClip, Status: http.StatusBadRequest, Code: "trial.invalidClip"},
	{Err: domain.ErrInvalidOverrides, Status: http.StatusBadRequest, Code: "trial.invalidOverrides"},
	{Err: domain.ErrDeleted, Status: http.StatusNotFound, Code: "trial.notFound"},
}

type handler struct {
	service      *application.Service
	authenticate func(*http.Request) (access.Actor, error)
	logger       *slog.Logger
}

func Register(mux *http.ServeMux, service *application.Service, authenticate func(*http.Request) (access.Actor, error), logger *slog.Logger) {
	h := handler{service: service, authenticate: authenticate, logger: logger}
	mux.HandleFunc("POST /api/trials", h.with(h.create))
	mux.HandleFunc("GET /api/trials", h.with(h.list))
	mux.HandleFunc("GET /api/trials/{id}", h.with(h.get))
	mux.HandleFunc("DELETE /api/trials/{id}", h.with(h.delete))
	mux.HandleFunc("POST /api/trials/{id}/video", h.with(h.startUpload))
	mux.HandleFunc("POST /api/trials/{id}/video/finalize", h.with(h.finalize))
	mux.HandleFunc("GET /api/trials/{id}/video/read-url", h.with(h.videoURL))
	mux.HandleFunc("POST /api/trials/{id}/analyses", h.with(h.analyse))
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

func (h handler) create(w http.ResponseWriter, r *http.Request, actor access.Actor) {
	var body struct {
		Name                  string `json:"name"`
		ParadigmKey           string `json:"paradigmKey"`
		ParadigmVersion       int    `json:"paradigmVersion"`
		EnvironmentRevisionID string `json:"environmentRevisionId"`
		Notes                 string `json:"notes"`
	}
	if !h.decode(w, r, &body) {
		return
	}
	t, err := h.service.Create(r.Context(), actor, application.CreateInput(body))
	h.respond(w, r, http.StatusCreated, toTrial(t), err)
}

func (h handler) list(w http.ResponseWriter, r *http.Request, actor access.Actor) {
	v := r.URL.Query()
	page, err := h.service.List(r.Context(), actor, application.ListQuery{Search: v.Get("search"), ParadigmKey: v.Get("paradigmKey"),
		Page: httpjson.QueryInt(v.Get("page")), PageSize: httpjson.QueryInt(v.Get("pageSize"))})
	h.respond(w, r, http.StatusOK, struct {
		Data     []summaryJSON `json:"data"`
		Total    int           `json:"total"`
		Page     int           `json:"page"`
		PageSize int           `json:"pageSize"`
	}{mapSlice(page.Items, toSummary), page.Total, page.Page, page.PageSize}, err)
}

func (h handler) get(w http.ResponseWriter, r *http.Request, actor access.Actor) {
	detail, err := h.service.Get(r.Context(), actor, r.PathValue("id"))
	h.respond(w, r, http.StatusOK, toDetail(detail), err)
}

func (h handler) delete(w http.ResponseWriter, r *http.Request, actor access.Actor) {
	if err := h.service.Delete(r.Context(), actor, r.PathValue("id")); err != nil {
		h.error(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h handler) startUpload(w http.ResponseWriter, r *http.Request, actor access.Actor) {
	var body struct {
		FileName    string `json:"fileName"`
		ContentType string `json:"contentType"`
		SizeBytes   int64  `json:"sizeBytes"`
		CRC32C      string `json:"crc32c"`
		ClipStartUs int64  `json:"clipStartUs"`
		ClipEndUs   *int64 `json:"clipEndUs"`
	}
	if !h.decode(w, r, &body) {
		return
	}
	upload, err := h.service.StartUpload(r.Context(), actor, r.PathValue("id"), domain.UploadRequest{FileName: body.FileName, ContentType: body.ContentType,
		SizeBytes: body.SizeBytes, CRC32C: body.CRC32C, Clip: domain.Clip{StartUs: body.ClipStartUs, EndUs: body.ClipEndUs}})
	h.respond(w, r, http.StatusCreated, struct {
		Trial  trialJSON  `json:"trial"`
		Video  videoJSON  `json:"video"`
		Upload uploadJSON `json:"upload"`
	}{toTrial(upload.Trial), toVideo(upload.Video), uploadJSON(upload.Request)}, err)
}

func (h handler) finalize(w http.ResponseWriter, r *http.Request, actor access.Actor) {
	var body struct{}
	if !h.decode(w, r, &body) {
		return
	}
	detail, err := h.service.FinalizeUpload(r.Context(), actor, r.PathValue("id"))
	h.respond(w, r, http.StatusOK, toDetail(detail), err)
}

func (h handler) videoURL(w http.ResponseWriter, r *http.Request, actor access.Actor) {
	url, expires, err := h.service.VideoURL(r.Context(), actor, r.PathValue("id"))
	h.respond(w, r, http.StatusOK, struct {
		URL       string    `json:"url"`
		ExpiresAt time.Time `json:"expiresAt"`
	}{url, expires}, err)
}

func (h handler) analyse(w http.ResponseWriter, r *http.Request, actor access.Actor) {
	var body struct {
		Overrides map[string]float64 `json:"overrides"`
	}
	if !h.decode(w, r, &body) {
		return
	}
	analysis, err := h.service.Analyse(r.Context(), actor, r.PathValue("id"), body.Overrides)
	h.respond(w, r, http.StatusCreated, toAnalysis(analysis), err)
}

func (h handler) decode(w http.ResponseWriter, r *http.Request, dst any) bool {
	if err := httpjson.Decode(w, r, dst); err != nil {
		h.error(w, r, err)
		return false
	}
	return true
}

func (h handler) respond(w http.ResponseWriter, r *http.Request, status int, body any, err error) {
	if err != nil {
		h.error(w, r, err)
		return
	}
	httpjson.Write(w, status, body)
}

func (h handler) error(w http.ResponseWriter, r *http.Request, err error) {
	httpjson.Fail(w, r, h.logger, err, failures)
}
