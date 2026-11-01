// Package testshttp exposes tests, their animals, trials and comments over HTTP.
package testshttp

import (
	access "github.com/HappyHackingSpace/Misko/backend/internal/access/domain"
	"github.com/HappyHackingSpace/Misko/backend/internal/platform/httpjson"
	"github.com/HappyHackingSpace/Misko/backend/internal/tests/application"
	"github.com/HappyHackingSpace/Misko/backend/internal/tests/domain"
	"log/slog"
	"net/http"
	"time"
)

var failures = []httpjson.Failure{
	{Err: access.ErrUnauthenticated, Status: http.StatusUnauthorized, Code: "auth.unauthenticated"},
	{Err: access.ErrForbidden, Status: http.StatusForbidden, Code: "auth.forbidden"},
	{Err: application.ErrExperimentNotFound, Status: http.StatusNotFound, Code: "experiment.notFound"},
	{Err: application.ErrExperimentNotRunning, Status: http.StatusConflict, Code: "test.experimentNotRunning"},
	{Err: application.ErrTimepointNotFound, Status: http.StatusNotFound, Code: "timepoint.notFound"},
	{Err: application.ErrProtocolVersionNotFound, Status: http.StatusNotFound, Code: "protocol.versionNotFound"},
	{Err: application.ErrStepNotFound, Status: http.StatusNotFound, Code: "protocol.stepNotFound"},
	{Err: application.ErrAllocationNotFound, Status: http.StatusNotFound, Code: "allocation.notFound"},
	{Err: application.ErrAnimalExcluded, Status: http.StatusConflict, Code: "test.animalExcluded"},
	{Err: application.ErrAnimalAlreadyScheduled, Status: http.StatusConflict, Code: "test.animalAlreadyScheduled"},
	{Err: application.ErrArenaNotFound, Status: http.StatusNotFound, Code: "environment.revisionNotFound"},
	{Err: application.ErrArenaParadigmMismatch, Status: http.StatusBadRequest, Code: "test.arenaParadigmMismatch"},
	{Err: application.ErrTestNotFound, Status: http.StatusNotFound, Code: "test.notFound"},
	{Err: application.ErrTestSubjectNotFound, Status: http.StatusNotFound, Code: "test.subjectNotFound"},
	{Err: application.ErrCommentNotFound, Status: http.StatusNotFound, Code: "comment.notFound"},
	{Err: application.ErrCannotEditComment, Status: http.StatusForbidden, Code: "comment.cannotEdit"},
	{Err: application.ErrCannotDeleteComment, Status: http.StatusForbidden, Code: "comment.cannotDelete"},
	{Err: application.ErrRateLimited, Status: http.StatusTooManyRequests, Code: "comment.rateLimited"},
	{Err: application.ErrInvalidQuery, Status: http.StatusBadRequest, Code: "common.invalidQuery"},
	{Err: domain.ErrInvalidTransition, Status: http.StatusConflict, Code: "test.invalidTransition"},
	{Err: domain.ErrTestNotInProgress, Status: http.StatusConflict, Code: "test.notInProgress"},
	{Err: domain.ErrNotPlanned, Status: http.StatusConflict, Code: "test.notPlanned"},
	{Err: domain.ErrLastSubject, Status: http.StatusConflict, Code: "test.lastSubject"},
	{Err: domain.ErrSubjectExcluded, Status: http.StatusConflict, Code: "test.subjectExcluded"},
	{Err: domain.ErrNoTrials, Status: http.StatusConflict, Code: "test.noTrials"},
	{Err: domain.ErrNoSubjects, Status: http.StatusBadRequest, Code: "test.noSubjects"},
	{Err: domain.ErrTooManySubjects, Status: http.StatusBadRequest, Code: "test.tooManySubjects"},
	{Err: domain.ErrMissingAllocation, Status: http.StatusBadRequest, Code: "test.missingAllocation"},
	{Err: domain.ErrMissingArena, Status: http.StatusBadRequest, Code: "test.missingArena"},
	{Err: domain.ErrDuplicateAnimal, Status: http.StatusConflict, Code: "test.duplicateAnimal"},
	{Err: domain.ErrDuplicateArena, Status: http.StatusConflict, Code: "test.duplicateArena"},
	{Err: domain.ErrCompletedBeforeTrial, Status: http.StatusBadRequest, Code: "test.completedBeforeTrial"},
	{Err: domain.ErrMissingTime, Status: http.StatusBadRequest, Code: "common.invalidTime"},
	{Err: domain.ErrFutureTime, Status: http.StatusBadRequest, Code: "common.futureTime"},
	{Err: domain.ErrInvalidReason, Status: http.StatusBadRequest, Code: "test.invalidReason"},
	{Err: domain.ErrInvalidNotes, Status: http.StatusBadRequest, Code: "common.invalidNotes"},
	{Err: domain.ErrInvalidRepetition, Status: http.StatusBadRequest, Code: "trial.invalidRepetition"},
	{Err: domain.ErrTrialBeforeTest, Status: http.StatusBadRequest, Code: "trial.beforeTest"},
	{Err: domain.ErrInvalidTrialEnd, Status: http.StatusBadRequest, Code: "trial.invalidEnd"},
	{Err: domain.ErrInvalidComment, Status: http.StatusBadRequest, Code: "comment.invalid"},
}

