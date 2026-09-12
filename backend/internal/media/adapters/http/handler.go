// Package mediahttp exposes test recordings, uploads and read URLs over HTTP.
// Signed URLs appear only in responses to the caller and are never logged.
package mediahttp

import (
	access "github.com/HappyHackingSpace/Misko/backend/internal/access/domain"
	"github.com/HappyHackingSpace/Misko/backend/internal/media/application"
	"github.com/HappyHackingSpace/Misko/backend/internal/media/domain"
	"github.com/HappyHackingSpace/Misko/backend/internal/platform/httpjson"
	"log/slog"
	"net/http"
	"strconv"
	"time"
)

var failures = []httpjson.Failure{
	{Err: access.ErrUnauthenticated, Status: http.StatusUnauthorized, Code: "auth.unauthenticated"},
	{Err: access.ErrForbidden, Status: http.StatusForbidden, Code: "auth.forbidden"},
	{Err: application.ErrStorageNotConfigured, Status: http.StatusServiceUnavailable, Code: "media.storageNotConfigured"},
	{Err: application.ErrTestNotFound, Status: http.StatusNotFound, Code: "test.notFound"},
	{Err: application.ErrTestCancelled, Status: http.StatusConflict, Code: "test.cancelled"},
	{Err: application.ErrRecordingNotFound, Status: http.StatusNotFound, Code: "recording.notFound"},
	{Err: application.ErrNotUploader, Status: http.StatusForbidden, Code: "media.notUploader"},
	{Err: application.ErrUploadIncomplete, Status: http.StatusConflict, Code: "media.uploadIncomplete"},
	{Err: application.ErrNotVerified, Status: http.StatusConflict, Code: "media.notVerified"},
	{Err: domain.ErrInvalidContentType, Status: http.StatusBadRequest, Code: "media.invalidContentType"},
	{Err: domain.ErrInvalidSize, Status: http.StatusBadRequest, Code: "media.invalidSize"},
	{Err: domain.ErrInvalidChecksum, Status: http.StatusBadRequest, Code: "media.invalidChecksum"},
	{Err: domain.ErrInvalidFileName, Status: http.StatusBadRequest, Code: "media.invalidFileName"},
	{Err: domain.ErrInvalidClip, Status: http.StatusBadRequest, Code: "media.invalidClip"},
	{Err: domain.ErrAlreadyFinalized, Status: http.StatusConflict, Code: "media.alreadyFinalized"},
}

type handler struct {
	service      *application.Service
	authenticate func(*http.Request) (access.Actor, error)
	logger       *slog.Logger
}

