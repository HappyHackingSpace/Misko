// Package analysishttp exposes analysis runs and the review of each animal's
// result to users, and the job protocol to analysis workers. Workers authenticate with "Authorization: Worker <token>";
// user tokens are not accepted on worker routes and worker tokens are not
// accepted on user routes.
package analysishttp

import (
	access "github.com/HappyHackingSpace/Misko/backend/internal/access/domain"
	"github.com/HappyHackingSpace/Misko/backend/internal/analysis/application"
	"github.com/HappyHackingSpace/Misko/backend/internal/analysis/domain"
	media "github.com/HappyHackingSpace/Misko/backend/internal/media/domain"
	"github.com/HappyHackingSpace/Misko/backend/internal/platform/httpjson"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"
)

var failures = []httpjson.Failure{
	{Err: access.ErrUnauthenticated, Status: http.StatusUnauthorized, Code: "auth.unauthenticated"},
	{Err: access.ErrForbidden, Status: http.StatusForbidden, Code: "auth.forbidden"},
	{Err: application.ErrWorkerUnauthenticated, Status: http.StatusUnauthorized, Code: "worker.unauthenticated"},
	{Err: application.ErrWorkerNotFound, Status: http.StatusNotFound, Code: "worker.notFound"},
	{Err: application.ErrWorkerNameTaken, Status: http.StatusConflict, Code: "worker.nameTaken"},
	{Err: application.ErrUnknownCapability, Status: http.StatusBadRequest, Code: "worker.unknownCapability"},
	{Err: domain.ErrInvalidWorkerName, Status: http.StatusBadRequest, Code: "worker.invalidName"},
	{Err: domain.ErrInvalidCapability, Status: http.StatusBadRequest, Code: "worker.invalidCapability"},
	{Err: application.ErrStorageNotConfigured, Status: http.StatusServiceUnavailable, Code: "media.storageNotConfigured"},
	{Err: application.ErrRunNotFound, Status: http.StatusNotFound, Code: "analysis.runNotFound"},
	{Err: application.ErrTestSubjectNotFound, Status: http.StatusNotFound, Code: "analysis.testSubjectNotFound"},
	{Err: application.ErrResultNotFound, Status: http.StatusNotFound, Code: "analysis.resultNotFound"},
	{Err: application.ErrExperimentClosed, Status: http.StatusConflict, Code: "analysis.experimentClosed"},
	{Err: application.ErrInvalidQuery, Status: http.StatusBadRequest, Code: "analysis.invalidQuery"},
	{Err: application.ErrInvalidBulk, Status: http.StatusBadRequest, Code: "analysis.invalidBulk"},
	{Err: domain.ErrInvalidRejection, Status: http.StatusBadRequest, Code: "analysis.invalidRejection"},
	{Err: domain.ErrInvalidNote, Status: http.StatusBadRequest, Code: "analysis.invalidNote"},
	{Err: domain.ErrRunNotChosen, Status: http.StatusConflict, Code: "analysis.runNotChosen"},
	{Err: application.ErrNotReady, Status: http.StatusConflict, Code: "analysis.notReady"},
	{Err: application.ErrRunPending, Status: http.StatusConflict, Code: "analysis.runPending"},
	{Err: application.ErrRunNotSucceeded, Status: http.StatusConflict, Code: "analysis.runNotSucceeded"},
	{Err: application.ErrUnknownOutput, Status: http.StatusBadRequest, Code: "analysis.unknownOutput"},
	{Err: application.ErrOutputNotVerified, Status: http.StatusConflict, Code: "analysis.outputNotVerified"},
	{Err: application.ErrInvalidOutput, Status: http.StatusBadRequest, Code: "analysis.invalidOutput"},
	{Err: domain.ErrStaleAttempt, Status: http.StatusConflict, Code: "analysis.staleAttempt"},
	{Err: domain.ErrNotClaimable, Status: http.StatusConflict, Code: "analysis.notClaimable"},
	{Err: domain.ErrInvalidReason, Status: http.StatusBadRequest, Code: "analysis.invalidReason"},
	{Err: domain.ErrInvalidModelVersion, Status: http.StatusBadRequest, Code: "analysis.invalidModelVersion"},
	{Err: domain.ErrInvalidTrajectory, Status: http.StatusBadRequest, Code: "analysis.invalidTrajectory"},
	{Err: domain.ErrMissingTrajectory, Status: http.StatusBadRequest, Code: "analysis.missingTrajectory"},
	{Err: domain.ErrInvalidArtifact, Status: http.StatusBadRequest, Code: "analysis.invalidArtifact"},
	{Err: domain.ErrMissingAnalyzedVideo, Status: http.StatusBadRequest, Code: "analysis.missingAnalyzedVideo"},
	{Err: domain.ErrInvalidPair, Status: http.StatusBadRequest, Code: "analysis.invalidPair"},
	{Err: domain.ErrInvalidDuration, Status: http.StatusBadRequest, Code: "analysis.invalidDuration"},
}

