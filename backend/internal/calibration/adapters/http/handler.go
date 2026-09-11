// Package calibrationhttp exposes per-video calibrations and the calibration
// status that gates analysis over HTTP.
package calibrationhttp

import (
	access "github.com/HappyHackingSpace/Misko/backend/internal/access/domain"
	"github.com/HappyHackingSpace/Misko/backend/internal/calibration/application"
	"github.com/HappyHackingSpace/Misko/backend/internal/calibration/domain"
	"github.com/HappyHackingSpace/Misko/backend/internal/platform/httpjson"
	"log/slog"
	"net/http"
	"time"
)

var failures = []httpjson.Failure{
	{Err: access.ErrUnauthenticated, Status: http.StatusUnauthorized, Code: "auth.unauthenticated"},
	{Err: access.ErrForbidden, Status: http.StatusForbidden, Code: "auth.forbidden"},
	{Err: application.ErrRecordingNotFound, Status: http.StatusNotFound, Code: "recording.notFound"},
	{Err: application.ErrVideoNotVerified, Status: http.StatusConflict, Code: "calibration.videoNotVerified"},
	{Err: domain.ErrNotRequired, Status: http.StatusConflict, Code: "calibration.notRequired"},
	{Err: domain.ErrStaleCorrection, Status: http.StatusConflict, Code: "calibration.staleCorrection"},
	{Err: domain.ErrCameraMismatch, Status: http.StatusBadRequest, Code: "calibration.cameraMismatch"},
	{Err: domain.ErrInvalidCamera, Status: http.StatusBadRequest, Code: "calibration.invalidCamera"},
	{Err: domain.ErrInvalidFrame, Status: http.StatusBadRequest, Code: "calibration.invalidFrame"},
	{Err: domain.ErrInvalidCrop, Status: http.StatusBadRequest, Code: "calibration.invalidCrop"},
	{Err: domain.ErrInvalidPlane, Status: http.StatusBadRequest, Code: "calibration.invalidPlane"},
	{Err: domain.ErrInvalidReferenceFrame, Status: http.StatusBadRequest, Code: "calibration.invalidReferenceFrame"},
	{Err: domain.ErrInsufficientFitPoints, Status: http.StatusBadRequest, Code: "calibration.insufficientFitPoints"},
	{Err: domain.ErrInsufficientCheckPoints, Status: http.StatusBadRequest, Code: "calibration.insufficientCheckPoints"},
	{Err: domain.ErrInvalidPoint, Status: http.StatusBadRequest, Code: "calibration.invalidPoint"},
	{Err: domain.ErrPointOutsideCrop, Status: http.StatusBadRequest, Code: "calibration.pointOutsideCrop"},
	{Err: domain.ErrPointOutsideEnvironment, Status: http.StatusBadRequest, Code: "calibration.pointOutsideEnvironment"},
	{Err: domain.ErrCheckPointReused, Status: http.StatusBadRequest, Code: "calibration.checkPointReused"},
	{Err: domain.ErrDegenerateFit, Status: http.StatusBadRequest, Code: "calibration.degenerateFit"},
}

type handler struct {
	service      *application.Service
	authenticate func(*http.Request) (access.Actor, error)
	logger       *slog.Logger
}

