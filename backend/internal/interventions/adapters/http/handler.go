// Package interventionshttp exposes disease models, substances, intervention
// plans, weights, conditions and administrations over HTTP.
package interventionshttp

import (
	access "github.com/HappyHackingSpace/Misko/backend/internal/access/domain"
	"github.com/HappyHackingSpace/Misko/backend/internal/interventions/application"
	"github.com/HappyHackingSpace/Misko/backend/internal/interventions/domain"
	"github.com/HappyHackingSpace/Misko/backend/internal/platform/httpjson"
	"log/slog"
	"net/http"
	"net/url"
	"time"
)

var failures = []httpjson.Failure{
	{Err: access.ErrUnauthenticated, Status: http.StatusUnauthorized, Code: "auth.unauthenticated"},
	{Err: access.ErrForbidden, Status: http.StatusForbidden, Code: "auth.forbidden"},
	{Err: application.ErrDiseaseModelNotFound, Status: http.StatusNotFound, Code: "diseaseModel.notFound"},
	{Err: application.ErrSubstanceNotFound, Status: http.StatusNotFound, Code: "substance.notFound"},
	{Err: application.ErrPlanNotFound, Status: http.StatusNotFound, Code: "plan.notFound"},
	{Err: application.ErrWeightNotFound, Status: http.StatusNotFound, Code: "weight.notFound"},
	{Err: application.ErrEnrollmentNotFound, Status: http.StatusNotFound, Code: "enrollment.notFound"},
	{Err: application.ErrSubjectNotFound, Status: http.StatusNotFound, Code: "subject.notFound"},
	{Err: application.ErrExperimentNotFound, Status: http.StatusNotFound, Code: "experiment.notFound"},
	{Err: application.ErrGroupNotFound, Status: http.StatusNotFound, Code: "group.notFound"},
	{Err: application.ErrPhaseNotFound, Status: http.StatusNotFound, Code: "phase.notFound"},
	{Err: application.ErrEnrollmentOtherSubject, Status: http.StatusBadRequest, Code: "condition.enrollmentOtherSubject"},
	{Err: application.ErrNameTaken, Status: http.StatusConflict, Code: "catalog.nameTaken"},
	{Err: application.ErrPlanGroupMismatch, Status: http.StatusBadRequest, Code: "administration.planGroupMismatch"},
	{Err: application.ErrPlanSubstanceMismatch, Status: http.StatusBadRequest, Code: "administration.planSubstanceMismatch"},
	{Err: application.ErrDoseIncomplete, Status: http.StatusBadRequest, Code: "plan.doseIncomplete"},
	{Err: application.ErrNoChanges, Status: http.StatusBadRequest, Code: "common.noChanges"},
	{Err: application.ErrInvalidQuery, Status: http.StatusBadRequest, Code: "common.invalidQuery"},
	{Err: domain.ErrInvalidAmount, Status: http.StatusBadRequest, Code: "dose.invalidAmount"},
	{Err: domain.ErrAmountTooSmall, Status: http.StatusBadRequest, Code: "dose.amountTooSmall"},
	{Err: domain.ErrInvalidUnit, Status: http.StatusBadRequest, Code: "dose.invalidUnit"},
	{Err: domain.ErrInvalidRoute, Status: http.StatusBadRequest, Code: "dose.invalidRoute"},
	{Err: domain.ErrInvalidWeight, Status: http.StatusBadRequest, Code: "weight.invalid"},
	{Err: domain.ErrInvalidName, Status: http.StatusBadRequest, Code: "common.invalidName"},
	{Err: domain.ErrInvalidDescription, Status: http.StatusBadRequest, Code: "common.invalidDescription"},
	{Err: domain.ErrInvalidSchedule, Status: http.StatusBadRequest, Code: "plan.invalidSchedule"},
	{Err: domain.ErrInvalidNotes, Status: http.StatusBadRequest, Code: "common.invalidNotes"},
	{Err: domain.ErrInvalidStatus, Status: http.StatusBadRequest, Code: "condition.invalidStatus"},
	{Err: domain.ErrMissingTime, Status: http.StatusBadRequest, Code: "common.invalidTime"},
	{Err: domain.ErrFutureTime, Status: http.StatusBadRequest, Code: "common.futureTime"},
	{Err: domain.ErrBeforeEnrollment, Status: http.StatusBadRequest, Code: "administration.beforeEnrollment"},
	{Err: domain.ErrWeightRequired, Status: http.StatusBadRequest, Code: "administration.weightRequired"},
	{Err: domain.ErrWeightOtherSubject, Status: http.StatusBadRequest, Code: "administration.weightOtherSubject"},
	{Err: domain.ErrWeightNotRelevant, Status: http.StatusBadRequest, Code: "administration.weightNotRelevant"},
}

