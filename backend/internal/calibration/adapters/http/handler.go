// Package calibrationhttp exposes environment calibrations, overrides per
// animal of a test (test subject) and the calibration status that gates
// analysis over HTTP.
package calibrationhttp

import (
	access "github.com/HappyHackingSpace/Misko/backend/internal/access/domain"
	"github.com/HappyHackingSpace/Misko/backend/internal/calibration/application"
	"github.com/HappyHackingSpace/Misko/backend/internal/calibration/domain"
	"github.com/HappyHackingSpace/Misko/backend/internal/platform/httpjson"
	"log/slog"
	"net/http"
	"strconv"
	"time"
)

var failures = []httpjson.Failure{
	{Err: access.ErrUnauthenticated, Status: http.StatusUnauthorized, Code: "auth.unauthenticated"},
	{Err: access.ErrForbidden, Status: http.StatusForbidden, Code: "auth.forbidden"},
	{Err: application.ErrTestNotFound, Status: http.StatusNotFound, Code: "test.notFound"},
	{Err: application.ErrTestSubjectNotFound, Status: http.StatusNotFound, Code: "test.subjectNotFound"},
	{Err: application.ErrRevisionNotFound, Status: http.StatusNotFound, Code: "environment.revisionNotFound"},
	{Err: application.ErrNoVideo, Status: http.StatusConflict, Code: "calibration.noVideo"},
	{Err: application.ErrVideoNotVerified, Status: http.StatusConflict, Code: "calibration.videoNotVerified"},
	{Err: application.ErrInvalidScope, Status: http.StatusConflict, Code: "calibration.invalidScope"},
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
	{Err: domain.ErrUnknownZone, Status: http.StatusBadRequest, Code: "calibration.unknownZone"},
	{Err: domain.ErrMissingZone, Status: http.StatusBadRequest, Code: "calibration.missingZone"},
	{Err: domain.ErrInvalidZone, Status: http.StatusBadRequest, Code: "calibration.invalidZone"},
	{Err: domain.ErrZoneOutsideFrame, Status: http.StatusBadRequest, Code: "calibration.zoneOutsideFrame"},
	{Err: domain.ErrZonesFrozen, Status: http.StatusConflict, Code: "calibration.zonesFrozen"},
}

type handler struct {
	service      *application.Service
	authenticate func(*http.Request) (access.Actor, error)
	logger       *slog.Logger
}

func Register(mux *http.ServeMux, service *application.Service, authenticate func(*http.Request) (access.Actor, error), logger *slog.Logger) {
	h := handler{service: service, authenticate: authenticate, logger: logger}
	mux.HandleFunc("GET /api/tests/{id}/calibration-status", h.with(h.testStatus))
	mux.HandleFunc("GET /api/tests/{id}/subjects/{testSubjectId}/calibrations", h.with(h.list))
	mux.HandleFunc("POST /api/tests/{id}/subjects/{testSubjectId}/calibrations", h.with(h.create))
	mux.HandleFunc("GET /api/tests/{id}/subjects/{testSubjectId}/calibration-status", h.with(h.status))
	mux.HandleFunc("GET /api/environments/{id}/revisions/{number}/calibrations", h.with(h.listEnvironment))
	mux.HandleFunc("POST /api/environments/{id}/revisions/{number}/calibrations", h.with(h.createEnvironment))
	mux.HandleFunc("GET /api/environments/{id}/revisions/{number}/calibration-status", h.with(h.environmentStatus))
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
	in, ok := h.decodeInput(w, r)
	if !ok {
		return
	}
	c, err := h.service.Calibrate(r.Context(), actor, r.PathValue("id"), r.PathValue("testSubjectId"), in)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpjson.Write(w, http.StatusCreated, calibrationJSON(c))
}

func (h handler) createEnvironment(w http.ResponseWriter, r *http.Request, actor access.Actor) {
	number, ok := revisionNumber(r)
	if !ok {
		h.fail(w, r, application.ErrRevisionNotFound)
		return
	}
	in, ok := h.decodeInput(w, r)
	if !ok {
		return
	}
	c, err := h.service.CalibrateEnvironment(r.Context(), actor, r.PathValue("id"), number, in)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpjson.Write(w, http.StatusCreated, calibrationJSON(c))
}

