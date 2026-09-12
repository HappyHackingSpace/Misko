// Package experimentshttp exposes experiments, phases, groups, enrollments and
// group assignments over HTTP.
package experimentshttp

import (
	access "github.com/HappyHackingSpace/Misko/backend/internal/access/domain"
	"github.com/HappyHackingSpace/Misko/backend/internal/experiments/application"
	"github.com/HappyHackingSpace/Misko/backend/internal/experiments/domain"
	"github.com/HappyHackingSpace/Misko/backend/internal/platform/httpjson"
	"log/slog"
	"net/http"
	"time"
)

var failures = []httpjson.Failure{
	{Err: access.ErrUnauthenticated, Status: http.StatusUnauthorized, Code: "auth.unauthenticated"},
	{Err: access.ErrForbidden, Status: http.StatusForbidden, Code: "auth.forbidden"},
	{Err: application.ErrExperimentNotFound, Status: http.StatusNotFound, Code: "experiment.notFound"},
	{Err: application.ErrPhaseNotFound, Status: http.StatusNotFound, Code: "phase.notFound"},
	{Err: application.ErrGroupNotFound, Status: http.StatusNotFound, Code: "group.notFound"},
	{Err: application.ErrEnrollmentNotFound, Status: http.StatusNotFound, Code: "enrollment.notFound"},
	{Err: application.ErrSubjectNotFound, Status: http.StatusBadRequest, Code: "enrollment.subjectNotFound"},
	{Err: application.ErrCodeTaken, Status: http.StatusConflict, Code: "experiment.codeTaken"},
	{Err: application.ErrPhaseConflict, Status: http.StatusConflict, Code: "phase.conflict"},
	{Err: application.ErrGroupNameTaken, Status: http.StatusConflict, Code: "group.nameTaken"},
	{Err: application.ErrGroupInUse, Status: http.StatusConflict, Code: "group.inUse"},
	{Err: application.ErrPhaseInUse, Status: http.StatusConflict, Code: "phase.inUse"},
	{Err: application.ErrAlreadyEnrolled, Status: http.StatusConflict, Code: "enrollment.duplicate"},
	{Err: application.ErrOverlappingAssignment, Status: http.StatusConflict, Code: "assignment.overlap"},
	{Err: application.ErrInvalidTime, Status: http.StatusBadRequest, Code: "common.invalidTime"},
	{Err: application.ErrNoChanges, Status: http.StatusBadRequest, Code: "common.noChanges"},
	{Err: application.ErrInvalidQuery, Status: http.StatusBadRequest, Code: "common.invalidQuery"},
	{Err: domain.ErrInvalidCode, Status: http.StatusBadRequest, Code: "experiment.invalidCode"},
	{Err: domain.ErrInvalidTitle, Status: http.StatusBadRequest, Code: "experiment.invalidTitle"},
	{Err: domain.ErrInvalidDescription, Status: http.StatusBadRequest, Code: "common.invalidDescription"},
	{Err: domain.ErrInvalidName, Status: http.StatusBadRequest, Code: "common.invalidName"},
	{Err: domain.ErrInvalidPosition, Status: http.StatusBadRequest, Code: "phase.invalidPosition"},
	{Err: domain.ErrInvalidRole, Status: http.StatusBadRequest, Code: "group.invalidRole"},
	{Err: domain.ErrInvalidTargetSize, Status: http.StatusBadRequest, Code: "group.invalidTargetSize"},
	{Err: domain.ErrGroupNotInExperiment, Status: http.StatusBadRequest, Code: "assignment.groupNotInExperiment"},
	{Err: domain.ErrAssignmentBeforeEnrollment, Status: http.StatusBadRequest, Code: "assignment.beforeEnrollment"},
	{Err: domain.ErrAssignmentOutOfOrder, Status: http.StatusConflict, Code: "assignment.outOfOrder"},
	{Err: domain.ErrAlreadyInGroup, Status: http.StatusConflict, Code: "assignment.alreadyInGroup"},
}

type handler struct {
	service      *application.Service
	authenticate func(*http.Request) (access.Actor, error)
	logger       *slog.Logger
}