type handler struct {
	service      *application.Service
	authenticate func(*http.Request) (access.Actor, error)
	logger       *slog.Logger
}

func Register(mux *http.ServeMux, service *application.Service, authenticate func(*http.Request) (access.Actor, error), logger *slog.Logger) {
	h := handler{service: service, authenticate: authenticate, logger: logger}
	mux.HandleFunc("GET /api/disease-models", h.with(h.listDiseaseModels))
	mux.HandleFunc("POST /api/disease-models", h.with(h.createDiseaseModel))
	mux.HandleFunc("PATCH /api/disease-models/{id}", h.with(h.updateDiseaseModel))
	mux.HandleFunc("GET /api/substances", h.with(h.listSubstances))
	mux.HandleFunc("POST /api/substances", h.with(h.createSubstance))
	mux.HandleFunc("PATCH /api/substances/{id}", h.with(h.updateSubstance))
	mux.HandleFunc("GET /api/experiments/{id}/intervention-plans", h.with(h.listPlans))
	mux.HandleFunc("POST /api/experiments/{id}/intervention-plans", h.with(h.createPlan))
	mux.HandleFunc("PATCH /api/experiments/{id}/intervention-plans/{planId}", h.with(h.updatePlan))
	mux.HandleFunc("GET /api/subjects/{id}/weights", h.with(h.listWeights))
	mux.HandleFunc("POST /api/subjects/{id}/weights", h.with(h.recordWeight))
	mux.HandleFunc("GET /api/subjects/{id}/conditions", h.with(h.subjectConditions))
	mux.HandleFunc("POST /api/subjects/{id}/conditions", h.with(h.recordCondition))
	mux.HandleFunc("GET /api/conditions", h.with(h.listConditions))
	mux.HandleFunc("POST /api/experiments/{id}/enrollments/{enrollmentId}/administrations", h.with(h.recordAdministration))
	mux.HandleFunc("GET /api/subjects/{id}/administrations", h.with(h.subjectAdministrations))
	mux.HandleFunc("GET /api/administrations", h.with(h.listAdministrations))
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

type catalogBody struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type catalogPatchBody struct {
	Name        *string `json:"name"`
	Description *string `json:"description"`
}

func (h handler) listDiseaseModels(w http.ResponseWriter, r *http.Request, actor access.Actor) {
	models, err := h.service.DiseaseModels(r.Context(), actor)
	h.respond(w, r, http.StatusOK, dataJSON{mapSlice(models, diseaseModelJSON)}, err)
}

func (h handler) createDiseaseModel(w http.ResponseWriter, r *http.Request, actor access.Actor) {
	var body catalogBody
	if !h.decode(w, r, &body) {
		return
	}
	m, err := h.service.CreateDiseaseModel(r.Context(), actor, application.CatalogInput(body))
	h.respond(w, r, http.StatusCreated, diseaseModelJSON(m), err)
}

func (h handler) updateDiseaseModel(w http.ResponseWriter, r *http.Request, actor access.Actor) {
	var body catalogPatchBody
	if !h.decode(w, r, &body) {
		return
	}
	m, err := h.service.UpdateDiseaseModel(r.Context(), actor, r.PathValue("id"), application.CatalogPatch(body))
	h.respond(w, r, http.StatusOK, diseaseModelJSON(m), err)
}

func (h handler) listSubstances(w http.ResponseWriter, r *http.Request, actor access.Actor) {
	substances, err := h.service.Substances(r.Context(), actor)
	h.respond(w, r, http.StatusOK, dataJSON{mapSlice(substances, substanceJSON)}, err)
}

func (h handler) createSubstance(w http.ResponseWriter, r *http.Request, actor access.Actor) {
	var body catalogBody
	if !h.decode(w, r, &body) {
		return
	}
	s, err := h.service.CreateSubstance(r.Context(), actor, application.CatalogInput(body))
	h.respond(w, r, http.StatusCreated, substanceJSON(s), err)
}

func (h handler) updateSubstance(w http.ResponseWriter, r *http.Request, actor access.Actor) {
	var body catalogPatchBody
	if !h.decode(w, r, &body) {
		return
	}
	s, err := h.service.UpdateSubstance(r.Context(), actor, r.PathValue("id"), application.CatalogPatch(body))
	h.respond(w, r, http.StatusOK, substanceJSON(s), err)
}

func (h handler) listPlans(w http.ResponseWriter, r *http.Request, actor access.Actor) {
	plans, err := h.service.Plans(r.Context(), actor, r.PathValue("id"))
	h.respond(w, r, http.StatusOK, dataJSON{mapSlice(plans, planJSON)}, err)
}

func (h handler) createPlan(w http.ResponseWriter, r *http.Request, actor access.Actor) {
	var body struct {
		GroupID     string `json:"groupId"`
		PhaseID     string `json:"phaseId"`
		SubstanceID string `json:"substanceId"`
		Amount      string `json:"amount"`
		Unit        string `json:"unit"`
		Route       string `json:"route"`
		Schedule    string `json:"schedule"`
		Notes       string `json:"notes"`
	}
	if !h.decode(w, r, &body) {
		return
	}
	p, err := h.service.CreatePlan(r.Context(), actor, r.PathValue("id"), application.PlanInput(body))
	h.respond(w, r, http.StatusCreated, planJSON(p), err)
}

func (h handler) updatePlan(w http.ResponseWriter, r *http.Request, actor access.Actor) {
	var body struct {
		Amount   *string `json:"amount"`
		Unit     *string `json:"unit"`
		Route    *string `json:"route"`
		Schedule *string `json:"schedule"`
		Notes    *string `json:"notes"`
	}
	if !h.decode(w, r, &body) {
		return
	}
	p, err := h.service.UpdatePlan(r.Context(), actor, r.PathValue("id"), r.PathValue("planId"), application.PlanPatch(body))
	h.respond(w, r, http.StatusOK, planJSON(p), err)
}

func (h handler) listWeights(w http.ResponseWriter, r *http.Request, actor access.Actor) {
	weights, err := h.service.Weights(r.Context(), actor, r.PathValue("id"))
	h.respond(w, r, http.StatusOK, dataJSON{mapSlice(weights, weightJSON)}, err)
}

func (h handler) recordWeight(w http.ResponseWriter, r *http.Request, actor access.Actor) {
	var body struct {
		Grams      string    `json:"grams"`
		MeasuredAt time.Time `json:"measuredAt"`
	}
	if !h.decode(w, r, &body) {
		return
	}
	weight, err := h.service.RecordWeight(r.Context(), actor, r.PathValue("id"), application.WeightInput(body))
	h.respond(w, r, http.StatusCreated, weightJSON(weight), err)
}

func (h handler) subjectConditions(w http.ResponseWriter, r *http.Request, actor access.Actor) {
	history, err := h.service.SubjectConditions(r.Context(), actor, r.PathValue("id"), r.URL.Query().Get("diseaseModelId"))
	h.respond(w, r, http.StatusOK, struct {
		History []conditionJSONBody `json:"history"`
		Current []conditionJSONBody `json:"current"`
	}{mapSlice(history.History, conditionJSON), mapSlice(history.Current, conditionJSON)}, err)
}

func (h handler) recordCondition(w http.ResponseWriter, r *http.Request, actor access.Actor) {
	var body struct {
		DiseaseModelID string    `json:"diseaseModelId"`
		Status         string    `json:"status"`
		EnrollmentID   string    `json:"enrollmentId"`
		Notes          string    `json:"notes"`
		ObservedAt     time.Time `json:"observedAt"`
	}
	if !h.decode(w, r, &body) {
		return
	}
	c, err := h.service.RecordCondition(r.Context(), actor, r.PathValue("id"), application.ConditionInput(body))
	h.respond(w, r, http.StatusCreated, conditionJSON(c), err)
}

func (h handler) listConditions(w http.ResponseWriter, r *http.Request, actor access.Actor) {
	q := r.URL.Query()
	current := q.Get("current")
	if current != "" && current != "true" && current != "false" {
		h.fail(w, r, application.ErrInvalidQuery)
		return
	}
	page, err := h.service.ListConditions(r.Context(), actor, application.ConditionQuery{
		DiseaseModelID: q.Get("diseaseModelId"), Status: q.Get("status"), CurrentOnly: current == "true",
		Page: httpjson.QueryInt(q.Get("page")), PageSize: httpjson.QueryInt(q.Get("pageSize")),
	})
	h.respond(w, r, http.StatusOK, pageJSON{mapSlice(page.Conditions, conditionJSON), page.Total, page.Page, page.PageSize}, err)
}

func (h handler) recordAdministration(w http.ResponseWriter, r *http.Request, actor access.Actor) {
	var body struct {
		SubstanceID string    `json:"substanceId"`
		PlanID      string    `json:"planId"`
		WeightID    string    `json:"weightMeasurementId"`
		Amount      string    `json:"amount"`
		Unit        string    `json:"unit"`
		Route       string    `json:"route"`
		Notes       string    `json:"notes"`
		At          time.Time `json:"administeredAt"`
	}
	if !h.decode(w, r, &body) {
		return
	}
	a, err := h.service.RecordAdministration(r.Context(), actor, r.PathValue("id"), r.PathValue("enrollmentId"), application.AdministrationInput{
		SubstanceID: body.SubstanceID, PlanID: body.PlanID, WeightID: body.WeightID, Amount: body.Amount, Unit: body.Unit,
		Route: body.Route, Notes: body.Notes, AdministeredAt: body.At,
	})
	h.respond(w, r, http.StatusCreated, administrationJSON(a), err)
}

func (h handler) subjectAdministrations(w http.ResponseWriter, r *http.Request, actor access.Actor) {
	q, ok := h.administrationQuery(w, r)
	if !ok {
		return
	}
	page, err := h.service.SubjectAdministrations(r.Context(), actor, r.PathValue("id"), q)
	h.respond(w, r, http.StatusOK, pageJSON{mapSlice(page.Administrations, administrationJSON), page.Total, page.Page, page.PageSize}, err)
}

func (h handler) listAdministrations(w http.ResponseWriter, r *http.Request, actor access.Actor) {
	q, ok := h.administrationQuery(w, r)
	if !ok {
		return
	}
	page, err := h.service.ListAdministrations(r.Context(), actor, q)
	h.respond(w, r, http.StatusOK, pageJSON{mapSlice(page.Administrations, administrationJSON), page.Total, page.Page, page.PageSize}, err)
}

// administrationQuery parses RFC 3339 from/to bounds; to is exclusive.
func (h handler) administrationQuery(w http.ResponseWriter, r *http.Request) (application.AdministrationQuery, bool) {
	values := r.URL.Query()
	q := application.AdministrationQuery{
		SubstanceID: values.Get("substanceId"), ExperimentID: values.Get("experimentId"),
		Page: httpjson.QueryInt(values.Get("page")), PageSize: httpjson.QueryInt(values.Get("pageSize")),
	}
	var ok bool
	if q.From, ok = queryTime(values, "from"); !ok {
		h.fail(w, r, application.ErrInvalidQuery)
		return q, false
	}
	if q.To, ok = queryTime(values, "to"); !ok {
		h.fail(w, r, application.ErrInvalidQuery)
		return q, false
	}
	return q, true
}

func queryTime(values url.Values, key string) (time.Time, bool) {
	raw := values.Get(key)
	if raw == "" {
		return time.Time{}, true
	}
	t, err := time.Parse(time.RFC3339Nano, raw)
	return t, err == nil
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

type catalogJSON struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description *string   `json:"description"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

func diseaseModelJSON(m domain.DiseaseModel) catalogJSON {
	return catalogJSON{m.ID, m.Name, nullable(m.Description), m.CreatedAt, m.UpdatedAt}
}

func substanceJSON(s domain.Substance) catalogJSON {
	return catalogJSON{s.ID, s.Name, nullable(s.Description), s.CreatedAt, s.UpdatedAt}
}

type doseJSON struct {
	Amount string `json:"amount"`
	Unit   string `json:"unit"`
}

func toDose(d domain.Dose) doseJSON { return doseJSON{d.Amount(), string(d.Unit)} }

type planJSONBody struct {
	ID           string    `json:"id"`
	ExperimentID string    `json:"experimentId"`
	GroupID      string    `json:"groupId"`
	PhaseID      *string   `json:"phaseId"`
	SubstanceID  string    `json:"substanceId"`
	Dose         doseJSON  `json:"dose"`
	Route        string    `json:"route"`
	Schedule     string    `json:"schedule"`
	Notes        *string   `json:"notes"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

func planJSON(p domain.Plan) planJSONBody {
	return planJSONBody{p.ID, p.ExperimentID, p.GroupID, nullable(p.PhaseID), p.SubstanceID, toDose(p.Dose), string(p.Route), p.Schedule, nullable(p.Notes), p.CreatedAt, p.UpdatedAt}
}

type weightJSONBody struct {
	ID         string    `json:"id"`
	SubjectID  string    `json:"subjectId"`
	Grams      string    `json:"grams"`
	MeasuredAt time.Time `json:"measuredAt"`
	RecordedBy string    `json:"recordedBy"`
	CreatedAt  time.Time `json:"createdAt"`
}

func weightJSON(w domain.Weight) weightJSONBody {
	return weightJSONBody{w.ID, w.SubjectID, domain.FormatGrams(w.Milligrams), w.MeasuredAt, w.RecordedBy, w.CreatedAt}
}

type conditionJSONBody struct {
	ID               string    `json:"id"`
	SubjectID        string    `json:"subjectId"`
	DiseaseModelID   string    `json:"diseaseModelId"`
	DiseaseModelName string    `json:"diseaseModelName"`
	EnrollmentID     *string   `json:"enrollmentId"`
	Status           string    `json:"status"`
	ObservedAt       time.Time `json:"observedAt"`
	Notes            *string   `json:"notes"`
	RecordedBy       string    `json:"recordedBy"`
	CreatedAt        time.Time `json:"createdAt"`
}

func conditionJSON(c domain.Condition) conditionJSONBody {
	return conditionJSONBody{c.ID, c.SubjectID, c.DiseaseModelID, c.DiseaseModelName, nullable(c.EnrollmentID), string(c.Status), c.ObservedAt, nullable(c.Notes), c.RecordedBy, c.CreatedAt}
}

type administrationJSONBody struct {
	ID                  string    `json:"id"`
	ExperimentID        string    `json:"experimentId"`
	EnrollmentID        string    `json:"enrollmentId"`
	SubjectID           string    `json:"subjectId"`
	SubstanceID         string    `json:"substanceId"`
	SubstanceName       string    `json:"substanceName"`
	PlanID              *string   `json:"planId"`
	WeightMeasurementID *string   `json:"weightMeasurementId"`
	BodyWeightGrams     *string   `json:"bodyWeightGrams"`
	Dose                doseJSON  `json:"dose"`
	AbsoluteDose        *doseJSON `json:"absoluteDose"`
	Route               string    `json:"route"`
	AdministeredAt      time.Time `json:"administeredAt"`
	Notes               *string   `json:"notes"`
	RecordedBy          string    `json:"recordedBy"`
	CreatedAt           time.Time `json:"createdAt"`
}

func administrationJSON(a domain.Administration) administrationJSONBody {
	out := administrationJSONBody{
		ID: a.ID, ExperimentID: a.ExperimentID, EnrollmentID: a.EnrollmentID, SubjectID: a.SubjectID,
		SubstanceID: a.SubstanceID, SubstanceName: a.SubstanceName, PlanID: nullable(a.PlanID), WeightMeasurementID: nullable(a.WeightID),
		Dose: toDose(a.Dose), Route: string(a.Route), AdministeredAt: a.AdministeredAt, Notes: nullable(a.Notes), RecordedBy: a.RecordedBy, CreatedAt: a.CreatedAt,
	}
	if a.BodyMilligrams > 0 {
		out.BodyWeightGrams = nullable(domain.FormatGrams(a.BodyMilligrams))
	}
	if abs, ok := a.AbsoluteDose(); ok {
		d := toDose(abs)
		out.AbsoluteDose = &d
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