type handler struct {
	service      *application.Service
	authenticate func(*http.Request) (access.Actor, error)
	logger       *slog.Logger
}

func Register(mux *http.ServeMux, service *application.Service, authenticate func(*http.Request) (access.Actor, error), logger *slog.Logger) {
	h := handler{service: service, authenticate: authenticate, logger: logger}
	mux.HandleFunc("POST /api/analysis/workers", h.user(h.registerWorker))
	mux.HandleFunc("GET /api/analysis/workers", h.user(h.workers))
	mux.HandleFunc("POST /api/analysis/workers/{id}/disable", h.user(h.disableWorker))
	mux.HandleFunc("GET /api/analysis/capabilities", h.user(h.capabilities))
	mux.HandleFunc("POST /api/analysis/test-subjects/{testSubjectId}/runs", h.user(h.reanalyze))
	mux.HandleFunc("GET /api/tests/{id}/analysis-runs", h.user(h.runs))
	mux.HandleFunc("GET /api/analysis/results", h.user(h.results))
	mux.HandleFunc("POST /api/analysis/results/bulk-accept", h.user(h.acceptMany))
	mux.HandleFunc("GET /api/analysis/results/{testSubjectId}", h.user(h.result))
	mux.HandleFunc("POST /api/analysis/results/{testSubjectId}/accept", h.user(h.accept))
	mux.HandleFunc("POST /api/analysis/results/{testSubjectId}/reject", h.user(h.reject))
	mux.HandleFunc("POST /api/analysis/results/{testSubjectId}/run", h.user(h.chooseRun))
	mux.HandleFunc("GET /api/analysis-runs/{runId}", h.user(h.run))
	mux.HandleFunc("GET /api/analysis-runs/{runId}/video-pair", h.user(h.videoPair))

	mux.HandleFunc("POST /api/worker/claim", h.worker(h.claim))
	mux.HandleFunc("POST /api/worker/runs/{runId}/heartbeat", h.worker(h.heartbeat))
	mux.HandleFunc("POST /api/worker/runs/{runId}/source-url", h.worker(h.sourceURL))
	mux.HandleFunc("POST /api/worker/runs/{runId}/outputs", h.worker(h.output))
	mux.HandleFunc("POST /api/worker/runs/{runId}/result", h.worker(h.submit))
	mux.HandleFunc("POST /api/worker/runs/{runId}/failure", h.worker(h.fail))
}

func (h handler) user(next func(http.ResponseWriter, *http.Request, access.Actor)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		actor, err := h.authenticate(r)
		if err != nil {
			h.error(w, r, err)
			return
		}
		next(w, r, actor)
	}
}

func (h handler) worker(next func(http.ResponseWriter, *http.Request, domain.Worker)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Worker ")
		if !ok {
			h.error(w, r, application.ErrWorkerUnauthenticated)
			return
		}
		worker, err := h.service.AuthenticateWorker(r.Context(), token)
		if err != nil {
			h.error(w, r, err)
			return
		}
		next(w, r, worker)
	}
}

type capabilityJSON struct {
	ParadigmKey     string `json:"paradigmKey"`
	ParadigmVersion int    `json:"paradigmVersion"`
}

func (h handler) registerWorker(w http.ResponseWriter, r *http.Request, actor access.Actor) {
	var body struct {
		Name         string           `json:"name"`
		ModelVersion string           `json:"modelVersion"`
		Capabilities []capabilityJSON `json:"capabilities"`
	}
	if !h.decode(w, r, &body) {
		return
	}
	registered, err := h.service.RegisterWorker(r.Context(), actor, application.WorkerInput{Name: body.Name, ModelVersion: body.ModelVersion, Capabilities: capabilities(body.Capabilities)})
	h.respond(w, r, http.StatusCreated, struct {
		Worker workerJSON `json:"worker"`
		Token  string     `json:"token"`
	}{toWorker(registered.Worker), registered.Token}, err)
}

