// Package experimentshttp exposes the study design over HTTP: experiments and
// their lifecycle, groups, regimen items, timepoints, endpoints and
// allocations.
package experimentshttp

import (
	"errors"
	"log/slog"
	"net/http"
	"time"

	access "github.com/HappyHackingSpace/Misko/backend/internal/access/domain"
	"github.com/HappyHackingSpace/Misko/backend/internal/experiments/application"
	"github.com/HappyHackingSpace/Misko/backend/internal/experiments/domain"
	"github.com/HappyHackingSpace/Misko/backend/internal/platform/httpjson"
)

var failures = []httpjson.Failure{
	{Err: access.ErrUnauthenticated, Status: http.StatusUnauthorized, Code: "auth.unauthenticated"},
	{Err: access.ErrForbidden, Status: http.StatusForbidden, Code: "auth.forbidden"},

	{Err: application.ErrExperimentNotFound, Status: http.StatusNotFound, Code: "experiment.notFound"},
	{Err: application.ErrGroupNotFound, Status: http.StatusNotFound, Code: "group.notFound"},
	{Err: application.ErrRegimenItemNotFound, Status: http.StatusNotFound, Code: "regimen.notFound"},
	{Err: application.ErrTimepointNotFound, Status: http.StatusNotFound, Code: "timepoint.notFound"},
	{Err: application.ErrEndpointNotFound, Status: http.StatusNotFound, Code: "endpoint.notFound"},
	{Err: application.ErrAllocationNotFound, Status: http.StatusNotFound, Code: "allocation.notFound"},

	{Err: application.ErrCodeTaken, Status: http.StatusConflict, Code: "experiment.codeTaken"},
	{Err: application.ErrGroupCodeTaken, Status: http.StatusConflict, Code: "group.codeTaken"},
	{Err: application.ErrGroupNameTaken, Status: http.StatusConflict, Code: "group.nameTaken"},
	{Err: application.ErrGroupInUse, Status: http.StatusConflict, Code: "group.inUse"},
	{Err: application.ErrTimepointConflict, Status: http.StatusConflict, Code: "timepoint.conflict"},
	{Err: application.ErrTimepointInUse, Status: http.StatusConflict, Code: "timepoint.inUse"},
	{Err: application.ErrTreatmentItemTaken, Status: http.StatusConflict, Code: "regimen.treatmentTaken"},
	{Err: application.ErrEndpointDuplicate, Status: http.StatusConflict, Code: "endpoint.duplicate"},
	{Err: application.ErrPrimaryEndpointTaken, Status: http.StatusConflict, Code: "endpoint.primaryTaken"},
	{Err: application.ErrAlreadyAllocated, Status: http.StatusConflict, Code: "allocation.subjectAllocated"},
	{Err: application.ErrNotEnoughPlaces, Status: http.StatusConflict, Code: "allocation.notEnoughPlaces"},
	{Err: application.ErrDesignFrozen, Status: http.StatusConflict, Code: "experiment.designFrozen"},
	{Err: application.ErrAllocationFrozen, Status: http.StatusConflict, Code: "allocation.frozen"},
	{Err: application.ErrNotDraft, Status: http.StatusConflict, Code: "experiment.notDraft"},
	{Err: application.ErrNotRunning, Status: http.StatusConflict, Code: "experiment.notRunning"},
	{Err: application.ErrExperimentCompleted, Status: http.StatusConflict, Code: "experiment.completed"},
	{Err: application.ErrOpenTests, Status: http.StatusConflict, Code: "experiment.testsOpen"},
	{Err: application.ErrInvalidTransition, Status: http.StatusConflict, Code: "experiment.invalidTransition"},
	{Err: domain.ErrAlreadyExcluded, Status: http.StatusConflict, Code: "allocation.alreadyExcluded"},

	{Err: application.ErrDiseaseModelNotFound, Status: http.StatusBadRequest, Code: "group.unknownDiseaseModel"},
	{Err: application.ErrSubstanceNotFound, Status: http.StatusBadRequest, Code: "regimen.unknownSubstance"},
	{Err: application.ErrSubjectNotFound, Status: http.StatusBadRequest, Code: "allocation.unknownSubject"},
	{Err: application.ErrGroupNotInExperiment, Status: http.StatusBadRequest, Code: "group.notInExperiment"},
	{Err: application.ErrTimepointNotInExperiment, Status: http.StatusBadRequest, Code: "timepoint.notInExperiment"},
	{Err: application.ErrMetricUnavailable, Status: http.StatusBadRequest, Code: "endpoint.metricUnavailable"},
	{Err: application.ErrInvalidDayZero, Status: http.StatusBadRequest, Code: "experiment.invalidDayZero"},
	{Err: application.ErrInvalidSubjects, Status: http.StatusBadRequest, Code: "allocation.invalidSubjects"},
	{Err: application.ErrInvalidGroups, Status: http.StatusBadRequest, Code: "allocation.invalidGroups"},
	{Err: application.ErrNoChanges, Status: http.StatusBadRequest, Code: "common.noChanges"},
	{Err: application.ErrInvalidQuery, Status: http.StatusBadRequest, Code: "common.invalidQuery"},

	{Err: domain.ErrInvalidCode, Status: http.StatusBadRequest, Code: "experiment.invalidCode"},
	{Err: domain.ErrInvalidTitle, Status: http.StatusBadRequest, Code: "experiment.invalidTitle"},
	{Err: domain.ErrInvalidDescription, Status: http.StatusBadRequest, Code: "common.invalidDescription"},
	{Err: domain.ErrInvalidHypothesis, Status: http.StatusBadRequest, Code: "experiment.invalidHypothesis"},
	{Err: domain.ErrInvalidRandomisation, Status: http.StatusBadRequest, Code: "experiment.invalidRandomisation"},
	{Err: domain.ErrInvalidGroupCode, Status: http.StatusBadRequest, Code: "group.invalidCode"},
	{Err: domain.ErrInvalidName, Status: http.StatusBadRequest, Code: "common.invalidName"},
	{Err: domain.ErrInvalidRole, Status: http.StatusBadRequest, Code: "group.invalidRole"},
	{Err: domain.ErrInvalidTargetSize, Status: http.StatusBadRequest, Code: "group.invalidTargetSize"},
	{Err: domain.ErrDiseaseModelRequired, Status: http.StatusBadRequest, Code: "group.diseaseModelRequired"},
	{Err: domain.ErrDiseaseModelNotAllowed, Status: http.StatusBadRequest, Code: "group.diseaseModelNotAllowed"},
	{Err: domain.ErrInvalidPurpose, Status: http.StatusBadRequest, Code: "regimen.invalidPurpose"},
	{Err: domain.ErrInvalidAmount, Status: http.StatusBadRequest, Code: "dose.invalidAmount"},
	{Err: domain.ErrInvalidUnit, Status: http.StatusBadRequest, Code: "dose.invalidUnit"},
	{Err: domain.ErrInvalidRoute, Status: http.StatusBadRequest, Code: "dose.invalidRoute"},
	{Err: domain.ErrInvalidFrequency, Status: http.StatusBadRequest, Code: "regimen.invalidFrequency"},
	{Err: domain.ErrInvalidInterval, Status: http.StatusBadRequest, Code: "regimen.invalidInterval"},
	{Err: domain.ErrInvalidDays, Status: http.StatusBadRequest, Code: "regimen.invalidDays"},
	{Err: domain.ErrInvalidStudyDay, Status: http.StatusBadRequest, Code: "timepoint.invalidStudyDay"},
	{Err: domain.ErrInvalidPosition, Status: http.StatusBadRequest, Code: "timepoint.invalidPosition"},
	{Err: domain.ErrInvalidParadigm, Status: http.StatusBadRequest, Code: "endpoint.invalidParadigm"},
	{Err: domain.ErrInvalidMetric, Status: http.StatusBadRequest, Code: "endpoint.invalidMetric"},
	{Err: domain.ErrInvalidKind, Status: http.StatusBadRequest, Code: "endpoint.invalidKind"},
	{Err: domain.ErrInvalidDirection, Status: http.StatusBadRequest, Code: "endpoint.invalidDirection"},
	{Err: domain.ErrBaselineSameTimepoint, Status: http.StatusBadRequest, Code: "endpoint.baselineSameTimepoint"},
	{Err: domain.ErrInvalidCage, Status: http.StatusBadRequest, Code: "allocation.invalidCage"},
	{Err: domain.ErrInvalidExclusionReason, Status: http.StatusBadRequest, Code: "allocation.invalidExclusionReason"},
	{Err: domain.ErrInvalidExclusionNote, Status: http.StatusBadRequest, Code: "allocation.invalidExclusionNote"},
	{Err: domain.ErrExclusionBeforeAllocation, Status: http.StatusBadRequest, Code: "allocation.exclusionBeforeAllocation"},
	{Err: domain.ErrFutureTime, Status: http.StatusBadRequest, Code: "common.futureTime"},
}