func Register(mux *http.ServeMux, service *application.Service, authenticate func(*http.Request) (access.Actor, error), logger *slog.Logger) {
	h := handler{service: service, authenticate: authenticate, logger: logger}
	mux.HandleFunc("GET /api/tests/{id}/recordings", h.with(h.list))
	mux.HandleFunc("POST /api/tests/{id}/recordings", h.with(h.start))
	mux.HandleFunc("POST /api/tests/{id}/recordings/{recordingId}/finalize", h.with(h.finalize))
	mux.HandleFunc("GET /api/tests/{id}/recordings/{recordingId}/read-url", h.with(h.readURL))
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

func (h handler) list(w http.ResponseWriter, r *http.Request, actor access.Actor) {
	recordings, err := h.service.Recordings(r.Context(), actor, r.PathValue("id"))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	data := make([]recordingJSON, 0, len(recordings))
	for _, rec := range recordings {
		data = append(data, toRecording(rec))
	}
	httpjson.Write(w, http.StatusOK, struct {
		Data []recordingJSON `json:"data"`
	}{data})
}

func (h handler) start(w http.ResponseWriter, r *http.Request, actor access.Actor) {
	var body struct {
		FileName    string `json:"fileName"`
		ContentType string `json:"contentType"`
		SizeBytes   int64  `json:"sizeBytes"`
		CRC32C      string `json:"crc32c"`
		ClipStartUs int64  `json:"clipStartUs"`
		ClipEndUs   *int64 `json:"clipEndUs"`
	}
	if err := httpjson.Decode(w, r, &body); err != nil {
		h.fail(w, r, err)
		return
	}
	upload, err := h.service.StartUpload(r.Context(), actor, r.PathValue("id"), domain.UploadRequest(body))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpjson.Write(w, http.StatusCreated, struct {
		Recording recordingJSON `json:"recording"`
		Upload    uploadJSON    `json:"upload"`
	}{toRecording(upload.Recording), uploadJSON{upload.Request.Method, upload.Request.URL, upload.Request.Headers, upload.Request.ExpiresAt}})
}

func (h handler) finalize(w http.ResponseWriter, r *http.Request, actor access.Actor) {
	var body struct{}
	if err := httpjson.Decode(w, r, &body); err != nil {
		h.fail(w, r, err)
		return
	}
	rec, err := h.service.FinalizeUpload(r.Context(), actor, r.PathValue("id"), r.PathValue("recordingId"))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpjson.Write(w, http.StatusOK, toRecording(rec))
}

func (h handler) readURL(w http.ResponseWriter, r *http.Request, actor access.Actor) {
	link, err := h.service.ReadURL(r.Context(), actor, r.PathValue("id"), r.PathValue("recordingId"))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpjson.Write(w, http.StatusOK, struct {
		URL       string    `json:"url"`
		ExpiresAt time.Time `json:"expiresAt"`
	}{link.URL, link.ExpiresAt})
}

func (h handler) fail(w http.ResponseWriter, r *http.Request, err error) {
	httpjson.Fail(w, r, h.logger, err, failures)
}

type uploadJSON struct {
	Method    string            `json:"method"`
	URL       string            `json:"url"`
	Headers   map[string]string `json:"headers"`
	ExpiresAt time.Time         `json:"expiresAt"`
}

type videoJSON struct {
	ID              string     `json:"id"`
	Kind            string     `json:"kind"`
	ContentType     string     `json:"contentType"`
	FileName        *string    `json:"fileName"`
	SizeBytes       int64      `json:"sizeBytes"`
	CRC32C          string     `json:"crc32c"`
	Status          string     `json:"status"`
	Generation      *string    `json:"generation"`
	RejectionReason *string    `json:"rejectionReason"`
	VerifiedAt      *time.Time `json:"verifiedAt"`
	CreatedBy       string     `json:"createdBy"`
	CreatedAt       time.Time  `json:"createdAt"`
}

type recordingJSON struct {
	ID           string    `json:"id"`
	ExperimentID string    `json:"experimentId"`
	TestID       string    `json:"testId"`
	ClipStartUs  int64     `json:"clipStartUs"`
	ClipEndUs    *int64    `json:"clipEndUs"`
	Video        videoJSON `json:"video"`
	CreatedBy    string    `json:"createdBy"`
	CreatedAt    time.Time `json:"createdAt"`
}

func toRecording(rec domain.Recording) recordingJSON {
	a := rec.Asset
	video := videoJSON{
		ID: a.ID, Kind: string(a.Kind), ContentType: a.ContentType, FileName: nullable(a.FileName), SizeBytes: a.SizeBytes,
		CRC32C: domain.FormatCRC32C(a.CRC32C), Status: string(a.Status), RejectionReason: nullable(a.RejectionReason),
		VerifiedAt: a.VerifiedAt, CreatedBy: a.CreatedBy, CreatedAt: a.CreatedAt,
	}
	if a.Generation > 0 {
		// Generations exceed the integers JavaScript represents exactly, so they are strings.
		video.Generation = nullable(strconv.FormatInt(a.Generation, 10))
	}
	return recordingJSON{rec.ID, rec.ExperimentID, rec.TestID, rec.Clip.StartUs, rec.Clip.EndUs, video, rec.CreatedBy, rec.CreatedAt}
}

func nullable(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
