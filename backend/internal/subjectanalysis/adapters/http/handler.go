// Package subjectanalysishttp exposes the analyses of the animals of a test
// and the review of each animal's result. The routes keep the names the panel
// knows: a "run" is an analysis job of one animal, and its id is the job id.
// A job's own result (metrics, events, videos) is read from the analysis
// service's job routes.
package subjectanalysishttp

import (
	"encoding/base64"
	"encoding/binary"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	access "github.com/HappyHackingSpace/Misko/backend/internal/access/domain"
	"github.com/HappyHackingSpace/Misko/backend/internal/platform/httpjson"
	"github.com/HappyHackingSpace/Misko/backend/internal/subjectanalysis/application"
	"github.com/HappyHackingSpace/Misko/backend/internal/subjectanalysis/domain"
)

var failures = []httpjson.Failure{
	{Err: access.ErrUnauthenticated, Status: http.StatusUnauthorized, Code: "auth.unauthenticated"},
	{Err: access.ErrForbidden, Status: http.StatusForbidden, Code: "auth.forbidden"},
	{Err: application.ErrTestSubjectNotFound, Status: http.StatusNotFound, Code: "analysis.testSubjectNotFound"},
	{Err: application.ErrRunNotFound, Status: http.StatusNotFound, Code: "analysis.runNotFound"},
	{Err: application.ErrResultNotFound, Status: http.StatusNotFound, Code: "analysis.resultNotFound"},
	{Err: application.ErrExperimentClosed, Status: http.StatusConflict, Code: "analysis.experimentClosed"},
	{Err: application.ErrInvalidQuery, Status: http.StatusBadRequest, Code: "analysis.invalidQuery"},
	{Err: application.ErrInvalidBulk, Status: http.StatusBadRequest, Code: "analysis.invalidBulk"},
	{Err: application.ErrNotReady, Status: http.StatusConflict, Code: "analysis.notReady"},
	{Err: application.ErrRunPending, Status: http.StatusConflict, Code: "analysis.runPending"},
	{Err: domain.ErrInvalidRejection, Status: http.StatusBadRequest, Code: "analysis.invalidRejection"},
	{Err: domain.ErrInvalidNote, Status: http.StatusBadRequest, Code: "analysis.invalidNote"},
	{Err: domain.ErrRunNotChosen, Status: http.StatusConflict, Code: "analysis.runNotChosen"},
}

type handler struct {
	service      *application.Service
	authenticate func(*http.Request) (access.Actor, error)
	logger       *slog.Logger
}

func Register(mux *http.ServeMux, service *application.Service, authenticate func(*http.Request) (access.Actor, error), logger *slog.Logger) {
	h := handler{service: service, authenticate: authenticate, logger: logger}
	mux.HandleFunc("POST /api/analysis/test-subjects/{testSubjectId}/runs", h.with(h.reanalyze))
	mux.HandleFunc("GET /api/tests/{id}/analysis-runs", h.with(h.runs))
	mux.HandleFunc("GET /api/analysis/results", h.with(h.results))
	mux.HandleFunc("POST /api/analysis/results/bulk-accept", h.with(h.acceptMany))
	mux.HandleFunc("GET /api/analysis/results/{testSubjectId}", h.with(h.result))
	mux.HandleFunc("POST /api/analysis/results/{testSubjectId}/accept", h.with(h.accept))
	mux.HandleFunc("POST /api/analysis/results/{testSubjectId}/reject", h.with(h.reject))
	mux.HandleFunc("POST /api/analysis/results/{testSubjectId}/run", h.with(h.chooseRun))
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

// chooseRun accepts the job as runId, the name the panel sends, or jobId.
func (h handler) chooseRun(w http.ResponseWriter, r *http.Request, actor access.Actor) {
	var body struct {
		RunID string `json:"runId"`
		JobID string `json:"jobId"`
	}
	if !h.decode(w, r, &body) {
		return
	}
	job := body.JobID
	if job == "" {
		job = body.RunID
	}
	item, err := h.service.ChooseRun(r.Context(), actor, r.PathValue("testSubjectId"), job)
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

// runJSON is one analysis of an animal: the link's facts and the job's state.
type runJSON struct {
	ID                  string             `json:"id"`
	JobID               string             `json:"jobId"`
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
	j := r.Job
	params := j.Parameters
	if params == nil {
		params = map[string]float64{}
	}
	return runJSON{
		ID: r.JobID, JobID: r.JobID, ExperimentID: r.ExperimentID, TestID: r.TestID, TestSubjectID: r.TestSubjectID, RecordingID: r.RecordingID,
		SourceVideoID: j.SourceAssetID, SourceGeneration: strconv.FormatInt(j.SourceGeneration, 10), SourceCRC32C: formatCRC32C(j.SourceCRC32C),
		ClipStartUs: j.ClipStartUs, ClipEndUs: j.ClipEndUs, CalibrationID: nullable(r.CalibrationID), ParadigmKey: j.ParadigmKey,
		ParadigmVersion: j.ParadigmVersion, MetricEngineVersion: j.MetricEngineVersion, ResultSchemaVersion: j.ResultSchemaVersion, Parameters: params,
		Trigger: string(r.Trigger), Status: string(j.Status), Attempt: j.Attempt, MaxAttempts: j.MaxAttempts, WorkerID: nullable(j.WorkerID),
		LeaseExpiresAt: j.LeaseExpiresAt, ModelVersion: nullable(j.ModelVersion), FailureReason: nullable(j.FailureReason),
		CreatedBy: nullable(r.CreatedBy), CreatedAt: j.CreatedAt, FinishedAt: j.FinishedAt,
	}
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

type arenaJSON struct {
	EnvironmentID         string `json:"environmentId"`
	EnvironmentName       string `json:"environmentName"`
	EnvironmentRevisionID string `json:"environmentRevisionId"`
	Revision              int    `json:"revision"`
}

type metricJSON struct {
	Key           string   `json:"key"`
	Unit          string   `json:"unit"`
	Value         *float64 `json:"value"`
	MissingReason *string  `json:"missingReason"`
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

func mapSlice[T, U any](items []T, convert func(T) U) []U {
	out := make([]U, 0, len(items))
	for _, item := range items {
		out = append(out, convert(item))
	}
	return out
}

func nullable(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func formatCRC32C(v uint32) string {
	return base64.StdEncoding.EncodeToString(binary.BigEndian.AppendUint32(nil, v))
}