func Register(mux *http.ServeMux, service *application.Service, authenticate func(*http.Request) (access.Actor, error), logger *slog.Logger) {
	h := handler{service: service, authenticate: authenticate, logger: logger}
	mux.HandleFunc("GET /api/experiments", h.with(h.listExperiments))
	mux.HandleFunc("POST /api/experiments", h.with(h.createExperiment))
	mux.HandleFunc("GET /api/experiments/{id}", h.with(h.getExperiment))
	mux.HandleFunc("PATCH /api/experiments/{id}", h.with(h.updateExperiment))
	mux.HandleFunc("GET /api/experiments/{id}/phases", h.with(h.listPhases))
	mux.HandleFunc("POST /api/experiments/{id}/phases", h.with(h.createPhase))
	mux.HandleFunc("PATCH /api/experiments/{id}/phases/{phaseId}", h.with(h.updatePhase))
	mux.HandleFunc("DELETE /api/experiments/{id}/phases/{phaseId}", h.with(h.deletePhase))
	mux.HandleFunc("GET /api/experiments/{id}/groups", h.with(h.listGroups))
	mux.HandleFunc("POST /api/experiments/{id}/groups", h.with(h.createGroup))
	mux.HandleFunc("PATCH /api/experiments/{id}/groups/{groupId}", h.with(h.updateGroup))
	mux.HandleFunc("DELETE /api/experiments/{id}/groups/{groupId}", h.with(h.deleteGroup))
	mux.HandleFunc("GET /api/experiments/{id}/enrollments", h.with(h.listEnrollments))
	mux.HandleFunc("POST /api/experiments/{id}/enrollments", h.with(h.enroll))
	mux.HandleFunc("GET /api/experiments/{id}/enrollments/{enrollmentId}", h.with(h.getEnrollment))
	mux.HandleFunc("POST /api/experiments/{id}/enrollments/{enrollmentId}/assignments", h.with(h.assign))
	mux.HandleFunc("GET /api/subjects/{id}/enrollments", h.with(h.subjectEnrollments))
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

func (h handler) listExperiments(w http.ResponseWriter, r *http.Request, actor access.Actor) {
	q := r.URL.Query()
	page, err := h.service.ListExperiments(r.Context(), actor, application.ExperimentQuery{
		Search: q.Get("search"), Sort: q.Get("sort"), Order: q.Get("order"),
		Page: httpjson.QueryInt(q.Get("page")), PageSize: httpjson.QueryInt(q.Get("pageSize")),
	})
	if err != nil {
		h.fail(w, r, err)
		return
	}
	h.write(w, http.StatusOK, pageJSON{mapSlice(page.Experiments, toExperiment), page.Total, page.Page, page.PageSize})
}

func (h handler) createExperiment(w http.ResponseWriter, r *http.Request, actor access.Actor) {
	var body struct {
		Code            string `json:"code"`
		Title           string `json:"title"`
		Description     string `json:"description"`
		RequiresControl bool   `json:"requiresControl"`
	}
	if !h.decode(w, r, &body) {
		return
	}
	e, err := h.service.CreateExperiment(r.Context(), actor, application.ExperimentInput(body))
	h.respond(w, r, http.StatusCreated, toExperiment(e), err)
}

func (h handler) getExperiment(w http.ResponseWriter, r *http.Request, actor access.Actor) {
	detail, err := h.service.Experiment(r.Context(), actor, r.PathValue("id"))
	h.respond(w, r, http.StatusOK, struct {
		experimentJSON
		ControlRequirementMet bool `json:"controlRequirementMet"`
	}{toExperiment(detail.Experiment), detail.ControlRequirementMet}, err)
}

func (h handler) updateExperiment(w http.ResponseWriter, r *http.Request, actor access.Actor) {
	var body struct {
		Code            *string `json:"code"`
		Title           *string `json:"title"`
		Description     *string `json:"description"`
		RequiresControl *bool   `json:"requiresControl"`
	}
	if !h.decode(w, r, &body) {
		return
	}
	e, err := h.service.UpdateExperiment(r.Context(), actor, r.PathValue("id"), application.ExperimentPatch(body))
	h.respond(w, r, http.StatusOK, toExperiment(e), err)
}

func (h handler) listPhases(w http.ResponseWriter, r *http.Request, actor access.Actor) {
	phases, err := h.service.Phases(r.Context(), actor, r.PathValue("id"))
	h.respond(w, r, http.StatusOK, dataJSON{mapSlice(phases, toPhase)}, err)
}

func (h handler) createPhase(w http.ResponseWriter, r *http.Request, actor access.Actor) {
	var body struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Position    int    `json:"position"`
	}
	if !h.decode(w, r, &body) {
		return
	}
	p, err := h.service.CreatePhase(r.Context(), actor, r.PathValue("id"), application.PhaseInput(body))
	h.respond(w, r, http.StatusCreated, toPhase(p), err)
}