func (h handler) workers(w http.ResponseWriter, r *http.Request, actor access.Actor) {
	list, err := h.service.Workers(r.Context(), actor)
	h.respond(w, r, http.StatusOK, dataJSON{mapSlice(list, toWorker)}, err)
}

func (h handler) disableWorker(w http.ResponseWriter, r *http.Request, actor access.Actor) {
	worker, err := h.service.DisableWorker(r.Context(), actor, r.PathValue("id"))
	h.respond(w, r, http.StatusOK, toWorker(worker), err)
}

func (h handler) capabilities(w http.ResponseWriter, r *http.Request, actor access.Actor) {
	list, err := h.service.Capabilities(r.Context(), actor)
	h.respond(w, r, http.StatusOK, dataJSON{mapSlice(list, func(c domain.Capability) capabilityJSON { return capabilityJSON(c) })}, err)
}

func (h handler) reanalyze(w http.ResponseWriter, r *http.Request, actor access.Actor) {
	var body struct{}
	if !h.decode(w, r, &body) {
		return
	}
	run, err := h.service.Reanalyze(r.Context(), actor, r.PathValue("testSubjectId"))
	h.respond(w, r, http.StatusCreated, toRun(run), err)
}

func (h handler) runs(w http.ResponseWriter, r *http.Request, actor access.Actor) {
	list, err := h.service.Runs(r.Context(), actor, r.PathValue("id"), r.URL.Query().Get("testSubjectId"))
	h.respond(w, r, http.StatusOK, dataJSON{mapSlice(list, toRun)}, err)
}

func (h handler) results(w http.ResponseWriter, r *http.Request, actor access.Actor) {
	v := r.URL.Query()
	page, err := h.service.Results(r.Context(), actor, application.ReviewQuery{
		ExperimentID: v.Get("experimentId"), Status: v.Get("status"), Page: httpjson.QueryInt(v.Get("page")), PageSize: httpjson.QueryInt(v.Get("pageSize")),
	})
	h.respond(w, r, http.StatusOK, struct {
		Data     []reviewJSON `json:"data"`
		Total    int          `json:"total"`
		Page     int          `json:"page"`
		PageSize int          `json:"pageSize"`
	}{mapSlice(page.Items, toReview), page.Total, page.Page, page.PageSize}, err)
}

func (h handler) result(w http.ResponseWriter, r *http.Request, actor access.Actor) {
	detail, err := h.service.Result(r.Context(), actor, r.PathValue("testSubjectId"))
	h.respond(w, r, http.StatusOK, struct {
		reviewJSON
		Runs []runJSON `json:"runs"`
	}{toReview(detail.Item), mapSlice(detail.Runs, toRun)}, err)
}

func (h handler) accept(w http.ResponseWriter, r *http.Request, actor access.Actor) {
	var body struct {
		Note string `json:"note"`
	}
	if !h.decode(w, r, &body) {
		return
	}
	item, err := h.service.Accept(r.Context(), actor, r.PathValue("testSubjectId"), body.Note)
	h.respond(w, r, http.StatusOK, toReview(item), err)
}

func (h handler) reject(w http.ResponseWriter, r *http.Request, actor access.Actor) {
	var body struct {
		Reason string `json:"reason"`
		Note   string `json:"note"`
	}
	if !h.decode(w, r, &body) {
		return
	}
	item, err := h.service.Reject(r.Context(), actor, r.PathValue("testSubjectId"), body.Reason, body.Note)
	h.respond(w, r, http.StatusOK, toReview(item), err)
}

func (h handler) chooseRun(w http.ResponseWriter, r *http.Request, actor access.Actor) {
	var body struct {
		RunID string `json:"runId"`
	}
	if !h.decode(w, r, &body) {
		return
	}
	item, err := h.service.ChooseRun(r.Context(), actor, r.PathValue("testSubjectId"), body.RunID)
	h.respond(w, r, http.StatusOK, toReview(item), err)
}

func (h handler) acceptMany(w http.ResponseWriter, r *http.Request, actor access.Actor) {
	var body struct {
		TestSubjectIDs []string `json:"testSubjectIds"`
		Note           string   `json:"note"`
	}
	if !h.decode(w, r, &body) {
		return
	}
	items, err := h.service.AcceptMany(r.Context(), actor, body.TestSubjectIDs, body.Note)
	h.respond(w, r, http.StatusOK, dataJSON{mapSlice(items, toReview)}, err)
}

