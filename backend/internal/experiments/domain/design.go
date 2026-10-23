package domain

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
)

var (
	ErrInvalidGroupCode       = errors.New("group code must contain 1 to 16 ASCII letters, digits, '.', '_' or '-'")
	ErrInvalidName            = errors.New("name must contain 1 to 120 printable characters")
	ErrInvalidRole            = errors.New("role must be HEALTHY_CONTROL, SHAM, DISEASE_CONTROL, POSITIVE_CONTROL or TREATMENT")
	ErrInvalidTargetSize      = errors.New("target size must be between 0 and 100000")
	ErrDiseaseModelRequired   = errors.New("every group except a healthy control needs a disease model")
	ErrDiseaseModelNotAllowed = errors.New("a healthy control has no disease model")

	ErrInvalidPurpose    = errors.New("purpose must be NONE, INDUCTION, VEHICLE or TREATMENT")
	ErrPurposeNotAllowed = errors.New("the group's role cannot receive an item with this purpose")
	ErrNoSubstanceFields = errors.New("a period without any substance (purpose NONE) has only a first and a last study day")
	ErrInvalidAmount     = errors.New("amount must be a positive decimal with at most 9 integer and 6 fractional digits")
	ErrInvalidUnit       = errors.New("unit must be one of mg/kg, ug/kg, mL/kg, IU/kg, mg, ug, g, mL, uL, IU")
	ErrInvalidRoute      = errors.New("route is not supported")
	ErrInvalidFrequency  = errors.New("frequency must be ONCE, DAILY, TWICE_DAILY, WEEKLY or EVERY_N_DAYS")
	ErrInvalidInterval   = errors.New("interval days must be between 2 and 365 for EVERY_N_DAYS and absent otherwise")
	ErrInvalidDays       = errors.New("study days must be between -365 and 3650, end on or after start, and be equal for ONCE")

	ErrInvalidStudyDay = errors.New("study day must be between -365 and 3650")
	ErrInvalidPosition = errors.New("position must be between 1 and 1000")

	ErrInvalidParadigm       = errors.New("paradigm key must be 1 to 40 upper-case letters or '_'")
	ErrInvalidMetric         = errors.New("metric key must be 1 to 80 lower-case letters, digits or '_', starting with a letter")
	ErrInvalidKind           = errors.New("kind must be PRIMARY or SECONDARY")
	ErrInvalidDirection      = errors.New("direction must be HIGHER_IS_BETTER or LOWER_IS_BETTER")
	ErrBaselineSameTimepoint = errors.New("the baseline timepoint must differ from the endpoint's timepoint")
)

const (
	MinStudyDay = -365
	MaxStudyDay = 3650
)

var (
	groupCodePattern = regexp.MustCompile(`^[A-Za-z0-9._-]{1,16}$`)
	paradigmPattern  = regexp.MustCompile(`^[A-Z][A-Z_]{0,39}$`)
	metricPattern    = regexp.MustCompile(`^[a-z][a-z0-9_]{0,79}$`)
	amountPattern    = regexp.MustCompile(`^([0-9]{1,9})(?:\.([0-9]{1,6}))?$`)
)

type Role string

const (
	HealthyControl  Role = "HEALTHY_CONTROL"
	Sham            Role = "SHAM"
	DiseaseControl  Role = "DISEASE_CONTROL"
	PositiveControl Role = "POSITIVE_CONTROL"
	Treatment       Role = "TREATMENT"
)

func ParseRole(raw string) (Role, error) {
	switch r := Role(raw); r {
	case HealthyControl, Sham, DiseaseControl, PositiveControl, Treatment:
		return r, nil
	}
	return "", ErrInvalidRole
}