func Register(mux *http.ServeMux, service *application.Service, authenticate func(*http.Request) (access.Actor, error), logger *slog.Logger) {
	h := handler{service: service, authenticate: authenticate, logger: logger}
	mux.HandleFunc("GET /api/tests/{id}/recordings/{recordingId}/calibrations", h.with(h.list))
	mux.HandleFunc("POST /api/tests/{id}/recordings/{recordingId}/calibrations", h.with(h.create))
	mux.HandleFunc("GET /api/tests/{id}/recordings/{recordingId}/calibration-status", h.with(h.status))
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

type pointJSON struct {
	PixelX float64 `json:"pixelX"`
	PixelY float64 `json:"pixelY"`
	WorldX float64 `json:"worldX"`
	WorldY float64 `json:"worldY"`
}

type cropJSON struct {
	X      int `json:"x"`
	Y      int `json:"y"`
	Width  int `json:"width"`
	Height int `json:"height"`
}

func (h handler) create(w http.ResponseWriter, r *http.Request, actor access.Actor) {
	var body struct {
		CameraID         string      `json:"cameraId"`
		FrameWidth       int         `json:"frameWidth"`
		FrameHeight      int         `json:"frameHeight"`
		Crop             cropJSON    `json:"crop"`
		ReferenceFrameUs int64       `json:"referenceFrameUs"`
		MeasurementPlane string      `json:"measurementPlane"`
		FitPoints        []pointJSON `json:"fitPoints"`
		CheckPoints      []pointJSON `json:"checkPoints"`
		SupersedesID     string      `json:"supersedesId"`
	}
	if err := httpjson.Decode(w, r, &body); err != nil {
		h.fail(w, r, err)
		return
	}
	c, err := h.service.Calibrate(r.Context(), actor, r.PathValue("id"), r.PathValue("recordingId"), domain.Input{
		CameraID: body.CameraID, FrameWidth: body.FrameWidth, FrameHeight: body.FrameHeight, Crop: domain.Rect(body.Crop),
		ReferenceFrameUs: body.ReferenceFrameUs, Plane: domain.Plane(body.MeasurementPlane),
		Fit: correspondences(body.FitPoints), Check: correspondences(body.CheckPoints), SupersedesID: body.SupersedesID,
	})
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpjson.Write(w, http.StatusCreated, calibrationJSON(c))
}

func (h handler) list(w http.ResponseWriter, r *http.Request, actor access.Actor) {
	list, err := h.service.Calibrations(r.Context(), actor, r.PathValue("id"), r.PathValue("recordingId"))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	data := make([]calibrationJSONBody, 0, len(list))
	for _, c := range list {
		data = append(data, calibrationJSON(c))
	}
	httpjson.Write(w, http.StatusOK, struct {
		Data []calibrationJSONBody `json:"data"`
	}{data})
}

func (h handler) status(w http.ResponseWriter, r *http.Request, actor access.Actor) {
	view, err := h.service.Status(r.Context(), actor, r.PathValue("id"), r.PathValue("recordingId"))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	out := struct {
		Status      string               `json:"status"`
		Calibration *calibrationJSONBody `json:"calibration"`
	}{Status: string(view.Status)}
	if view.Current != nil {
		c := calibrationJSON(*view.Current)
		out.Calibration = &c
	}
	httpjson.Write(w, http.StatusOK, out)
}

func (h handler) fail(w http.ResponseWriter, r *http.Request, err error) {
	httpjson.Fail(w, r, h.logger, err, failures)
}

func correspondences(in []pointJSON) []domain.Correspondence {
	out := make([]domain.Correspondence, 0, len(in))
	for _, p := range in {
		out = append(out, domain.Correspondence(p))
	}
	return out
}

type calibrationJSONBody struct {
	ID               string      `json:"id"`
	RecordingID      string      `json:"recordingId"`
	SupersedesID     *string     `json:"supersedesId"`
	CameraID         string      `json:"cameraId"`
	FrameWidth       int         `json:"frameWidth"`
	FrameHeight      int         `json:"frameHeight"`
	Crop             cropJSON    `json:"crop"`
	ReferenceFrameUs int64       `json:"referenceFrameUs"`
	MeasurementPlane string      `json:"measurementPlane"`
	FitPoints        []pointJSON `json:"fitPoints"`
	CheckPoints      []pointJSON `json:"checkPoints"`
	Transform        [9]float64  `json:"transform"`
	FitRMSErrorCm    float64     `json:"fitRmsErrorCm"`
	CheckRMSErrorCm  float64     `json:"checkRmsErrorCm"`
	CheckMaxErrorCm  float64     `json:"checkMaxErrorCm"`
	ToleranceCm      float64     `json:"toleranceCm"`
	AlgorithmVersion string      `json:"algorithmVersion"`
	Status           string      `json:"status"`
	RejectionReason  *string     `json:"rejectionReason"`
	CreatedBy        string      `json:"createdBy"`
	CreatedAt        time.Time   `json:"createdAt"`
}

func calibrationJSON(c domain.Calibration) calibrationJSONBody {
	return calibrationJSONBody{
		ID: c.ID, RecordingID: c.RecordingID, SupersedesID: nullable(c.SupersedesID), CameraID: c.CameraID,
		FrameWidth: c.FrameWidth, FrameHeight: c.FrameHeight, Crop: cropJSON(c.Crop), ReferenceFrameUs: c.ReferenceFrameUs,
		MeasurementPlane: string(c.Plane), FitPoints: pointsJSON(c.Fit), CheckPoints: pointsJSON(c.Check), Transform: c.Transform,
		FitRMSErrorCm: c.FitRMSErrorCm, CheckRMSErrorCm: c.CheckRMSErrorCm, CheckMaxErrorCm: c.CheckMaxErrorCm, ToleranceCm: c.ToleranceCm,
		AlgorithmVersion: c.AlgorithmVersion, Status: string(c.Status), RejectionReason: nullable(c.RejectionReason),
		CreatedBy: c.CreatedBy, CreatedAt: c.CreatedAt,
	}
}

func pointsJSON(in []domain.Correspondence) []pointJSON {
	out := make([]pointJSON, 0, len(in))
	for _, p := range in {
		out = append(out, pointJSON(p))
	}
	return out
}

func nullable(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