// decodeInput reads a calibration request. referenceFrameUs is ignored for an
// environment, which has no video.
func (h handler) decodeInput(w http.ResponseWriter, r *http.Request) (domain.Input, bool) {
	var body struct {
		CameraID         string          `json:"cameraId"`
		FrameWidth       int             `json:"frameWidth"`
		FrameHeight      int             `json:"frameHeight"`
		Crop             cropJSON        `json:"crop"`
		ReferenceFrameUs int64           `json:"referenceFrameUs"`
		MeasurementPlane string          `json:"measurementPlane"`
		FitPoints        []pointJSON     `json:"fitPoints"`
		CheckPoints      []pointJSON     `json:"checkPoints"`
		SupersedesID     string          `json:"supersedesId"`
		Zones            []zoneInputJSON `json:"zones"`
	}
	if err := httpjson.Decode(w, r, &body); err != nil {
		h.fail(w, r, err)
		return domain.Input{}, false
	}
	return domain.Input{
		CameraID: body.CameraID, FrameWidth: body.FrameWidth, FrameHeight: body.FrameHeight, Crop: domain.Rect(body.Crop),
		ReferenceFrameUs: body.ReferenceFrameUs, Plane: domain.Plane(body.MeasurementPlane),
		Fit: correspondences(body.FitPoints), Check: correspondences(body.CheckPoints), SupersedesID: body.SupersedesID,
		Zones: mapZones(body.Zones, func(z zoneInputJSON) domain.ZoneInput { return domain.ZoneInput{Key: z.Key, Shape: z.toShape()} }),
	}, true
}

type vertexJSON struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

type circleJSON struct {
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Radius float64 `json:"radius"`
}

// shapeJSON is a circle or a polygon; exactly one is set.
type shapeJSON struct {
	Circle  *circleJSON  `json:"circle,omitempty"`
	Polygon []vertexJSON `json:"polygon,omitempty"`
}

func (s shapeJSON) toShape() domain.Shape {
	out := domain.Shape{}
	if s.Circle != nil {
		c := domain.Circle(*s.Circle)
		out.Circle = &c
	}
	for _, v := range s.Polygon {
		out.Polygon = append(out.Polygon, domain.Vertex(v))
	}
	return out
}

func toShapeJSON(s domain.Shape) shapeJSON {
	out := shapeJSON{}
	if s.Circle != nil {
		c := circleJSON(*s.Circle)
		out.Circle = &c
	}
	for _, v := range s.Polygon {
		out.Polygon = append(out.Polygon, vertexJSON(v))
	}
	return out
}

// zoneInputJSON is one calibrated zone shape as drawn, in full-frame pixels.
type zoneInputJSON struct {
	Key string `json:"key"`
	shapeJSON
}

// zoneJSON is one calibrated zone shape as drawn in pixels and converted to
// centimetres.
type zoneJSON struct {
	Key         string    `json:"key"`
	Pixels      shapeJSON `json:"pixels"`
	Centimeters shapeJSON `json:"centimeters"`
}

func mapZones[T, U any](items []T, convert func(T) U) []U {
	out := make([]U, 0, len(items))
	for _, item := range items {
		out = append(out, convert(item))
	}
	return out
}

func revisionNumber(r *http.Request) (int, bool) {
	number, err := strconv.Atoi(r.PathValue("number"))
	return number, err == nil
}

func (h handler) list(w http.ResponseWriter, r *http.Request, actor access.Actor) {
	list, err := h.service.Overrides(r.Context(), actor, r.PathValue("id"), r.PathValue("testSubjectId"))
	h.writeList(w, r, list, err)
}

func (h handler) listEnvironment(w http.ResponseWriter, r *http.Request, actor access.Actor) {
	number, ok := revisionNumber(r)
	if !ok {
		h.fail(w, r, application.ErrRevisionNotFound)
		return
	}
	list, err := h.service.EnvironmentCalibrations(r.Context(), actor, r.PathValue("id"), number)
	h.writeList(w, r, list, err)
}