// Group is an arm of the study, defined by its role and disease model. Only a
// healthy control has no disease model. TargetSize 0 means no target yet.
type Group struct {
	ID             string
	ExperimentID   string
	Code           string
	Name           string
	Role           Role
	DiseaseModelID string
	TargetSize     int
	Description    string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type GroupFields struct {
	Code, Name, Role, DiseaseModelID, Description string
	TargetSize                                    int
}

func NewGroup(experimentID string, f GroupFields) (Group, error) {
	g := Group{ExperimentID: experimentID}
	return g, g.Apply(f)
}

func (g *Group) Apply(f GroupFields) error {
	out := *g
	var err error
	if out.Code = strings.TrimSpace(f.Code); !groupCodePattern.MatchString(out.Code) {
		return ErrInvalidGroupCode
	}
	if out.Name, err = line(f.Name, 120, ErrInvalidName); err != nil {
		return err
	}
	if out.Role, err = ParseRole(f.Role); err != nil {
		return err
	}
	out.DiseaseModelID = strings.TrimSpace(f.DiseaseModelID)
	switch {
	case out.Role == HealthyControl && out.DiseaseModelID != "":
		return ErrDiseaseModelNotAllowed
	case out.Role != HealthyControl && out.DiseaseModelID == "":
		return ErrDiseaseModelRequired
	}
	if f.TargetSize < 0 || f.TargetSize > 100000 {
		return ErrInvalidTargetSize
	}
	out.TargetSize = f.TargetSize
	if out.Description, err = text(f.Description, 5000, ErrInvalidDescription); err != nil {
		return err
	}
	*g = out
	return nil
}

func (g Group) Fields() GroupFields {
	return GroupFields{Code: g.Code, Name: g.Name, Role: string(g.Role), DiseaseModelID: g.DiseaseModelID, Description: g.Description, TargetSize: g.TargetSize}
}

type Purpose string

const (
	// NoSubstance is a period in which the group receives no substance at
	// all: a healthy control's only kind of item. It has a first and a last
	// study day and nothing else.
	NoSubstance      Purpose = "NONE"
	Induction        Purpose = "INDUCTION"
	Vehicle          Purpose = "VEHICLE"
	TreatmentPurpose Purpose = "TREATMENT"
)

func ParsePurpose(raw string) (Purpose, error) {
	switch p := Purpose(raw); p {
	case NoSubstance, Induction, Vehicle, TreatmentPurpose:
		return p, nil
	}
	return "", ErrInvalidPurpose
}

// Purposes lists the purposes a group of the role can ever receive: a healthy
// control only periods without any substance, a sham only a vehicle, a
// disease control its induction and a vehicle, and the treated groups also
// their one treatment. No other role has a period without a substance. Which
// items a group must have before the study starts is checked by Readiness.
func (r Role) Purposes() []Purpose {
	switch r {
	case HealthyControl:
		return []Purpose{NoSubstance}
	case Sham:
		return []Purpose{Vehicle}
	case DiseaseControl:
		return []Purpose{Induction, Vehicle}
	case PositiveControl, Treatment:
		return []Purpose{Induction, Vehicle, TreatmentPurpose}
	}
	return nil
}

// AllowPurpose returns ErrPurposeNotAllowed when the role can never receive
// an item with the purpose.
func (r Role) AllowPurpose(p Purpose) error {
	if !slices.Contains(r.Purposes(), p) {
		return ErrPurposeNotAllowed
	}
	return nil
}

var units = []string{"mg/kg", "ug/kg", "mL/kg", "IU/kg", "mg", "ug", "g", "mL", "uL", "IU"}

var routes = []string{"ORAL", "INTRAPERITONEAL", "SUBCUTANEOUS", "INTRAVENOUS", "INTRAMUSCULAR", "INTRANASAL",
	"INTRACEREBROVENTRICULAR", "TOPICAL", "INHALATION"}

type Frequency string

const (
	Once       Frequency = "ONCE"
	Daily      Frequency = "DAILY"
	TwiceDaily Frequency = "TWICE_DAILY"
	Weekly     Frequency = "WEEKLY"
	EveryNDays Frequency = "EVERY_N_DAYS"
)

func ParseFrequency(raw string) (Frequency, error) {
	switch f := Frequency(raw); f {
	case Once, Daily, TwiceDaily, Weekly, EveryNDays:
		return f, nil
	}
	return "", ErrInvalidFrequency
}

// RegimenItem is what a group receives: its disease induction, a vehicle or
// its treatment, as a dose per administration over study days. Amounts are
// stored exactly in millionths of Unit. A NoSubstance item is only a period
// of study days: its substance, dose, unit, route and frequency are empty.
type RegimenItem struct {
	ID           string
	ExperimentID string
	GroupID      string
	Purpose      Purpose
	SubstanceID  string
	DoseMicro    int64
	Unit         string
	Route        string
	Frequency    Frequency
	// IntervalDays is set only for EVERY_N_DAYS; 0 otherwise.
	IntervalDays int
	StartDay     int
	EndDay       int
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

type RegimenFields struct {
	GroupID, Purpose, SubstanceID, Amount, Unit, Route, Frequency string
	IntervalDays, StartDay, EndDay                                int
}

func NewRegimenItem(experimentID string, f RegimenFields) (RegimenItem, error) {
	r := RegimenItem{ExperimentID: experimentID}
	return r, r.Apply(f)
}

func (r *RegimenItem) Apply(f RegimenFields) error {
	out := *r
	var err error
	out.GroupID, out.SubstanceID = strings.TrimSpace(f.GroupID), strings.TrimSpace(f.SubstanceID)
	if out.Purpose, err = ParsePurpose(f.Purpose); err != nil {
		return err
	}
	if out.Purpose == NoSubstance {
		if out.SubstanceID != "" || strings.TrimSpace(f.Amount) != "" || f.Unit != "" || f.Route != "" || f.Frequency != "" || f.IntervalDays != 0 {
			return ErrNoSubstanceFields
		}
		if !studyDay(f.StartDay) || !studyDay(f.EndDay) || f.EndDay < f.StartDay {
			return ErrInvalidDays
		}
		out.DoseMicro, out.Unit, out.Route, out.Frequency, out.IntervalDays = 0, "", "", "", 0
		out.StartDay, out.EndDay = f.StartDay, f.EndDay
		*r = out
		return nil
	}
	if out.DoseMicro, err = ParseMicro(f.Amount); err != nil {
		return err
	}
	if !slices.Contains(units, f.Unit) {
		return ErrInvalidUnit
	}
	if !slices.Contains(routes, f.Route) {
		return ErrInvalidRoute
	}
	out.Unit, out.Route = f.Unit, f.Route
	if out.Frequency, err = ParseFrequency(f.Frequency); err != nil {
		return err
	}
	if (out.Frequency == EveryNDays) != (f.IntervalDays != 0) || (f.IntervalDays != 0 && (f.IntervalDays < 2 || f.IntervalDays > 365)) {
		return ErrInvalidInterval
	}
	out.IntervalDays = f.IntervalDays
	if !studyDay(f.StartDay) || !studyDay(f.EndDay) || f.EndDay < f.StartDay || (out.Frequency == Once && f.EndDay != f.StartDay) {
		return ErrInvalidDays
	}
	out.StartDay, out.EndDay = f.StartDay, f.EndDay
	*r = out
	return nil
}

func (r RegimenItem) Fields() RegimenFields {
	return RegimenFields{GroupID: r.GroupID, Purpose: string(r.Purpose), SubstanceID: r.SubstanceID, Amount: r.Amount(),
		Unit: r.Unit, Route: r.Route, Frequency: string(r.Frequency), IntervalDays: r.IntervalDays, StartDay: r.StartDay, EndDay: r.EndDay}
}

// Amount is the dose per administration as an exact decimal; empty for a
// period without any substance.
func (r RegimenItem) Amount() string {
	if r.Purpose == NoSubstance {
		return ""
	}
	return FormatMicro(r.DoseMicro)
}

// Timepoint is when animals are tested, in study days relative to day 0.
type Timepoint struct {
	ID           string
	ExperimentID string
	Name         string
	StudyDay     int
	Position     int
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

type TimepointFields struct {
	Name               string
	StudyDay, Position int
}

func NewTimepoint(experimentID string, f TimepointFields) (Timepoint, error) {
	t := Timepoint{ExperimentID: experimentID}
	return t, t.Apply(f)
}

func (t *Timepoint) Apply(f TimepointFields) error {
	name, err := line(f.Name, 120, ErrInvalidName)
	if err != nil {
		return err
	}
	if !studyDay(f.StudyDay) {
		return ErrInvalidStudyDay
	}
	if f.Position < 1 || f.Position > 1000 {
		return ErrInvalidPosition
	}
	t.Name, t.StudyDay, t.Position = name, f.StudyDay, f.Position
	return nil
}

func (t Timepoint) Fields() TimepointFields {
	return TimepointFields{Name: t.Name, StudyDay: t.StudyDay, Position: t.Position}
}

type EndpointKind string

const (
	Primary   EndpointKind = "PRIMARY"
	Secondary EndpointKind = "SECONDARY"
)

type Direction string

const (
	HigherIsBetter Direction = "HIGHER_IS_BETTER"
	LowerIsBetter  Direction = "LOWER_IS_BETTER"
)

// Endpoint is one metric of one paradigm at one timepoint that the study is
// judged on, optionally as change from a baseline timepoint.
type Endpoint struct {
	ID                  string
	ExperimentID        string
	TimepointID         string
	BaselineTimepointID string
	ParadigmKey         string
	MetricKey           string
	Kind                EndpointKind
	Direction           Direction
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

type EndpointFields struct {
	TimepointID, BaselineTimepointID, ParadigmKey, MetricKey, Kind, Direction string
}

func NewEndpoint(experimentID string, f EndpointFields) (Endpoint, error) {
	e := Endpoint{ExperimentID: experimentID}
	return e, e.Apply(f)
}

func (e *Endpoint) Apply(f EndpointFields) error {
	out := *e
	out.TimepointID, out.BaselineTimepointID = strings.TrimSpace(f.TimepointID), strings.TrimSpace(f.BaselineTimepointID)
	if out.BaselineTimepointID != "" && out.BaselineTimepointID == out.TimepointID {
		return ErrBaselineSameTimepoint
	}
	if !paradigmPattern.MatchString(f.ParadigmKey) {
		return ErrInvalidParadigm
	}
	if !metricPattern.MatchString(f.MetricKey) {
		return ErrInvalidMetric
	}
	out.ParadigmKey, out.MetricKey = f.ParadigmKey, f.MetricKey
	switch k := EndpointKind(f.Kind); k {
	case Primary, Secondary:
		out.Kind = k
	default:
		return ErrInvalidKind
	}
	switch d := Direction(f.Direction); d {
	case HigherIsBetter, LowerIsBetter:
		out.Direction = d
	default:
		return ErrInvalidDirection
	}
	*e = out
	return nil
}

func (e Endpoint) Fields() EndpointFields {
	return EndpointFields{TimepointID: e.TimepointID, BaselineTimepointID: e.BaselineTimepointID, ParadigmKey: e.ParadigmKey,
		MetricKey: e.MetricKey, Kind: string(e.Kind), Direction: string(e.Direction)}
}

// ParseMicro parses a positive decimal into millionths, exactly.
func ParseMicro(raw string) (int64, error) {
	m := amountPattern.FindStringSubmatch(strings.TrimSpace(raw))
	if m == nil {
		return 0, ErrInvalidAmount
	}
	whole, _ := strconv.ParseInt(m[1], 10, 64)
	fraction, _ := strconv.ParseInt(m[2]+strings.Repeat("0", 6-len(m[2])), 10, 64)
	if v := whole*1_000_000 + fraction; v > 0 {
		return v, nil
	}
	return 0, ErrInvalidAmount
}

// FormatMicro writes millionths as the shortest exact decimal.
func FormatMicro(micro int64) string {
	if micro%1_000_000 == 0 {
		return strconv.FormatInt(micro/1_000_000, 10)
	}
	return strings.TrimRight(fmt.Sprintf("%d.%06d", micro/1_000_000, micro%1_000_000), "0")
}

func studyDay(d int) bool { return d >= MinStudyDay && d <= MaxStudyDay }