type handler struct {
	service      *application.Service
	authenticate func(*http.Request) (access.Actor, error)
	logger       *slog.Logger
}

func Register(mux *http.ServeMux, service *application.Service, authenticate func(*http.Request) (access.Actor, error), logger *slog.Logger) {
	h := handler{service: service, authenticate: authenticate, logger: logger}
	const e = "/api/experiments/{id}"
	mux.HandleFunc("GET /api/experiments", h.with(h.listExperiments))
	mux.HandleFunc("POST /api/experiments", h.with(h.createExperiment))
	mux.HandleFunc("GET "+e, h.with(h.getExperiment))
	mux.HandleFunc("PATCH "+e, h.with(h.updateExperiment))
	mux.HandleFunc("GET "+e+"/readiness", h.with(h.readiness))
	mux.HandleFunc("POST "+e+"/start", h.with(h.start))
	mux.HandleFunc("POST "+e+"/complete", h.with(h.complete))
	mux.HandleFunc("GET "+e+"/metric-options", h.with(h.metricOptions))

	mux.HandleFunc("GET "+e+"/groups", h.with(h.listGroups))
	mux.HandleFunc("POST "+e+"/groups", h.with(h.createGroup))
	mux.HandleFunc("PATCH "+e+"/groups/{groupId}", h.with(h.updateGroup))
	mux.HandleFunc("DELETE "+e+"/groups/{groupId}", h.with(h.deleteGroup))

	mux.HandleFunc("GET "+e+"/regimen-items", h.with(h.listRegimenItems))
	mux.HandleFunc("POST "+e+"/regimen-items", h.with(h.createRegimenItem))
	mux.HandleFunc("PATCH "+e+"/regimen-items/{itemId}", h.with(h.updateRegimenItem))
	mux.HandleFunc("DELETE "+e+"/regimen-items/{itemId}", h.with(h.deleteRegimenItem))

	mux.HandleFunc("GET "+e+"/timepoints", h.with(h.listTimepoints))
	mux.HandleFunc("POST "+e+"/timepoints", h.with(h.createTimepoint))
	mux.HandleFunc("PATCH "+e+"/timepoints/{timepointId}", h.with(h.updateTimepoint))
	mux.HandleFunc("DELETE "+e+"/timepoints/{timepointId}", h.with(h.deleteTimepoint))

	mux.HandleFunc("GET "+e+"/endpoints", h.with(h.listEndpoints))
	mux.HandleFunc("POST "+e+"/endpoints", h.with(h.createEndpoint))
	mux.HandleFunc("PATCH "+e+"/endpoints/{endpointId}", h.with(h.updateEndpoint))
	mux.HandleFunc("DELETE "+e+"/endpoints/{endpointId}", h.with(h.deleteEndpoint))

	mux.HandleFunc("GET "+e+"/allocations", h.with(h.listAllocations))
	mux.HandleFunc("POST "+e+"/allocations", h.with(h.allocate))
	mux.HandleFunc("POST "+e+"/allocations/randomise", h.with(h.randomise))
	mux.HandleFunc("PATCH "+e+"/allocations/{allocationId}", h.with(h.updateAllocation))
	mux.HandleFunc("DELETE "+e+"/allocations/{allocationId}", h.with(h.removeAllocation))
	mux.HandleFunc("POST "+e+"/allocations/{allocationId}/exclusion", h.with(h.exclude))
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

// Experiments and lifecycle

func (h handler) listExperiments(w http.ResponseWriter, r *http.Request, actor access.Actor) {
	q := r.URL.Query()
	page, err := h.service.ListExperiments(r.Context(), actor, application.ExperimentQuery{
		Search: q.Get("search"), Status: q.Get("status"), Sort: q.Get("sort"), Order: q.Get("order"),
		Page: httpjson.QueryInt(q.Get("page")), PageSize: httpjson.QueryInt(q.Get("pageSize")),
	})
	h.respond(w, r, http.StatusOK, pageJSON{mapSlice(page.Experiments, toExperiment), page.Total, page.Page, page.PageSize}, err)
}

func (h handler) createExperiment(w http.ResponseWriter, r *http.Request, actor access.Actor) {
	var body struct {
		Code          string `json:"code"`
		Title         string `json:"title"`
		Description   string `json:"description"`
		Hypothesis    string `json:"hypothesis"`
		Randomisation string `json:"randomisation"`
		Blinded       bool   `json:"blinded"`
	}
	if !h.decode(w, r, &body) {
		return
	}
	e, err := h.service.CreateExperiment(r.Context(), actor, domain.ExperimentFields(body))
	h.respond(w, r, http.StatusCreated, toExperiment(e), err)
}

func (h handler) getExperiment(w http.ResponseWriter, r *http.Request, actor access.Actor) {
	e, err := h.service.Experiment(r.Context(), actor, r.PathValue("id"))
	h.respond(w, r, http.StatusOK, toExperiment(e), err)
}

func (h handler) updateExperiment(w http.ResponseWriter, r *http.Request, actor access.Actor) {
	var body struct {
		Code          *string `json:"code"`
		Title         *string `json:"title"`
		Description   *string `json:"description"`
		Hypothesis    *string `json:"hypothesis"`
		Randomisation *string `json:"randomisation"`
		Blinded       *bool   `json:"blinded"`
	}
	if !h.decode(w, r, &body) {
		return
	}
	e, err := h.service.UpdateExperiment(r.Context(), actor, r.PathValue("id"), application.ExperimentPatch(body))
	h.respond(w, r, http.StatusOK, toExperiment(e), err)
}

func (h handler) readiness(w http.ResponseWriter, r *http.Request, actor access.Actor) {
	result, err := h.service.Readiness(r.Context(), actor, r.PathValue("id"))
	h.respond(w, r, http.StatusOK, readinessJSON{string(result.Status), len(result.Problems) == 0, mapSlice(result.Problems, toProblem)}, err)
}

func (h handler) start(w http.ResponseWriter, r *http.Request, actor access.Actor) {
	var body struct {
		DayZero string `json:"dayZero"`
	}
	if !h.decode(w, r, &body) {
		return
	}
	e, err := h.service.Start(r.Context(), actor, r.PathValue("id"), body.DayZero)
	h.respond(w, r, http.StatusOK, toExperiment(e), err)
}

func (h handler) complete(w http.ResponseWriter, r *http.Request, actor access.Actor) {
	e, err := h.service.Complete(r.Context(), actor, r.PathValue("id"))
	h.respond(w, r, http.StatusOK, toExperiment(e), err)
}

func (h handler) metricOptions(w http.ResponseWriter, r *http.Request, actor access.Actor) {
	options, err := h.service.MetricOptions(r.Context(), actor, r.PathValue("id"))
	h.respond(w, r, http.StatusOK, dataJSON{mapSlice(options, func(o application.MetricOption) metricOptionJSON {
		return metricOptionJSON{o.Paradigm.Key, o.Paradigm.Version, o.Metrics}
	})}, err)
}

// Groups

func (h handler) listGroups(w http.ResponseWriter, r *http.Request, actor access.Actor) {
	fills, err := h.service.Groups(r.Context(), actor, r.PathValue("id"))
	h.respond(w, r, http.StatusOK, dataJSON{mapSlice(fills, toGroupFill)}, err)
}

func (h handler) createGroup(w http.ResponseWriter, r *http.Request, actor access.Actor) {
	var body struct {
		Code           string `json:"code"`
		Name           string `json:"name"`
		Role           string `json:"role"`
		DiseaseModelID string `json:"diseaseModelId"`
		Description    string `json:"description"`
		TargetSize     int    `json:"targetSize"`
	}
	if !h.decode(w, r, &body) {
		return
	}
	g, err := h.service.CreateGroup(r.Context(), actor, r.PathValue("id"), domain.GroupFields(body))
	h.respond(w, r, http.StatusCreated, toGroup(g), err)
}

func (h handler) updateGroup(w http.ResponseWriter, r *http.Request, actor access.Actor) {
	var body struct {
		Code           *string `json:"code"`
		Name           *string `json:"name"`
		Role           *string `json:"role"`
		DiseaseModelID *string `json:"diseaseModelId"`
		Description    *string `json:"description"`
		TargetSize     *int    `json:"targetSize"`
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

// Regimen items

func (h handler) listRegimenItems(w http.ResponseWriter, r *http.Request, actor access.Actor) {
	items, err := h.service.RegimenItems(r.Context(), actor, r.PathValue("id"))
	h.respond(w, r, http.StatusOK, dataJSON{mapSlice(items, toRegimenItem)}, err)
}

func (h handler) createRegimenItem(w http.ResponseWriter, r *http.Request, actor access.Actor) {
	var body struct {
		GroupID      string `json:"groupId"`
		Purpose      string `json:"purpose"`
		SubstanceID  string `json:"substanceId"`
		Amount       string `json:"amount"`
		Unit         string `json:"unit"`
		Route        string `json:"route"`
		Frequency    string `json:"frequency"`
		IntervalDays int    `json:"intervalDays"`
		StartDay     int    `json:"startDay"`
		EndDay       int    `json:"endDay"`
	}
	if !h.decode(w, r, &body) {
		return
	}
	item, err := h.service.CreateRegimenItem(r.Context(), actor, r.PathValue("id"), domain.RegimenFields(body))
	h.respond(w, r, http.StatusCreated, toRegimenItem(item), err)
}

func (h handler) updateRegimenItem(w http.ResponseWriter, r *http.Request, actor access.Actor) {
	var body struct {
		GroupID      *string `json:"groupId"`
		Purpose      *string `json:"purpose"`
		SubstanceID  *string `json:"substanceId"`
		Amount       *string `json:"amount"`
		Unit         *string `json:"unit"`
		Route        *string `json:"route"`
		Frequency    *string `json:"frequency"`
		IntervalDays *int    `json:"intervalDays"`
		StartDay     *int    `json:"startDay"`
		EndDay       *int    `json:"endDay"`
	}
	if !h.decode(w, r, &body) {
		return
	}
	item, err := h.service.UpdateRegimenItem(r.Context(), actor, r.PathValue("id"), r.PathValue("itemId"), application.RegimenPatch(body))
	h.respond(w, r, http.StatusOK, toRegimenItem(item), err)
}

func (h handler) deleteRegimenItem(w http.ResponseWriter, r *http.Request, actor access.Actor) {
	h.noContent(w, r, h.service.DeleteRegimenItem(r.Context(), actor, r.PathValue("id"), r.PathValue("itemId")))
}

// Timepoints

func (h handler) listTimepoints(w http.ResponseWriter, r *http.Request, actor access.Actor) {
	timepoints, err := h.service.Timepoints(r.Context(), actor, r.PathValue("id"))
	h.respond(w, r, http.StatusOK, dataJSON{mapSlice(timepoints, toTimepoint)}, err)
}

func (h handler) createTimepoint(w http.ResponseWriter, r *http.Request, actor access.Actor) {
	var body struct {
		Name     string `json:"name"`
		StudyDay int    `json:"studyDay"`
		Position int    `json:"position"`
	}
	if !h.decode(w, r, &body) {
		return
	}
	t, err := h.service.CreateTimepoint(r.Context(), actor, r.PathValue("id"), domain.TimepointFields(body))
	h.respond(w, r, http.StatusCreated, toTimepoint(t), err)
}

func (h handler) updateTimepoint(w http.ResponseWriter, r *http.Request, actor access.Actor) {
	var body struct {
		Name     *string `json:"name"`
		StudyDay *int    `json:"studyDay"`
		Position *int    `json:"position"`
	}
	if !h.decode(w, r, &body) {
		return
	}
	t, err := h.service.UpdateTimepoint(r.Context(), actor, r.PathValue("id"), r.PathValue("timepointId"), application.TimepointPatch(body))
	h.respond(w, r, http.StatusOK, toTimepoint(t), err)
}

func (h handler) deleteTimepoint(w http.ResponseWriter, r *http.Request, actor access.Actor) {
	h.noContent(w, r, h.service.DeleteTimepoint(r.Context(), actor, r.PathValue("id"), r.PathValue("timepointId")))
}

// Endpoints

func (h handler) listEndpoints(w http.ResponseWriter, r *http.Request, actor access.Actor) {
	endpoints, err := h.service.Endpoints(r.Context(), actor, r.PathValue("id"))
	h.respond(w, r, http.StatusOK, dataJSON{mapSlice(endpoints, toEndpoint)}, err)
}

func (h handler) createEndpoint(w http.ResponseWriter, r *http.Request, actor access.Actor) {
	var body struct {
		TimepointID         string `json:"timepointId"`
		BaselineTimepointID string `json:"baselineTimepointId"`
		ParadigmKey         string `json:"paradigmKey"`
		MetricKey           string `json:"metricKey"`
		Kind                string `json:"kind"`
		Direction           string `json:"direction"`
	}
	if !h.decode(w, r, &body) {
		return
	}
	e, err := h.service.CreateEndpoint(r.Context(), actor, r.PathValue("id"), domain.EndpointFields(body))
	h.respond(w, r, http.StatusCreated, toEndpoint(e), err)
}

func (h handler) updateEndpoint(w http.ResponseWriter, r *http.Request, actor access.Actor) {
	var body struct {
		TimepointID         *string `json:"timepointId"`
		BaselineTimepointID *string `json:"baselineTimepointId"`
		ParadigmKey         *string `json:"paradigmKey"`
		MetricKey           *string `json:"metricKey"`
		Kind                *string `json:"kind"`
		Direction           *string `json:"direction"`
	}
	if !h.decode(w, r, &body) {
		return
	}
	e, err := h.service.UpdateEndpoint(r.Context(), actor, r.PathValue("id"), r.PathValue("endpointId"), application.EndpointPatch(body))
	h.respond(w, r, http.StatusOK, toEndpoint(e), err)
}

func (h handler) deleteEndpoint(w http.ResponseWriter, r *http.Request, actor access.Actor) {
	h.noContent(w, r, h.service.DeleteEndpoint(r.Context(), actor, r.PathValue("id"), r.PathValue("endpointId")))
}

// Allocations

func (h handler) listAllocations(w http.ResponseWriter, r *http.Request, actor access.Actor) {
	q := r.URL.Query()
	list, err := h.service.Allocations(r.Context(), actor, r.PathValue("id"), application.AllocationQuery{GroupID: q.Get("groupId"), Excluded: q.Get("excluded")})
	h.respond(w, r, http.StatusOK, struct {
		Data   []allocationJSON `json:"data"`
		Groups []groupFillJSON  `json:"groups"`
	}{mapSlice(list.Allocations, func(v application.AllocationView) allocationJSON {
		out := toAllocation(v.Allocation)
		out.SubjectCode = &v.SubjectCode
		return out
	}), mapSlice(list.Groups, toGroupFill)}, err)
}

func (h handler) allocate(w http.ResponseWriter, r *http.Request, actor access.Actor) {
	var body struct {
		SubjectID   string     `json:"subjectId"`
		GroupID     string     `json:"groupId"`
		Cage        string     `json:"cage"`
		AllocatedAt *time.Time `json:"allocatedAt"`
	}
	if !h.decode(w, r, &body) {
		return
	}
	in := application.AllocateInput{SubjectID: body.SubjectID, GroupID: body.GroupID, Cage: body.Cage}
	if body.AllocatedAt != nil {
		in.AllocatedAt = *body.AllocatedAt
	}
	a, err := h.service.Allocate(r.Context(), actor, r.PathValue("id"), in)
	h.respond(w, r, http.StatusCreated, toAllocation(a), err)
}

func (h handler) randomise(w http.ResponseWriter, r *http.Request, actor access.Actor) {
	var body struct {
		SubjectIDs []string `json:"subjectIds"`
		GroupIDs   []string `json:"groupIds"`
		Cage       string   `json:"cage"`
	}
	if !h.decode(w, r, &body) {
		return
	}
	created, err := h.service.Randomise(r.Context(), actor, r.PathValue("id"), application.RandomiseInput(body))
	h.respond(w, r, http.StatusCreated, dataJSON{mapSlice(created, toAllocation)}, err)
}

func (h handler) updateAllocation(w http.ResponseWriter, r *http.Request, actor access.Actor) {
	var body struct {
		GroupID *string `json:"groupId"`
		Cage    *string `json:"cage"`
	}
	if !h.decode(w, r, &body) {
		return
	}
	a, err := h.service.UpdateAllocation(r.Context(), actor, r.PathValue("id"), r.PathValue("allocationId"), application.AllocationPatch(body))
	h.respond(w, r, http.StatusOK, toAllocation(a), err)
}

func (h handler) removeAllocation(w http.ResponseWriter, r *http.Request, actor access.Actor) {
	h.noContent(w, r, h.service.RemoveAllocation(r.Context(), actor, r.PathValue("id"), r.PathValue("allocationId")))
}

func (h handler) exclude(w http.ResponseWriter, r *http.Request, actor access.Actor) {
	var body struct {
		Reason     string     `json:"reason"`
		Note       string     `json:"note"`
		ExcludedAt *time.Time `json:"excludedAt"`
	}
	if !h.decode(w, r, &body) {
		return
	}
	in := application.ExcludeInput{Reason: body.Reason, Note: body.Note}
	if body.ExcludedAt != nil {
		in.ExcludedAt = *body.ExcludedAt
	}
	a, err := h.service.Exclude(r.Context(), actor, r.PathValue("id"), r.PathValue("allocationId"), in)
	h.respond(w, r, http.StatusOK, toAllocation(a), err)
}

// Plumbing

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

func (h handler) noContent(w http.ResponseWriter, r *http.Request, err error) {
	if err != nil {
		h.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// fail answers an unmet start check with 422 and the list of unmet rules;
// every other error goes through the failures table.
func (h handler) fail(w http.ResponseWriter, r *http.Request, err error) {
	var notReady *application.NotReadyError
	if errors.As(err, &notReady) {
		httpjson.Write(w, http.StatusUnprocessableEntity, struct {
			Error string        `json:"error"`
			Code  string        `json:"code"`
			Unmet []problemJSON `json:"unmet"`
		}{application.ErrNotReady.Error(), "experiment.notReady", mapSlice(notReady.Problems, toProblem)})
		return
	}
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
	ID            string     `json:"id"`
	Code          string     `json:"code"`
	Title         string     `json:"title"`
	Description   *string    `json:"description"`
	Hypothesis    *string    `json:"hypothesis"`
	Randomisation *string    `json:"randomisation"`
	Blinded       bool       `json:"blinded"`
	Status        string     `json:"status"`
	DayZero       *string    `json:"dayZero"`
	StartedAt     *time.Time `json:"startedAt"`
	CompletedAt   *time.Time `json:"completedAt"`
	CreatedAt     time.Time  `json:"createdAt"`
	UpdatedAt     time.Time  `json:"updatedAt"`
}

func toExperiment(e domain.Experiment) experimentJSON {
	out := experimentJSON{e.ID, e.Code, e.Title, nullable(e.Description), nullable(e.Hypothesis), nullable(e.Randomisation), e.Blinded,
		string(e.Status), nil, e.StartedAt, e.CompletedAt, e.CreatedAt, e.UpdatedAt}
	if e.DayZero != nil {
		day := e.DayZero.Format(time.DateOnly)
		out.DayZero = &day
	}
	return out
}

type problemJSON struct {
	Rule       string  `json:"rule"`
	GroupID    *string `json:"groupId"`
	EndpointID *string `json:"endpointId"`
}

func toProblem(p domain.Problem) problemJSON {
	return problemJSON{p.Rule, nullable(p.GroupID), nullable(p.EndpointID)}
}

type readinessJSON struct {
	Status string        `json:"status"`
	Ready  bool          `json:"ready"`
	Unmet  []problemJSON `json:"unmet"`
}

type metricOptionJSON struct {
	ParadigmKey     string   `json:"paradigmKey"`
	ParadigmVersion int      `json:"paradigmVersion"`
	Metrics         []string `json:"metrics"`
}

type groupJSON struct {
	ID             string    `json:"id"`
	ExperimentID   string    `json:"experimentId"`
	Code           string    `json:"code"`
	Name           string    `json:"name"`
	Role           string    `json:"role"`
	DiseaseModelID *string   `json:"diseaseModelId"`
	TargetSize     *int      `json:"targetSize"`
	Description    *string   `json:"description"`
	CreatedAt      time.Time `json:"createdAt"`
	UpdatedAt      time.Time `json:"updatedAt"`
}

func toGroup(g domain.Group) groupJSON {
	out := groupJSON{g.ID, g.ExperimentID, g.Code, g.Name, string(g.Role), nullable(g.DiseaseModelID), nil, nullable(g.Description), g.CreatedAt, g.UpdatedAt}
	if g.TargetSize > 0 {
		out.TargetSize = &g.TargetSize
	}
	return out
}

type groupFillJSON struct {
	groupJSON
	Allocated int `json:"allocated"`
	Excluded  int `json:"excluded"`
}

func toGroupFill(f application.GroupFill) groupFillJSON {
	return groupFillJSON{toGroup(f.Group), f.Allocated, f.Excluded}
}

type regimenItemJSON struct {
	ID           string    `json:"id"`
	ExperimentID string    `json:"experimentId"`
	GroupID      string    `json:"groupId"`
	Purpose      string    `json:"purpose"`
	SubstanceID  string    `json:"substanceId"`
	Amount       string    `json:"amount"`
	Unit         string    `json:"unit"`
	Route        string    `json:"route"`
	Frequency    string    `json:"frequency"`
	IntervalDays *int      `json:"intervalDays"`
	StartDay     int       `json:"startDay"`
	EndDay       int       `json:"endDay"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

func toRegimenItem(r domain.RegimenItem) regimenItemJSON {
	out := regimenItemJSON{r.ID, r.ExperimentID, r.GroupID, string(r.Purpose), r.SubstanceID, r.Amount(), r.Unit, r.Route, string(r.Frequency),
		nil, r.StartDay, r.EndDay, r.CreatedAt, r.UpdatedAt}
	if r.IntervalDays > 0 {
		out.IntervalDays = &r.IntervalDays
	}
	return out
}

type timepointJSON struct {
	ID           string    `json:"id"`
	ExperimentID string    `json:"experimentId"`
	Name         string    `json:"name"`
	StudyDay     int       `json:"studyDay"`
	Position     int       `json:"position"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

func toTimepoint(t domain.Timepoint) timepointJSON {
	return timepointJSON{t.ID, t.ExperimentID, t.Name, t.StudyDay, t.Position, t.CreatedAt, t.UpdatedAt}
}

type endpointJSON struct {
	ID                  string    `json:"id"`
	ExperimentID        string    `json:"experimentId"`
	TimepointID         string    `json:"timepointId"`
	BaselineTimepointID *string   `json:"baselineTimepointId"`
	ParadigmKey         string    `json:"paradigmKey"`
	MetricKey           string    `json:"metricKey"`
	Kind                string    `json:"kind"`
	Direction           string    `json:"direction"`
	CreatedAt           time.Time `json:"createdAt"`
	UpdatedAt           time.Time `json:"updatedAt"`
}

func toEndpoint(e domain.Endpoint) endpointJSON {
	return endpointJSON{e.ID, e.ExperimentID, e.TimepointID, nullable(e.BaselineTimepointID), e.ParadigmKey, e.MetricKey, string(e.Kind),
		string(e.Direction), e.CreatedAt, e.UpdatedAt}
}

type allocationJSON struct {
	ID              string     `json:"id"`
	ExperimentID    string     `json:"experimentId"`
	GroupID         string     `json:"groupId"`
	SubjectID       string     `json:"subjectId"`
	SubjectCode     *string    `json:"subjectCode,omitempty"`
	AllocatedAt     time.Time  `json:"allocatedAt"`
	Cage            *string    `json:"cage"`
	Excluded        bool       `json:"excluded"`
	ExcludedAt      *time.Time `json:"excludedAt"`
	ExclusionReason *string    `json:"exclusionReason"`
	ExclusionNote   *string    `json:"exclusionNote"`
	CreatedBy       string     `json:"createdBy"`
	CreatedAt       time.Time  `json:"createdAt"`
}

func toAllocation(a domain.Allocation) allocationJSON {
	return allocationJSON{ID: a.ID, ExperimentID: a.ExperimentID, GroupID: a.GroupID, SubjectID: a.SubjectID, AllocatedAt: a.AllocatedAt,
		Cage: nullable(a.Cage), Excluded: a.Excluded(), ExcludedAt: a.ExcludedAt, ExclusionReason: nullable(string(a.ExclusionReason)),
		ExclusionNote: nullable(a.ExclusionNote), CreatedBy: a.CreatedBy, CreatedAt: a.CreatedAt}
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