func (h handler) updatePhase(w http.ResponseWriter, r *http.Request, actor access.Actor) {
	var body struct {
		Name        *string `json:"name"`
		Description *string `json:"description"`
		Position    *int    `json:"position"`
	}
	if !h.decode(w, r, &body) {
		return
	}
	p, err := h.service.UpdatePhase(r.Context(), actor, r.PathValue("id"), r.PathValue("phaseId"), application.PhasePatch(body))
	h.respond(w, r, http.StatusOK, toPhase(p), err)
}

func (h handler) deletePhase(w http.ResponseWriter, r *http.Request, actor access.Actor) {
	h.noContent(w, r, h.service.DeletePhase(r.Context(), actor, r.PathValue("id"), r.PathValue("phaseId")))
}

func (h handler) listGroups(w http.ResponseWriter, r *http.Request, actor access.Actor) {
	sizes, err := h.service.Groups(r.Context(), actor, r.PathValue("id"))
	h.respond(w, r, http.StatusOK, dataJSON{mapSlice(sizes, func(s application.GroupSize) any {
		return struct {
			groupJSON
			ActiveSubjects int `json:"activeSubjects"`
		}{toGroup(s.Group), s.ActiveSubjects}
	})}, err)
}

func (h handler) createGroup(w http.ResponseWriter, r *http.Request, actor access.Actor) {
	var body struct {
		Name        string `json:"name"`
		Role        string `json:"role"`
		Description string `json:"description"`
		TargetSize  int    `json:"targetSize"`
	}
	if !h.decode(w, r, &body) {
		return
	}
	g, err := h.service.CreateGroup(r.Context(), actor, r.PathValue("id"), application.GroupInput(body))
	h.respond(w, r, http.StatusCreated, toGroup(g), err)
}

func (h handler) updateGroup(w http.ResponseWriter, r *http.Request, actor access.Actor) {
	var body struct {
		Name        *string `json:"name"`
		Role        *string `json:"role"`
		Description *string `json:"description"`
		TargetSize  *int    `json:"targetSize"`
	}
	if !h.decode(w, r, &body) {
		return
	}
	g, err := h.service.UpdateGroup(r.Context(), actor, r.PathValue("id"), r.PathValue("groupId"), application.GroupPatch(body))
	h.respond(w, r, http.StatusOK, toGroup(g), err)
}

func (h handler) deleteGroup(w http.ResponseWriter, r *http.Request, actor access.Actor) {
	h.noContent(w, r, h.service.DeleteGroup(r.Context(), actor, r.PathValue("id"), r.PathValue("groupId")))
}

func (h handler) listEnrollments(w http.ResponseWriter, r *http.Request, actor access.Actor) {
	q := r.URL.Query()
	page, err := h.service.ListEnrollments(r.Context(), actor, r.PathValue("id"), application.EnrollmentQuery{
		SubjectID: q.Get("subjectId"), GroupID: q.Get("groupId"), Order: q.Get("order"),
		Page: httpjson.QueryInt(q.Get("page")), PageSize: httpjson.QueryInt(q.Get("pageSize")),
	})
	h.respond(w, r, http.StatusOK, pageJSON{mapSlice(page.Enrollments, toEnrollment), page.Total, page.Page, page.PageSize}, err)
}

func (h handler) enroll(w http.ResponseWriter, r *http.Request, actor access.Actor) {
	var body struct {
		SubjectID  string    `json:"subjectId"`
		GroupID    string    `json:"groupId"`
		EnrolledAt time.Time `json:"enrolledAt"`
	}
	if !h.decode(w, r, &body) {
		return
	}
	summary, err := h.service.Enroll(r.Context(), actor, r.PathValue("id"), application.EnrollInput(body))
	h.respond(w, r, http.StatusCreated, toEnrollment(summary), err)
}