func (h handler) run(w http.ResponseWriter, r *http.Request, actor access.Actor) {
	detail, err := h.service.RunDetail(r.Context(), actor, r.PathValue("runId"))
	if err != nil {
		h.error(w, r, err)
		return
	}
	body := struct {
		runJSON
		Result *resultJSON `json:"result"`
	}{runJSON: toRun(detail.Run)}
	if detail.Published != nil {
		body.Result = toResult(*detail.Published)
	}
	httpjson.Write(w, http.StatusOK, body)
}

func (h handler) videoPair(w http.ResponseWriter, r *http.Request, actor access.Actor) {
	links, err := h.service.VideoPair(r.Context(), actor, r.PathValue("runId"))
	h.respond(w, r, http.StatusOK, struct {
		OriginalURL        string    `json:"originalUrl"`
		AnalyzedURL        string    `json:"analyzedUrl"`
		ExpiresAt          time.Time `json:"expiresAt"`
		SourceVideoID      string    `json:"sourceVideoId"`
		SourceGeneration   string    `json:"sourceGeneration"`
		AnalyzedVideoID    string    `json:"analyzedVideoId"`
		AnalyzedGeneration string    `json:"analyzedGeneration"`
		SourceOffsetUs     int64     `json:"sourceOffsetUs"`
		OutputOffsetUs     int64     `json:"outputOffsetUs"`
		TimeMappingVersion string    `json:"timeMappingVersion"`
	}{links.OriginalURL, links.AnalyzedURL, links.ExpiresAt, links.Pair.SourceAssetID, itoa(links.Pair.SourceGeneration), links.Pair.AnalyzedAssetID,
		itoa(links.Pair.AnalyzedGeneration), links.Pair.SourceOffsetUs, links.Pair.OutputOffsetUs, links.Pair.TimeMappingVersion}, err)
}

type attemptBody struct {
	Attempt int `json:"attempt"`
}

