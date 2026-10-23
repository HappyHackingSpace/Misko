// Package dosinghttp exposes administrations, the daily due-dose list and
// induction outcomes over HTTP.
package dosinghttp

import (
	access "github.com/HappyHackingSpace/Misko/backend/internal/access/domain"
	"github.com/HappyHackingSpace/Misko/backend/internal/dosing/application"
	"github.com/HappyHackingSpace/Misko/backend/internal/dosing/domain"
	"github.com/HappyHackingSpace/Misko/backend/internal/platform/httpjson"
	"log/slog"
	"net/http"
	"net/url"
	"time"
)

var failures = []httpjson.Failure{
	{Err: access.ErrUnauthenticated, Status: http.StatusUnauthorized, Code: "auth.unauthenticated"},
	{Err: access.ErrForbidden, Status: http.StatusForbidden, Code: "auth.forbidden"},
	{Err: application.ErrExperimentNotFound, Status: http.StatusNotFound, Code: "experiment.notFound"},
	{Err: application.ErrAllocationNotFound, Status: http.StatusNotFound, Code: "allocation.notFound"},
	{Err: application.ErrRegimenItemNotFound, Status: http.StatusNotFound, Code: "regimenItem.notFound"},
	{Err: application.ErrSubstanceNotFound, Status: http.StatusNotFound, Code: "substance.notFound"},
	{Err: application.ErrWeightNotFound, Status: http.StatusNotFound, Code: "weight.notFound"},
	{Err: application.ErrExperimentNotRunning, Status: http.StatusConflict, Code: "experiment.notRunning"},
	{Err: application.ErrExperimentNotStarted, Status: http.StatusConflict, Code: "experiment.notStarted"},
	{Err: application.ErrRegimenItemOtherGroup, Status: http.StatusBadRequest, Code: "administration.regimenItemOtherGroup"},
	{Err: application.ErrNoSubstanceItem, Status: http.StatusBadRequest, Code: "administration.noSubstanceItem"},
	{Err: application.ErrSubstanceRequired, Status: http.StatusBadRequest, Code: "administration.unplannedIncomplete"},
	{Err: application.ErrSubstanceMismatch, Status: http.StatusBadRequest, Code: "administration.substanceMismatch"},
	{Err: application.ErrPurposeMismatch, Status: http.StatusBadRequest, Code: "administration.purposeMismatch"},
	{Err: application.ErrDoseIncomplete, Status: http.StatusBadRequest, Code: "dose.incomplete"},
	{Err: application.ErrNoDisease, Status: http.StatusConflict, Code: "inductionOutcome.noDisease"},
	{Err: application.ErrOutcomeExists, Status: http.StatusConflict, Code: "inductionOutcome.exists"},
	{Err: application.ErrInvalidQuery, Status: http.StatusBadRequest, Code: "common.invalidQuery"},
	{Err: domain.ErrInvalidAmount, Status: http.StatusBadRequest, Code: "dose.invalidAmount"},
	{Err: domain.ErrAmountTooSmall, Status: http.StatusBadRequest, Code: "dose.amountTooSmall"},
	{Err: domain.ErrInvalidUnit, Status: http.StatusBadRequest, Code: "dose.invalidUnit"},
	{Err: domain.ErrInvalidRoute, Status: http.StatusBadRequest, Code: "dose.invalidRoute"},
	{Err: domain.ErrInvalidPurpose, Status: http.StatusBadRequest, Code: "dose.invalidPurpose"},
	{Err: domain.ErrInvalidWeight, Status: http.StatusBadRequest, Code: "weight.invalid"},
	{Err: domain.ErrInvalidNotes, Status: http.StatusBadRequest, Code: "common.invalidNotes"},
	{Err: domain.ErrInvalidOutcome, Status: http.StatusBadRequest, Code: "inductionOutcome.invalidStatus"},
	{Err: domain.ErrInvalidDate, Status: http.StatusBadRequest, Code: "common.invalidDate"},
	{Err: domain.ErrMissingTime, Status: http.StatusBadRequest, Code: "common.invalidTime"},
	{Err: domain.ErrFutureTime, Status: http.StatusBadRequest, Code: "common.futureTime"},
	{Err: domain.ErrBeforeAllocation, Status: http.StatusBadRequest, Code: "dosing.beforeAllocation"},
	{Err: domain.ErrAfterExclusion, Status: http.StatusConflict, Code: "dosing.afterExclusion"},
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
	mux.HandleFunc("POST /api/experiments/{id}/allocations/{allocationId}/administrations", h.with(h.recordAdministration))
	mux.HandleFunc("GET /api/experiments/{id}/administrations", h.with(h.experimentAdministrations))
	mux.HandleFunc("GET /api/subjects/{id}/administrations", h.with(h.subjectAdministrations))
	mux.HandleFunc("GET /api/experiments/{id}/dosing", h.with(h.dueDoses))
	mux.HandleFunc("POST /api/experiments/{id}/allocations/{allocationId}/induction-outcome", h.with(h.recordOutcome))
	mux.HandleFunc("GET /api/experiments/{id}/induction-outcomes", h.with(h.listOutcomes))
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

func (h handler) recordAdministration(w http.ResponseWriter, r *http.Request, actor access.Actor) {
	var body struct {
		RegimenItemID string    `json:"regimenItemId"`
		Purpose       string    `json:"purpose"`
		SubstanceID   string    `json:"substanceId"`
		Amount        string    `json:"amount"`
		Unit          string    `json:"unit"`
		Route         string    `json:"route"`
		WeightID      string    `json:"weightMeasurementId"`
		Notes         string    `json:"notes"`
		At            time.Time `json:"administeredAt"`
	}
	if !h.decode(w, r, &body) {
		return
	}
	a, err := h.service.RecordAdministration(r.Context(), actor, r.PathValue("id"), r.PathValue("allocationId"), application.AdministrationInput{
		RegimenItemID: body.RegimenItemID, Purpose: body.Purpose, SubstanceID: body.SubstanceID, Amount: body.Amount, Unit: body.Unit,
		Route: body.Route, WeightID: body.WeightID, Notes: body.Notes, AdministeredAt: body.At,
	})
	h.respond(w, r, http.StatusCreated, administrationJSON(a), err)
}

func (h handler) experimentAdministrations(w http.ResponseWriter, r *http.Request, actor access.Actor) {
	q, ok := h.administrationQuery(w, r)
	if !ok {
		return
	}
	q.AllocationID = r.URL.Query().Get("allocationId")
	page, err := h.service.ExperimentAdministrations(r.Context(), actor, r.PathValue("id"), q)
	h.respond(w, r, http.StatusOK, pageJSON{mapSlice(page.Administrations, administrationJSON), page.Total, page.Page, page.PageSize}, err)
}

func (h handler) subjectAdministrations(w http.ResponseWriter, r *http.Request, actor access.Actor) {
	q, ok := h.administrationQuery(w, r)
	if !ok {
		return
	}
	page, err := h.service.SubjectAdministrations(r.Context(), actor, r.PathValue("id"), q)
	h.respond(w, r, http.StatusOK, pageJSON{mapSlice(page.Administrations, administrationJSON), page.Total, page.Page, page.PageSize}, err)
}

// administrationQuery parses RFC 3339 from/to bounds; to is exclusive.
func (h handler) administrationQuery(w http.ResponseWriter, r *http.Request) (application.AdministrationQuery, bool) {
	values := r.URL.Query()
	q := application.AdministrationQuery{
		SubstanceID: values.Get("substanceId"), Purpose: values.Get("purpose"),
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

func (h handler) dueDoses(w http.ResponseWriter, r *http.Request, actor access.Actor) {
	s, err := h.service.DueDoses(r.Context(), actor, r.PathValue("id"), r.URL.Query().Get("date"))
	h.respond(w, r, http.StatusOK, scheduleJSON(r.PathValue("id"), s), err)
}

func (h handler) recordOutcome(w http.ResponseWriter, r *http.Request, actor access.Actor) {
	var body struct {
		Status     string    `json:"status"`
		Note       string    `json:"note"`
		AssessedAt time.Time `json:"assessedAt"`
	}
	if !h.decode(w, r, &body) {
		return
	}
	o, err := h.service.RecordInductionOutcome(r.Context(), actor, r.PathValue("id"), r.PathValue("allocationId"), application.OutcomeInput(body))
	h.respond(w, r, http.StatusCreated, outcomeJSON(o), err)
}

func (h handler) listOutcomes(w http.ResponseWriter, r *http.Request, actor access.Actor) {
	rows, err := h.service.InductionOutcomes(r.Context(), actor, r.PathValue("id"), r.URL.Query().Get("status"))
	h.respond(w, r, http.StatusOK, dataJSON{mapSlice(rows, outcomeRowJSON)}, err)
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

type doseJSON struct {
	Amount string `json:"amount"`
	Unit   string `json:"unit"`
}

func toDose(d domain.Dose) doseJSON { return doseJSON{d.Amount(), string(d.Unit)} }

func optionalDose(d *domain.Dose) *doseJSON {
	if d == nil {
		return nil
	}
	out := toDose(*d)
	return &out
}

type administrationJSONBody struct {
	ID                  string    `json:"id"`
	ExperimentID        string    `json:"experimentId"`
	AllocationID        string    `json:"allocationId"`
	GroupID             string    `json:"groupId"`
	SubjectID           string    `json:"subjectId"`
	RegimenItemID       *string   `json:"regimenItemId"`
	Purpose             string    `json:"purpose"`
	SubstanceID         string    `json:"substanceId"`
	SubstanceName       string    `json:"substanceName"`
	Dose                doseJSON  `json:"dose"`
	AbsoluteDose        *doseJSON `json:"absoluteDose"`
	Route               string    `json:"route"`
	WeightMeasurementID *string   `json:"weightMeasurementId"`
	BodyWeightGrams     *string   `json:"bodyWeightGrams"`
	AdministeredAt      time.Time `json:"administeredAt"`
	Notes               *string   `json:"notes"`
	RecordedBy          string    `json:"recordedBy"`
	CreatedAt           time.Time `json:"createdAt"`
}

func administrationJSON(a domain.Administration) administrationJSONBody {
	out := administrationJSONBody{
		ID: a.ID, ExperimentID: a.ExperimentID, AllocationID: a.AllocationID, GroupID: a.GroupID, SubjectID: a.SubjectID,
		RegimenItemID: nullable(a.RegimenItemID), Purpose: string(a.Purpose), SubstanceID: a.SubstanceID, SubstanceName: a.SubstanceName,
		Dose: toDose(a.Dose), Route: string(a.Route), WeightMeasurementID: nullable(a.WeightID),
		AdministeredAt: a.AdministeredAt, Notes: nullable(a.Notes), RecordedBy: a.RecordedBy, CreatedAt: a.CreatedAt,
	}
	if a.BodyMilligrams > 0 {
		out.BodyWeightGrams = nullable(domain.FormatGrams(a.BodyMilligrams))
	}
	if abs, ok := a.AbsoluteDose(); ok {
		out.AbsoluteDose = optionalDose(&abs)
	}
	return out
}

type weightJSONBody struct {
	ID         string    `json:"id"`
	Grams      string    `json:"grams"`
	MeasuredAt time.Time `json:"measuredAt"`
}

type dueDoseJSON struct {
	RegimenItemID string    `json:"regimenItemId"`
	Purpose       string    `json:"purpose"`
	SubstanceID   string    `json:"substanceId"`
	SubstanceName string    `json:"substanceName"`
	Dose          doseJSON  `json:"dose"`
	AbsoluteDose  *doseJSON `json:"absoluteDose"`
	Route         string    `json:"route"`
	Frequency     string    `json:"frequency"`
	Due           int       `json:"due"`
	Given         int       `json:"given"`
	Complete      bool      `json:"complete"`
}

type dueAnimalJSON struct {
	AllocationID string          `json:"allocationId"`
	SubjectID    string          `json:"subjectId"`
	SubjectCode  string          `json:"subjectCode"`
	GroupID      string          `json:"groupId"`
	GroupCode    string          `json:"groupCode"`
	Cage         *string         `json:"cage"`
	Excluded     bool            `json:"excluded"`
	Weight       *weightJSONBody `json:"weight"`
	Doses        []dueDoseJSON   `json:"doses"`
}

type scheduleJSONBody struct {
	ExperimentID string          `json:"experimentId"`
	Date         string          `json:"date"`
	StudyDay     int             `json:"studyDay"`
	Animals      []dueAnimalJSON `json:"animals"`
}

func scheduleJSON(experimentID string, s domain.Schedule) scheduleJSONBody {
	return scheduleJSONBody{
		ExperimentID: experimentID, Date: s.Date.Format(time.DateOnly), StudyDay: s.StudyDay,
		Animals: mapSlice(s.Animals, func(a domain.DueAnimal) dueAnimalJSON {
			out := dueAnimalJSON{
				AllocationID: a.Allocation.ID, SubjectID: a.Allocation.SubjectID, SubjectCode: a.SubjectCode,
				GroupID: a.Allocation.GroupID, GroupCode: a.GroupCode, Cage: nullable(a.Cage), Excluded: a.Excluded,
				Doses: mapSlice(a.Doses, func(d domain.DueDose) dueDoseJSON {
					return dueDoseJSON{
						RegimenItemID: d.Item.ID, Purpose: string(d.Item.Purpose), SubstanceID: d.Item.SubstanceID, SubstanceName: d.Item.SubstanceName,
						Dose: toDose(d.Item.Dose), AbsoluteDose: optionalDose(d.AbsoluteDose), Route: string(d.Item.Route),
						Frequency: string(d.Item.Frequency), Due: d.Due, Given: d.Given, Complete: d.Complete(),
					}
				}),
			}
			if a.Weight != nil {
				out.Weight = &weightJSONBody{a.Weight.ID, domain.FormatGrams(a.Weight.Milligrams), a.Weight.MeasuredAt}
			}
			return out
		}),
	}
}

type outcomeJSONBody struct {
	AllocationID string    `json:"allocationId"`
	ExperimentID string    `json:"experimentId"`
	Status       string    `json:"status"`
	AssessedAt   time.Time `json:"assessedAt"`
	Note         *string   `json:"note"`
	RecordedBy   string    `json:"recordedBy"`
	CreatedAt    time.Time `json:"createdAt"`
}

func outcomeJSON(o domain.InductionOutcome) outcomeJSONBody {
	return outcomeJSONBody{o.AllocationID, o.ExperimentID, string(o.Status), o.AssessedAt, nullable(o.Note), o.RecordedBy, o.CreatedAt}
}

type outcomeRowJSONBody struct {
	outcomeJSONBody
	SubjectID   string `json:"subjectId"`
	SubjectCode string `json:"subjectCode"`
	GroupID     string `json:"groupId"`
	GroupCode   string `json:"groupCode"`
}

func outcomeRowJSON(r application.OutcomeRow) outcomeRowJSONBody {
	return outcomeRowJSONBody{outcomeJSON(r.InductionOutcome), r.SubjectID, r.SubjectCode, r.GroupID, r.GroupCode}
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