func (h handler) getEnrollment(w http.ResponseWriter, r *http.Request, actor access.Actor) {
	detail, err := h.service.Enrollment(r.Context(), actor, r.PathValue("id"), r.PathValue("enrollmentId"))
	h.respond(w, r, http.StatusOK, struct {
		enrollmentJSON
		Assignments []assignmentJSON `json:"assignments"`
	}{toEnrollment(application.EnrollmentSummary{Enrollment: detail.Enrollment, CurrentGroupID: detail.CurrentGroupID}), mapSlice(detail.Assignments, toAssignment)}, err)
}

func (h handler) assign(w http.ResponseWriter, r *http.Request, actor access.Actor) {
	var body struct {
		GroupID       string    `json:"groupId"`
		EffectiveFrom time.Time `json:"effectiveFrom"`
	}
	if !h.decode(w, r, &body) {
		return
	}
	a, err := h.service.Assign(r.Context(), actor, r.PathValue("id"), r.PathValue("enrollmentId"), application.AssignInput(body))
	h.respond(w, r, http.StatusCreated, toAssignment(a), err)
}

func (h handler) subjectEnrollments(w http.ResponseWriter, r *http.Request, actor access.Actor) {
	list, err := h.service.SubjectEnrollments(r.Context(), actor, r.PathValue("id"))
	h.respond(w, r, http.StatusOK, dataJSON{mapSlice(list, toEnrollment)}, err)
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
	h.write(w, status, body)
}

func (h handler) write(w http.ResponseWriter, status int, body any) { httpjson.Write(w, status, body) }

func (h handler) noContent(w http.ResponseWriter, r *http.Request, err error) {
	if err != nil {
		h.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
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

type experimentJSON struct {
	ID              string    `json:"id"`
	Code            string    `json:"code"`
	Title           string    `json:"title"`
	Description     *string   `json:"description"`
	RequiresControl bool      `json:"requiresControl"`
	CreatedAt       time.Time `json:"createdAt"`
	UpdatedAt       time.Time `json:"updatedAt"`
}

func toExperiment(e domain.Experiment) experimentJSON {
	return experimentJSON{e.ID, e.Code, e.Title, nullable(e.Description), e.RequiresControl, e.CreatedAt, e.UpdatedAt}
}

type phaseJSON struct {
	ID           string    `json:"id"`
	ExperimentID string    `json:"experimentId"`
	Name         string    `json:"name"`
	Position     int       `json:"position"`
	Description  *string   `json:"description"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

func toPhase(p domain.Phase) phaseJSON {
	return phaseJSON{p.ID, p.ExperimentID, p.Name, p.Position, nullable(p.Description), p.CreatedAt, p.UpdatedAt}
}

type groupJSON struct {
	ID           string    `json:"id"`
	ExperimentID string    `json:"experimentId"`
	Name         string    `json:"name"`
	Role         string    `json:"role"`
	TargetSize   *int      `json:"targetSize"`
	Description  *string   `json:"description"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

func toGroup(g domain.Group) groupJSON {
	out := groupJSON{g.ID, g.ExperimentID, g.Name, string(g.Role), nil, nullable(g.Description), g.CreatedAt, g.UpdatedAt}
	if g.TargetSize > 0 {
		out.TargetSize = &g.TargetSize
	}
	return out
}

type enrollmentJSON struct {
	ID             string    `json:"id"`
	ExperimentID   string    `json:"experimentId"`
	SubjectID      string    `json:"subjectId"`
	EnrolledAt     time.Time `json:"enrolledAt"`
	CurrentGroupID *string   `json:"currentGroupId"`
	CreatedAt      time.Time `json:"createdAt"`
}

func toEnrollment(s application.EnrollmentSummary) enrollmentJSON {
	e := s.Enrollment
	return enrollmentJSON{e.ID, e.ExperimentID, e.SubjectID, e.EnrolledAt, nullable(s.CurrentGroupID), e.CreatedAt}
}

type assignmentJSON struct {
	ID        string     `json:"id"`
	GroupID   string     `json:"groupId"`
	ValidFrom time.Time  `json:"validFrom"`
	ValidTo   *time.Time `json:"validTo"`
	CreatedAt time.Time  `json:"createdAt"`
}

func toAssignment(a domain.Assignment) assignmentJSON {
	return assignmentJSON{a.ID, a.GroupID, a.ValidFrom, a.ValidTo, a.CreatedAt}
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