func (h handler) writeList(w http.ResponseWriter, r *http.Request, list []domain.Calibration, err error) {
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

func (h handler) testStatus(w http.ResponseWriter, r *http.Request, actor access.Actor) {
	list, err := h.service.TestStatus(r.Context(), actor, r.PathValue("id"))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	data := make([]subjectStatusJSON, 0, len(list))
	for _, s := range list {
		data = append(data, toSubjectStatus(s))
	}
	httpjson.Write(w, http.StatusOK, struct {
		Data []subjectStatusJSON `json:"data"`
	}{data})
}

func (h handler) status(w http.ResponseWriter, r *http.Request, actor access.Actor) {
	s, err := h.service.SubjectStatus(r.Context(), actor, r.PathValue("id"), r.PathValue("testSubjectId"))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpjson.Write(w, http.StatusOK, toSubjectStatus(s))
}

func (h handler) environmentStatus(w http.ResponseWriter, r *http.Request, actor access.Actor) {
	number, ok := revisionNumber(r)
	if !ok {
		h.fail(w, r, application.ErrRevisionNotFound)
		return
	}
	view, err := h.service.EnvironmentStatus(r.Context(), actor, r.PathValue("id"), number)
	h.writeStatus(w, r, view, err)
}

func (h handler) writeStatus(w http.ResponseWriter, r *http.Request, view application.StatusView, err error) {
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpjson.Write(w, http.StatusOK, toStatus(view))
}

type statusJSON struct {
	Status      string               `json:"status"`
	Source      *string              `json:"source"`
	Drift       *string              `json:"drift"`
	Calibration *calibrationJSONBody `json:"calibration"`
}

func toStatus(view application.StatusView) statusJSON {
	out := statusJSON{Status: string(view.Status), Source: nullable(string(view.Source)), Drift: nullable(string(view.Drift))}
	if view.Current != nil {
		c := calibrationJSON(*view.Current)
		out.Calibration = &c
	}
	return out
}

type subjectStatusJSON struct {
	TestSubjectID         string  `json:"testSubjectId"`
	EnvironmentRevisionID string  `json:"environmentRevisionId"`
	RecordingID           *string `json:"recordingId"`
	statusJSON
}

func toSubjectStatus(s application.SubjectStatus) subjectStatusJSON {
	return subjectStatusJSON{s.TestSubjectID, s.EnvironmentRevisionID, nullable(s.RecordingID), toStatus(s.StatusView)}
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
	ID                    string      `json:"id"`
	Scope                 string      `json:"scope"`
	TestSubjectID         *string     `json:"testSubjectId"`
	RecordingID           *string     `json:"recordingId"`
	EnvironmentRevisionID string      `json:"environmentRevisionId"`
	SupersedesID          *string     `json:"supersedesId"`
	CameraID              string      `json:"cameraId"`
	FrameWidth            int         `json:"frameWidth"`
	FrameHeight           int         `json:"frameHeight"`
	Crop                  cropJSON    `json:"crop"`
	ReferenceFrameUs      *int64      `json:"referenceFrameUs"`
	MeasurementPlane      string      `json:"measurementPlane"`
	FitPoints             []pointJSON `json:"fitPoints"`
	CheckPoints           []pointJSON `json:"checkPoints"`
	Transform             [9]float64  `json:"transform"`
	Zones                 []zoneJSON  `json:"zones"`
	FitRMSErrorCm         float64     `json:"fitRmsErrorCm"`
	CheckRMSErrorCm       float64     `json:"checkRmsErrorCm"`
	CheckMaxErrorCm       float64     `json:"checkMaxErrorCm"`
	ToleranceCm           float64     `json:"toleranceCm"`
	AlgorithmVersion      string      `json:"algorithmVersion"`
	Status                string      `json:"status"`
	RejectionReason       *string     `json:"rejectionReason"`
	CreatedBy             string      `json:"createdBy"`
	CreatedAt             time.Time   `json:"createdAt"`
}

func calibrationJSON(c domain.Calibration) calibrationJSONBody {
	return calibrationJSONBody{
		ID: c.ID, Scope: string(c.Scope()), TestSubjectID: nullable(c.TestSubjectID), RecordingID: nullable(c.RecordingID), EnvironmentRevisionID: c.EnvironmentRevisionID, SupersedesID: nullable(c.SupersedesID), CameraID: c.CameraID,
		FrameWidth: c.FrameWidth, FrameHeight: c.FrameHeight, Crop: cropJSON(c.Crop), ReferenceFrameUs: referenceFrame(c),
		MeasurementPlane: string(c.Plane), FitPoints: pointsJSON(c.Fit), CheckPoints: pointsJSON(c.Check), Transform: c.Transform,
		Zones: mapZones(c.Zones, func(z domain.Zone) zoneJSON {
			return zoneJSON{Key: z.Key, Pixels: toShapeJSON(z.Pixels), Centimeters: toShapeJSON(z.Centimeters)}
		}),
		FitRMSErrorCm: c.FitRMSErrorCm, CheckRMSErrorCm: c.CheckRMSErrorCm, CheckMaxErrorCm: c.CheckMaxErrorCm, ToleranceCm: c.ToleranceCm,
		AlgorithmVersion: c.AlgorithmVersion, Status: string(c.Status), RejectionReason: nullable(c.RejectionReason),
		CreatedBy: c.CreatedBy, CreatedAt: c.CreatedAt,
	}
}

// referenceFrame is null for an environment calibration, which has no video.
func referenceFrame(c domain.Calibration) *int64 {
	if c.Scope() == domain.ScopeEnvironment {
		return nil
	}
	return &c.ReferenceFrameUs
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
