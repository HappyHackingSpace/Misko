// Package analysishttp exposes the analysis service: worker management and
// job reads to users, and the job protocol to analysis workers. Workers
// authenticate with "Authorization: Worker <token>"; user tokens are not
// accepted on worker routes and worker tokens are not accepted on user routes.
// The worker routes keep the names workers know ("runs"); a run id is a job id.
package analysishttp

import (
	"log/slog"
	"net/http"
	"strings"

	access "github.com/HappyHackingSpace/Misko/backend/internal/access/domain"
	"github.com/HappyHackingSpace/Misko/backend/internal/analysis/application"
	"github.com/HappyHackingSpace/Misko/backend/internal/analysis/domain"
	"github.com/HappyHackingSpace/Misko/backend/internal/platform/httpjson"
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
	{Err: application.ErrJobNotFound, Status: http.StatusNotFound, Code: "analysis.jobNotFound"},
	{Err: application.ErrJobNotSucceeded, Status: http.StatusConflict, Code: "analysis.jobNotSucceeded"},
	{Err: application.ErrOutputNotFound, Status: http.StatusNotFound, Code: "analysis.outputNotFound"},
	{Err: application.ErrSourceNotFound, Status: http.StatusNotFound, Code: "analysis.sourceNotFound"},
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
	mux.HandleFunc("GET /api/analysis/jobs/{jobId}", h.user(h.job))
	mux.HandleFunc("GET /api/analysis/jobs/{jobId}/video-pair", h.user(h.videoPair))
	mux.HandleFunc("GET /api/analysis/jobs/{jobId}/outputs/{kind}/read-url", h.user(h.outputURL))
	// Deprecated names of the two job reads, kept while the panel moves to the job routes.
	mux.HandleFunc("GET /api/analysis-runs/{jobId}", h.user(h.job))
	mux.HandleFunc("GET /api/analysis-runs/{jobId}/video-pair", h.user(h.videoPair))

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

func (h handler) job(w http.ResponseWriter, r *http.Request, actor access.Actor) {
	detail, err := h.service.JobDetail(r.Context(), actor, r.PathValue("jobId"))
	h.respond(w, r, http.StatusOK, toJobDetail(detail), err)
}

func (h handler) videoPair(w http.ResponseWriter, r *http.Request, actor access.Actor) {
	links, err := h.service.VideoPair(r.Context(), actor, r.PathValue("jobId"))
	h.respond(w, r, http.StatusOK, toVideoPair(links), err)
}

func (h handler) outputURL(w http.ResponseWriter, r *http.Request, actor access.Actor) {
	link, err := h.service.OutputURL(r.Context(), actor, r.PathValue("jobId"), r.PathValue("kind"))
	h.respond(w, r, http.StatusOK, readLinkJSON{link.URL, link.ExpiresAt}, err)
}

type attemptBody struct {
	Attempt int `json:"attempt"`
}

func (h handler) claim(w http.ResponseWriter, r *http.Request, worker domain.Worker) {
	var body struct{}
	if !h.decode(w, r, &body) {
		return
	}
	claimed, err := h.service.Claim(r.Context(), worker)
	if err == application.ErrNoJob {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	h.respond(w, r, http.StatusOK, toClaim(claimed), err)
}

func (h handler) heartbeat(w http.ResponseWriter, r *http.Request, worker domain.Worker) {
	var body attemptBody
	if !h.decode(w, r, &body) {
		return
	}
	job, err := h.service.Heartbeat(r.Context(), worker, r.PathValue("runId"), body.Attempt)
	h.respond(w, r, http.StatusOK, toJob(job), err)
}

func (h handler) sourceURL(w http.ResponseWriter, r *http.Request, worker domain.Worker) {
	var body attemptBody
	if !h.decode(w, r, &body) {
		return
	}
	url, expires, err := h.service.SourceURL(r.Context(), worker, r.PathValue("runId"), body.Attempt)
	h.respond(w, r, http.StatusOK, readLinkJSON{url, expires}, err)
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
	checksum, ok := parseCRC32C(body.CRC32C)
	if !ok {
		h.error(w, r, application.ErrInvalidOutput)
		return
	}
	upload, signed, err := h.service.RequestOutput(r.Context(), worker, r.PathValue("runId"), application.OutputRequest{
		Attempt: body.Attempt, Kind: body.Kind, FileName: body.FileName, ContentType: body.ContentType, SizeBytes: body.SizeBytes, CRC32C: checksum,
	})
	h.respond(w, r, http.StatusCreated, struct {
		ObjectName string            `json:"objectName"`
		Upload     signedRequestJSON `json:"upload"`
	}{upload.ObjectName, signedRequestJSON(signed)}, err)
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
	job, err := h.service.SubmitResult(r.Context(), worker, r.PathValue("runId"), sub)
	h.respond(w, r, http.StatusOK, toJob(job), err)
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
	job, err := h.service.Fail(r.Context(), worker, r.PathValue("runId"), body.Attempt, body.Reason, body.Retryable)
	h.respond(w, r, http.StatusOK, toJob(job), err)
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