type handler struct {
	service      *application.Service
	authenticate func(*http.Request) (access.Actor, error)
	logger       *slog.Logger
}

func Register(mux *http.ServeMux, service *application.Service, authenticate func(*http.Request) (access.Actor, error), logger *slog.Logger) {
	h := handler{service: service, authenticate: authenticate, logger: logger}
	mux.HandleFunc("GET /api/tests", h.with(h.listAll))
	mux.HandleFunc("GET /api/experiments/{id}/tests", h.with(h.list))
	mux.HandleFunc("POST /api/experiments/{id}/tests", h.with(h.create))
	mux.HandleFunc("GET /api/tests/{id}", h.with(h.get))
	mux.HandleFunc("POST /api/tests/{id}/start", h.with(h.start))
	mux.HandleFunc("POST /api/tests/{id}/complete", h.with(h.complete))
	mux.HandleFunc("POST /api/tests/{id}/cancel", h.with(h.cancel))
	mux.HandleFunc("POST /api/tests/{id}/subjects", h.with(h.addSubject))
	mux.HandleFunc("DELETE /api/tests/{id}/subjects/{subjectId}", h.with(h.removeSubject))
	mux.HandleFunc("GET /api/tests/{id}/subjects/{subjectId}/trials", h.with(h.trials))
	mux.HandleFunc("POST /api/tests/{id}/subjects/{subjectId}/trials", h.with(h.recordTrial))
	mux.HandleFunc("GET /api/tests/{id}/comments", h.with(h.comments))
	mux.HandleFunc("POST /api/tests/{id}/comments", h.with(h.createComment))
	mux.HandleFunc("PATCH /api/tests/{id}/comments/{commentId}", h.with(h.updateComment))
	mux.HandleFunc("DELETE /api/tests/{id}/comments/{commentId}", h.with(h.deleteComment))
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

func query(r *http.Request) application.TestQuery {
	q := r.URL.Query()
	return application.TestQuery{
		ExperimentID: q.Get("experimentId"), TimepointID: q.Get("timepointId"), SubjectID: q.Get("subjectId"), Status: q.Get("status"),
		ParadigmKey: q.Get("paradigmKey"), From: q.Get("from"), To: q.Get("to"), Order: q.Get("order"), Search: q.Get("search"),
		Page: httpjson.QueryInt(q.Get("page")), PageSize: httpjson.QueryInt(q.Get("pageSize")),
	}
}

func (h handler) list(w http.ResponseWriter, r *http.Request, actor access.Actor) {
	page, err := h.service.ListTests(r.Context(), actor, r.PathValue("id"), query(r))
	h.respond(w, r, http.StatusOK, pageJSON{mapSlice(page.Tests, testJSON), page.Total, page.Page, page.PageSize}, err)
}

func (h handler) listAll(w http.ResponseWriter, r *http.Request, actor access.Actor) {
	page, err := h.service.ListAllTests(r.Context(), actor, query(r))
	h.respond(w, r, http.StatusOK, pageJSON{mapSlice(page.Tests, testJSON), page.Total, page.Page, page.PageSize}, err)
}

type participantBody struct {
	AllocationID          string `json:"allocationId"`
	EnvironmentRevisionID string `json:"environmentRevisionId"`
}

func (h handler) create(w http.ResponseWriter, r *http.Request, actor access.Actor) {
	var body struct {
		TimepointID       string            `json:"timepointId"`
		ProtocolVersionID string            `json:"protocolVersionId"`
		StepPosition      int               `json:"stepPosition"`
		ScheduledAt       time.Time         `json:"scheduledAt"`
		Notes             string            `json:"notes"`
		Subjects          []participantBody `json:"subjects"`
	}
	if !h.decode(w, r, &body) {
		return
	}
	in := application.TestInput{
		TimepointID: body.TimepointID, ProtocolVersionID: body.ProtocolVersionID, StepPosition: body.StepPosition,
		ScheduledAt: body.ScheduledAt, Notes: body.Notes,
	}
	for _, p := range body.Subjects {
		in.Subjects = append(in.Subjects, domain.Participant(p))
	}
	t, err := h.service.CreateTest(r.Context(), actor, r.PathValue("id"), in)
	h.respond(w, r, http.StatusCreated, testJSON(t), err)
}

func (h handler) addSubject(w http.ResponseWriter, r *http.Request, actor access.Actor) {
	var body participantBody
	if !h.decode(w, r, &body) {
		return
	}
	t, err := h.service.AddSubject(r.Context(), actor, r.PathValue("id"), domain.Participant(body))
	h.respond(w, r, http.StatusCreated, testJSON(t), err)
}

func (h handler) removeSubject(w http.ResponseWriter, r *http.Request, actor access.Actor) {
	t, err := h.service.RemoveSubject(r.Context(), actor, r.PathValue("id"), r.PathValue("subjectId"))
	h.respond(w, r, http.StatusOK, testJSON(t), err)
}

func (h handler) get(w http.ResponseWriter, r *http.Request, actor access.Actor) {
	detail, err := h.service.Test(r.Context(), actor, r.PathValue("id"))
	h.respond(w, r, http.StatusOK, testDetail(detail), err)
}

func (h handler) start(w http.ResponseWriter, r *http.Request, actor access.Actor) {
	var body struct {
		StartedAt time.Time `json:"startedAt"`
	}
	if !h.decode(w, r, &body) {
		return
	}
	t, err := h.service.StartTest(r.Context(), actor, r.PathValue("id"), body.StartedAt)
	h.respond(w, r, http.StatusOK, testJSON(t), err)
}

func (h handler) complete(w http.ResponseWriter, r *http.Request, actor access.Actor) {
	var body struct {
		CompletedAt time.Time `json:"completedAt"`
	}
	if !h.decode(w, r, &body) {
		return
	}
	t, err := h.service.CompleteTest(r.Context(), actor, r.PathValue("id"), body.CompletedAt)
	h.respond(w, r, http.StatusOK, testJSON(t), err)
}

func (h handler) cancel(w http.ResponseWriter, r *http.Request, actor access.Actor) {
	var body struct {
		Reason string `json:"reason"`
	}
	if !h.decode(w, r, &body) {
		return
	}
	t, err := h.service.CancelTest(r.Context(), actor, r.PathValue("id"), body.Reason)
	h.respond(w, r, http.StatusOK, testJSON(t), err)
}

func (h handler) trials(w http.ResponseWriter, r *http.Request, actor access.Actor) {
	trials, err := h.service.Trials(r.Context(), actor, r.PathValue("id"), r.PathValue("subjectId"))
	h.respond(w, r, http.StatusOK, dataJSON{mapSlice(trials, trialJSON)}, err)
}

func (h handler) recordTrial(w http.ResponseWriter, r *http.Request, actor access.Actor) {
	var body struct {
		Repetition int        `json:"repetition"`
		StartedAt  time.Time  `json:"startedAt"`
		EndedAt    *time.Time `json:"endedAt"`
		Notes      string     `json:"notes"`
	}
	if !h.decode(w, r, &body) {
		return
	}
	trial, err := h.service.RecordTrial(r.Context(), actor, r.PathValue("id"), r.PathValue("subjectId"), domain.TrialInput(body))
	h.respond(w, r, http.StatusCreated, trialJSON(trial), err)
}

func (h handler) comments(w http.ResponseWriter, r *http.Request, actor access.Actor) {
	comments, err := h.service.Comments(r.Context(), actor, r.PathValue("id"))
	h.respond(w, r, http.StatusOK, dataJSON{mapSlice(comments, commentJSON)}, err)
}

type commentBody struct {
	Body string `json:"body"`
}

func (h handler) createComment(w http.ResponseWriter, r *http.Request, actor access.Actor) {
	var body commentBody
	if !h.decode(w, r, &body) {
		return
	}
	c, err := h.service.CreateComment(r.Context(), actor, r.PathValue("id"), body.Body)
	h.respond(w, r, http.StatusCreated, commentJSON(c), err)
}

func (h handler) updateComment(w http.ResponseWriter, r *http.Request, actor access.Actor) {
	var body commentBody
	if !h.decode(w, r, &body) {
		return
	}
	c, err := h.service.UpdateComment(r.Context(), actor, r.PathValue("id"), r.PathValue("commentId"), body.Body)
	h.respond(w, r, http.StatusOK, commentJSON(c), err)
}

func (h handler) deleteComment(w http.ResponseWriter, r *http.Request, actor access.Actor) {
	if err := h.service.DeleteComment(r.Context(), actor, r.PathValue("id"), r.PathValue("commentId")); err != nil {
		h.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h handler) decode(w http.ResponseWriter, r *http.Request, dst any) bool {
	if err := httpjson.Decode(w, r, dst); err != nil {
		h.fail(w, r, err)
		return false
	}
	return true
}

func (h handler) respond(w http.ResponseWriter, r *http.Request, status int, body any, err error) {
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpjson.Write(w, status, body)
}

func (h handler) fail(w http.ResponseWriter, r *http.Request, err error) {
	httpjson.Fail(w, r, h.logger, err, failures)
}

type dataJSON struct {
	Data any `json:"data"`
}

type pageJSON struct {
	Data     any `json:"data"`
	Total    int `json:"total"`
	Page     int `json:"page"`
	PageSize int `json:"pageSize"`
}

type testJSONBody struct {
	ID                string            `json:"id"`
	ExperimentID      string            `json:"experimentId"`
	ExperimentCode    string            `json:"experimentCode"`
	TimepointID       string            `json:"timepointId"`
	TimepointName     string            `json:"timepointName"`
	StudyDay          int               `json:"studyDay"`
	ProtocolID        string            `json:"protocolId"`
	ProtocolName      string            `json:"protocolName"`
	ProtocolVersionID string            `json:"protocolVersionId"`
	ProtocolVersion   int               `json:"protocolVersion"`
	StepPosition      int               `json:"stepPosition"`
	ParadigmKey       string            `json:"paradigmKey"`
	ParadigmVersion   int               `json:"paradigmVersion"`
	PlannedTrials     int               `json:"plannedTrials"`
	Status            string            `json:"status"`
	ScheduledAt       time.Time         `json:"scheduledAt"`
	StartedAt         *time.Time        `json:"startedAt"`
	CompletedAt       *time.Time        `json:"completedAt"`
	CancelledAt       *time.Time        `json:"cancelledAt"`
	CancelReason      *string           `json:"cancelReason"`
	Notes             *string           `json:"notes"`
	CreatedBy         string            `json:"createdBy"`
	CreatedAt         time.Time         `json:"createdAt"`
	UpdatedAt         time.Time         `json:"updatedAt"`
	Subjects          []subjectJSONBody `json:"subjects"`
}

// subjectJSONBody is one animal of a test. Its group is left out on purpose,
// so a blinded study stays blind for the people running its tests.
type subjectJSONBody struct {
	ID                    string          `json:"id"`
	AllocationID          string          `json:"allocationId"`
	SubjectID             string          `json:"subjectId"`
	SubjectCode           string          `json:"subjectCode"`
	Excluded              bool            `json:"excluded"`
	EnvironmentRevisionID string          `json:"environmentRevisionId"`
	EnvironmentID         string          `json:"environmentId"`
	EnvironmentName       string          `json:"environmentName"`
	EnvironmentRevision   int             `json:"environmentRevision"`
	CreatedBy             string          `json:"createdBy"`
	CreatedAt             time.Time       `json:"createdAt"`
	Trials                []trialJSONBody `json:"trials,omitempty"`
}

func testJSON(t domain.Test) testJSONBody {
	out := testJSONBody{
		t.ID, t.ExperimentID, t.ExperimentCode, t.TimepointID, t.TimepointName, t.StudyDay, t.ProtocolID, t.ProtocolName, t.ProtocolVersionID,
		t.ProtocolVersion, t.StepPosition, t.ParadigmKey, t.ParadigmVersion, t.PlannedTrials, string(t.Status), t.ScheduledAt, t.StartedAt,
		t.CompletedAt, t.CancelledAt, nullable(t.CancelReason), nullable(t.Notes), t.CreatedBy, t.CreatedAt, t.UpdatedAt,
		make([]subjectJSONBody, 0, len(t.Subjects)),
	}
	for _, s := range t.Subjects {
		out.Subjects = append(out.Subjects, subjectJSONBody{
			s.ID, s.AllocationID, s.SubjectID, s.SubjectCode, s.Excluded, s.EnvironmentRevisionID, s.EnvironmentID, s.EnvironmentName,
			s.EnvironmentRevision, s.CreatedBy, s.CreatedAt, nil,
		})
	}
	return out
}

// testDetail lists every animal's trials under it; an animal without trials
// has an empty list.
func testDetail(d application.TestDetail) testJSONBody {
	out := testJSON(d.Test)
	for i := range out.Subjects {
		out.Subjects[i].Trials = []trialJSONBody{}
		for _, tr := range d.Trials {
			if tr.TestSubjectID == out.Subjects[i].ID {
				out.Subjects[i].Trials = append(out.Subjects[i].Trials, trialJSON(tr))
			}
		}
	}
	return out
}

type trialJSONBody struct {
	ID            string     `json:"id"`
	TestID        string     `json:"testId"`
	TestSubjectID string     `json:"testSubjectId"`
	Number        int        `json:"number"`
	Repetition    int        `json:"repetition"`
	Attempt       int        `json:"attempt"`
	StartedAt     time.Time  `json:"startedAt"`
	EndedAt       *time.Time `json:"endedAt"`
	Notes         *string    `json:"notes"`
	RecordedBy    string     `json:"recordedBy"`
	CreatedAt     time.Time  `json:"createdAt"`
}

func trialJSON(t domain.Trial) trialJSONBody {
	return trialJSONBody{t.ID, t.TestID, t.TestSubjectID, t.Number, t.Repetition, t.Attempt, t.StartedAt, t.EndedAt, nullable(t.Notes), t.RecordedBy, t.CreatedAt}
}

type commentJSONBody struct {
	ID         string    `json:"id"`
	TestID     string    `json:"testId"`
	AuthorID   string    `json:"authorId"`
	AuthorName *string   `json:"authorName"`
	Body       string    `json:"body"`
	Edited     bool      `json:"edited"`
	CreatedAt  time.Time `json:"createdAt"`
	UpdatedAt  time.Time `json:"updatedAt"`
}

func commentJSON(c domain.Comment) commentJSONBody {
	return commentJSONBody{c.ID, c.TestID, c.AuthorID, nullable(c.AuthorName), c.Body, c.UpdatedAt.After(c.CreatedAt), c.CreatedAt, c.UpdatedAt}
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