func (h handler) claim(w http.ResponseWriter, r *http.Request, worker domain.Worker) {
	var body struct{}
	if !h.decode(w, r, &body) {
		return
	}
	job, err := h.service.Claim(r.Context(), worker)
	if err == application.ErrNoRun {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	contract := contractJSON{RequiredZones: append([]string{}, job.Contract.RequiredZones...), MissingReasons: job.Contract.MissingReasons, Metrics: []metricSpecJSON{}, Events: []eventSpecJSON{}, QC: []qcJSON{}}
	for _, m := range job.Contract.Metrics {
		contract.Metrics = append(contract.Metrics, metricSpecJSON{m.Key, m.Unit, m.Integer, m.Min, m.Max})
	}
	for _, e := range job.Contract.Events {
		contract.Events = append(contract.Events, eventSpecJSON{e.Type, e.Kind})
	}
	for _, q := range job.Contract.QC {
		contract.QC = append(contract.QC, qcJSON(q))
	}
	var calibration *calibrationJSON
	if c := job.Calibration; c != nil {
		calibration = &calibrationJSON{ID: c.ID, FrameWidth: c.FrameWidth, FrameHeight: c.FrameHeight,
			Crop: cropJSON{c.CropX, c.CropY, c.CropWidth, c.CropHeight}, MeasurementPlane: c.Plane, Transform: c.Transform,
			Zones: mapSlice(c.Zones, func(z domain.Zone) zoneJSON {
				return zoneJSON{Key: z.Key, Pixels: toShapeJSON(z.Pixels), Centimeters: toShapeJSON(z.Centimeters)}
			})}
	}
	h.respond(w, r, http.StatusOK, struct {
		Run              runJSON          `json:"run"`
		TestSubject      testSubjectJSON  `json:"testSubject"`
		OutputPrefix     string           `json:"outputPrefix"`
		TrajectorySchema string           `json:"trajectorySchema"`
		Contract         contractJSON     `json:"contract"`
		Calibration      *calibrationJSON `json:"calibration"`
	}{toRun(job.Run), testSubjectJSON{job.Run.TestSubjectID, job.Arena}, job.Run.OutputPrefix(), domain.TrajectorySchema, contract, calibration}, err)
}

func (h handler) heartbeat(w http.ResponseWriter, r *http.Request, worker domain.Worker) {
	var body attemptBody
	if !h.decode(w, r, &body) {
		return
	}
	run, err := h.service.Heartbeat(r.Context(), worker, r.PathValue("runId"), body.Attempt)
	h.respond(w, r, http.StatusOK, toRun(run), err)
}

func (h handler) sourceURL(w http.ResponseWriter, r *http.Request, worker domain.Worker) {
	var body attemptBody
	if !h.decode(w, r, &body) {
		return
	}
	url, expires, err := h.service.SourceURL(r.Context(), worker, r.PathValue("runId"), body.Attempt)
	h.respond(w, r, http.StatusOK, struct {
		URL       string    `json:"url"`
		ExpiresAt time.Time `json:"expiresAt"`
	}{url, expires}, err)
}

func (h handler) output(w http.ResponseWriter, r *http.Request, worker domain.Worker) {
	var body struct {
		Attempt     int    `json:"attempt"`
		Kind        string `json:"kind"`
		FileName    string `json:"fileName"`
		ContentType string `json:"contentType"`
		SizeBytes   int64  `json:"sizeBytes"`
		CRC32C      string `json:"crc32c"`
	}
	if !h.decode(w, r, &body) {
		return
	}
	checksum, err := media.ParseCRC32C(body.CRC32C)
	if err != nil {
		h.error(w, r, application.ErrInvalidOutput)
		return
	}
	upload, signed, err := h.service.RequestOutput(r.Context(), worker, r.PathValue("runId"), application.OutputRequest{
		Attempt: body.Attempt, Kind: body.Kind, FileName: body.FileName, ContentType: body.ContentType, SizeBytes: body.SizeBytes, CRC32C: checksum,
	})
	h.respond(w, r, http.StatusCreated, struct {
		ObjectName string `json:"objectName"`
		Upload     struct {
			Method    string            `json:"method"`
			URL       string            `json:"url"`
			Headers   map[string]string `json:"headers"`
			ExpiresAt time.Time         `json:"expiresAt"`
		} `json:"upload"`
	}{ObjectName: upload.ObjectName, Upload: struct {
		Method    string            `json:"method"`
		URL       string            `json:"url"`
		Headers   map[string]string `json:"headers"`
		ExpiresAt time.Time         `json:"expiresAt"`
	}(signed)}, err)
}

func (h handler) submit(w http.ResponseWriter, r *http.Request, worker domain.Worker) {
	var body struct {
		Attempt             int    `json:"attempt"`
		ModelVersion        string `json:"modelVersion"`
		RecordingDurationUs int64  `json:"recordingDurationUs"`
		Artifacts           []struct {
			Kind       string `json:"kind"`
			ObjectName string `json:"objectName"`
		} `json:"artifacts"`
		Pair struct {
			AnalyzedObjectName string `json:"analyzedObjectName"`
			SourceOffsetUs     int64  `json:"sourceOffsetUs"`
			OutputOffsetUs     int64  `json:"outputOffsetUs"`
			TimeMappingVersion string `json:"timeMappingVersion"`
		} `json:"pair"`
	}
	if !h.decode(w, r, &body) {
		return
	}
	sub := domain.Submission{Attempt: body.Attempt, ModelVersion: body.ModelVersion, RecordingDurationUs: body.RecordingDurationUs, Pair: domain.PairInput(body.Pair)}
	for _, a := range body.Artifacts {
		sub.Artifacts = append(sub.Artifacts, domain.ArtifactInput(a))
	}
	run, err := h.service.Submit(r.Context(), worker, r.PathValue("runId"), sub)
	h.respond(w, r, http.StatusOK, toRun(run), err)
}

func (h handler) fail(w http.ResponseWriter, r *http.Request, worker domain.Worker) {
	var body struct {
		Attempt   int    `json:"attempt"`
		Reason    string `json:"reason"`
		Retryable bool   `json:"retryable"`
	}
	if !h.decode(w, r, &body) {
		return
	}
	run, err := h.service.Fail(r.Context(), worker, r.PathValue("runId"), body.Attempt, body.Reason, body.Retryable)
	h.respond(w, r, http.StatusOK, toRun(run), err)
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

type dataJSON struct {
	Data any `json:"data"`
}

type workerJSON struct {
	ID           string           `json:"id"`
	Name         string           `json:"name"`
	ModelVersion string           `json:"modelVersion"`
	Capabilities []capabilityJSON `json:"capabilities"`
	DisabledAt   *time.Time       `json:"disabledAt"`
	CreatedAt    time.Time        `json:"createdAt"`
}

func toWorker(w domain.Worker) workerJSON {
	return workerJSON{w.ID, w.Name, w.ModelVersion, mapSlice(w.Capabilities, func(c domain.Capability) capabilityJSON { return capabilityJSON(c) }), w.DisabledAt, w.CreatedAt}
}

func capabilities(in []capabilityJSON) []domain.Capability {
	out := make([]domain.Capability, 0, len(in))
	for _, c := range in {
		out = append(out, domain.Capability(c))
	}
	return out
}

type runJSON struct {
	ID                  string             `json:"id"`
	ExperimentID        string             `json:"experimentId"`
	TestID              string             `json:"testId"`
	TestSubjectID       string             `json:"testSubjectId"`
	RecordingID         string             `json:"recordingId"`
	SourceVideoID       string             `json:"sourceVideoId"`
	SourceGeneration    string             `json:"sourceGeneration"`
	SourceCRC32C        string             `json:"sourceCrc32c"`
	ClipStartUs         int64              `json:"clipStartUs"`
	ClipEndUs           *int64             `json:"clipEndUs"`
	CalibrationID       *string            `json:"calibrationId"`
	ParadigmKey         string             `json:"paradigmKey"`
	ParadigmVersion     int                `json:"paradigmVersion"`
	MetricEngineVersion int                `json:"metricEngineVersion"`
	ResultSchemaVersion int                `json:"resultSchemaVersion"`
	Parameters          map[string]float64 `json:"parameters"`
	Trigger             string             `json:"trigger"`
	Status              string             `json:"status"`
	Attempt             int                `json:"attempt"`
	MaxAttempts         int                `json:"maxAttempts"`
	WorkerID            *string            `json:"workerId"`
	LeaseExpiresAt      *time.Time         `json:"leaseExpiresAt"`
	ModelVersion        *string            `json:"modelVersion"`
	FailureReason       *string            `json:"failureReason"`
	CreatedBy           *string            `json:"createdBy"`
	CreatedAt           time.Time          `json:"createdAt"`
	FinishedAt          *time.Time         `json:"finishedAt"`
}

func toRun(r domain.Run) runJSON {
	params := r.Parameters
	if params == nil {
		params = map[string]float64{}
	}
	return runJSON{
		ID: r.ID, ExperimentID: r.ExperimentID, TestID: r.TestID, TestSubjectID: r.TestSubjectID, RecordingID: r.RecordingID, SourceVideoID: r.SourceAssetID,
		SourceGeneration: itoa(r.SourceGeneration), SourceCRC32C: media.FormatCRC32C(r.SourceCRC32C), ClipStartUs: r.ClipStartUs, ClipEndUs: r.ClipEndUs,
		CalibrationID: nullable(r.CalibrationID), ParadigmKey: r.ParadigmKey, ParadigmVersion: r.ParadigmVersion,
		MetricEngineVersion: r.MetricEngineVersion, ResultSchemaVersion: r.ResultSchemaVersion, Parameters: params, Trigger: string(r.Trigger), Status: string(r.Status), Attempt: r.Attempt,
		MaxAttempts: r.MaxAttempts, WorkerID: nullable(r.WorkerID), LeaseExpiresAt: r.LeaseExpiresAt, ModelVersion: nullable(r.ModelVersion),
		FailureReason: nullable(r.FailureReason), CreatedBy: nullable(r.CreatedBy), CreatedAt: r.CreatedAt, FinishedAt: r.FinishedAt,
	}
}

// testSubjectJSON names the animal a worker analyses by its test subject id
// and arena only; workers never learn the animal, group or treatment.
type testSubjectJSON struct {
	ID                    string `json:"id"`
	EnvironmentRevisionID string `json:"environmentRevisionId"`
}

type reviewGroupJSON struct {
	ID               string  `json:"id"`
	Code             string  `json:"code"`
	Name             string  `json:"name"`
	Role             string  `json:"role"`
	DiseaseModelName *string `json:"diseaseModelName"`
	SubstanceName    *string `json:"substanceName"`
	DoseMicro        *int64  `json:"doseMicro"`
	Unit             *string `json:"unit"`
}

type reviewRunJSON struct {
	ID                  string       `json:"id"`
	Trigger             string       `json:"trigger"`
	ModelVersion        *string      `json:"modelVersion"`
	MetricEngineVersion int          `json:"metricEngineVersion"`
	CalibrationID       *string      `json:"calibrationId"`
	RecordingID         string       `json:"recordingId"`
	FinishedAt          *time.Time   `json:"finishedAt"`
	Metrics             []metricJSON `json:"metrics"`
}

// reviewJSON is what the review screen shows. group is null in a blinded experiment.
type reviewJSON struct {
	TestSubjectID     string           `json:"testSubjectId"`
	Status            string           `json:"status"`
	Reason            *string          `json:"reason"`
	Note              *string          `json:"note"`
	ReviewedBy        *string          `json:"reviewedBy"`
	ReviewedAt        *time.Time       `json:"reviewedAt"`
	CreatedAt         time.Time        `json:"createdAt"`
	UpdatedAt         time.Time        `json:"updatedAt"`
	TestID            string           `json:"testId"`
	ExperimentID      string           `json:"experimentId"`
	ExperimentCode    string           `json:"experimentCode"`
	ExperimentStatus  string           `json:"experimentStatus"`
	Blinded           bool             `json:"blinded"`
	SubjectCode       string           `json:"subjectCode"`
	Excluded          bool             `json:"excluded"`
	Arena             arenaJSON        `json:"arena"`
	TimepointID       string           `json:"timepointId"`
	TimepointName     string           `json:"timepointName"`
	ParadigmKey       string           `json:"paradigmKey"`
	ParadigmVersion   int              `json:"paradigmVersion"`
	ScheduledAt       time.Time        `json:"scheduledAt"`
	Run               reviewRunJSON    `json:"run"`
	LatestRunID       string           `json:"latestRunId"`
	NewerRunAvailable bool             `json:"newerRunAvailable"`
	Group             *reviewGroupJSON `json:"group"`
}

type arenaJSON struct {
	EnvironmentID         string `json:"environmentId"`
	EnvironmentName       string `json:"environmentName"`
	EnvironmentRevisionID string `json:"environmentRevisionId"`
	Revision              int    `json:"revision"`
}

func toReview(i application.ReviewItem) reviewJSON {
	out := reviewJSON{
		TestSubjectID: i.Result.TestSubjectID, Status: string(i.Result.Status), Reason: nullable(i.Result.Reason), Note: nullable(i.Result.Note),
		ReviewedBy: nullable(i.Result.ReviewedBy), ReviewedAt: i.Result.ReviewedAt, CreatedAt: i.Result.CreatedAt, UpdatedAt: i.Result.UpdatedAt,
		TestID: i.TestID, ExperimentID: i.ExperimentID, ExperimentCode: i.ExperimentCode, ExperimentStatus: i.ExperimentStatus, Blinded: i.Blinded,
		SubjectCode: i.SubjectCode, Excluded: i.Excluded,
		Arena:       arenaJSON{i.EnvironmentID, i.EnvironmentName, i.EnvironmentRevisionID, i.EnvironmentRevision},
		TimepointID: i.TimepointID, TimepointName: i.TimepointName, ParadigmKey: i.ParadigmKey, ParadigmVersion: i.ParadigmVersion, ScheduledAt: i.ScheduledAt,
		Run: reviewRunJSON{ID: i.Run.ID, Trigger: i.Run.Trigger, ModelVersion: nullable(i.Run.ModelVersion), MetricEngineVersion: i.Run.MetricEngineVersion,
			CalibrationID: nullable(i.Run.CalibrationID), RecordingID: i.Run.RecordingID, FinishedAt: i.Run.FinishedAt, Metrics: []metricJSON{}},
		LatestRunID: i.LatestRunID, NewerRunAvailable: i.NewerRunAvailable(),
	}
	for _, m := range i.Metrics {
		out.Run.Metrics = append(out.Run.Metrics, metricJSON{m.Key, m.Unit, m.Value, nullable(m.MissingReason)})
	}
	if g := i.Group; g != nil {
		out.Group = &reviewGroupJSON{g.ID, g.Code, g.Name, g.Role, nullable(g.DiseaseModelName), nullable(g.SubstanceName), g.DoseMicro, nullable(g.Unit)}
	}
	return out
}

type metricJSON struct {
	Key           string   `json:"key"`
	Unit          string   `json:"unit"`
	Value         *float64 `json:"value"`
	MissingReason *string  `json:"missingReason"`
}

type eventJSON struct {
	Type       string  `json:"type"`
	Label      *string `json:"label"`
	Kind       string  `json:"kind"`
	StartUs    int64   `json:"startUs"`
	EndUs      int64   `json:"endUs"`
	Confidence float64 `json:"confidence"`
	TrialID    *string `json:"trialId"`
}

type artifactJSON struct {
	Kind        string `json:"kind"`
	ObjectName  string `json:"objectName"`
	Generation  string `json:"generation"`
	SizeBytes   int64  `json:"sizeBytes"`
	ContentType string `json:"contentType"`
}

type resultJSON struct {
	Metrics   []metricJSON   `json:"metrics"`
	Events    []eventJSON    `json:"events"`
	Artifacts []artifactJSON `json:"artifacts"`
	Pair      *struct {
		SourceVideoID      string `json:"sourceVideoId"`
		SourceGeneration   string `json:"sourceGeneration"`
		AnalyzedVideoID    string `json:"analyzedVideoId"`
		SourceOffsetUs     int64  `json:"sourceOffsetUs"`
		OutputOffsetUs     int64  `json:"outputOffsetUs"`
		TimeMappingVersion string `json:"timeMappingVersion"`
	} `json:"pair"`
}

func toResult(p application.Published) *resultJSON {
	out := &resultJSON{Metrics: []metricJSON{}, Events: []eventJSON{}, Artifacts: []artifactJSON{}}
	for _, m := range p.Metrics {
		out.Metrics = append(out.Metrics, metricJSON{m.Key, m.Unit, m.Value, nullable(m.MissingReason)})
	}
	for _, e := range p.Events {
		out.Events = append(out.Events, eventJSON{e.Type, nullable(e.Label), e.Kind, e.StartUs, e.EndUs, e.Confidence, nullable(e.TrialID)})
	}
	for _, a := range p.Artifacts {
		out.Artifacts = append(out.Artifacts, artifactJSON{a.Kind, a.ObjectName, itoa(a.Generation), a.SizeBytes, a.ContentType})
	}
	if p.Pair != nil {
		out.Pair = &struct {
			SourceVideoID      string `json:"sourceVideoId"`
			SourceGeneration   string `json:"sourceGeneration"`
			AnalyzedVideoID    string `json:"analyzedVideoId"`
			SourceOffsetUs     int64  `json:"sourceOffsetUs"`
			OutputOffsetUs     int64  `json:"outputOffsetUs"`
			TimeMappingVersion string `json:"timeMappingVersion"`
		}{p.Pair.SourceAssetID, itoa(p.Pair.SourceGeneration), p.Pair.AnalyzedAssetID, p.Pair.SourceOffsetUs, p.Pair.OutputOffsetUs, p.Pair.TimeMappingVersion}
	}
	return out
}

type metricSpecJSON struct {
	Key     string  `json:"key"`
	Unit    string  `json:"unit"`
	Integer bool    `json:"integer"`
	Min     float64 `json:"min"`
	Max     float64 `json:"max"`
}

type eventSpecJSON struct {
	Type string `json:"type"`
	Kind string `json:"kind"`
}

type qcJSON struct {
	Key      string  `json:"key"`
	Operator string  `json:"operator"`
	Value    float64 `json:"value"`
}

// contractJSON lists what the engine will compute; workers send trajectories, not metrics.
type contractJSON struct {
	// RequiredZones are the calibrated zones the claimed calibration carries.
	RequiredZones  []string         `json:"requiredZones"`
	Metrics        []metricSpecJSON `json:"metrics"`
	Events         []eventSpecJSON  `json:"events"`
	MissingReasons []string         `json:"missingReasons"`
	QC             []qcJSON         `json:"qc"`
}

type cropJSON struct {
	X      int `json:"x"`
	Y      int `json:"y"`
	Width  int `json:"width"`
	Height int `json:"height"`
}

type calibrationJSON struct {
	ID               string     `json:"id"`
	FrameWidth       int        `json:"frameWidth"`
	FrameHeight      int        `json:"frameHeight"`
	Crop             cropJSON   `json:"crop"`
	MeasurementPlane string     `json:"measurementPlane"`
	Transform        [9]float64 `json:"transform"`
	Zones            []zoneJSON `json:"zones"`
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

// zoneJSON is one calibrated zone shape in full-frame pixels and in the
// calibration's centimetres.
type zoneJSON struct {
	Key         string    `json:"key"`
	Pixels      shapeJSON `json:"pixels"`
	Centimeters shapeJSON `json:"centimeters"`
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

func mapSlice[T, U any](items []T, convert func(T) U) []U {
	out := make([]U, 0, len(items))
	for _, item := range items {
		out = append(out, convert(item))
	}
	return out
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }

func nullable(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
