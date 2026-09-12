// Package domain holds disease models, substances and intervention plans, and
// the append-only records of body weight, subject condition and actual
// administration. A plan is never an administration, and induction is never a
// confirmed disease.
package domain

import (
	"errors"
	"fmt"
	"math"
	"math/bits"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

var (
	ErrInvalidAmount      = errors.New("amount must be a positive decimal with at most 9 integer and 6 fractional digits")
	ErrAmountTooSmall     = errors.New("the converted amount rounds to zero")
	ErrInvalidUnit        = errors.New("unit must be one of mg, ug, g, mL, uL, IU, mg/kg, ug/kg, IU/kg")
	ErrInvalidRoute       = errors.New("route is not supported")
	ErrInvalidWeight      = errors.New("body weight must be between 0.001 and 5000 grams with at most 3 decimals")
	ErrInvalidName        = errors.New("name must contain 1 to 120 printable characters")
	ErrInvalidDescription = errors.New("description must contain at most 5000 characters")
	ErrInvalidSchedule    = errors.New("schedule must contain 1 to 500 printable characters")
	ErrInvalidNotes       = errors.New("notes must contain at most 2000 characters")
	ErrInvalidStatus      = errors.New("status must be INDUCED, CONFIRMED, NOT_CONFIRMED or RESOLVED")
	ErrMissingTime        = errors.New("a timestamp is required")
	ErrFutureTime         = errors.New("time cannot be in the future")
	ErrBeforeEnrollment   = errors.New("administration cannot precede enrollment")
	ErrWeightRequired     = errors.New("a body weight measurement is required for per-kilogram doses")
	ErrWeightOtherSubject = errors.New("weight measurement belongs to another subject")
	ErrWeightNotRelevant  = errors.New("weight must be measured within 7 days before the administration")
)

const (
	// ClockSkew tolerates small differences between client and server clocks.
	ClockSkew = 5 * time.Minute
	// WeightWindow is how old a weight may be to scale a per-kilogram dose.
	WeightWindow = 7 * 24 * time.Hour
	microScale   = 1_000_000
)

var (
	amountPattern = regexp.MustCompile(`^([0-9]{1,9})(?:\.([0-9]{1,6}))?$`)
	gramsPattern  = regexp.MustCompile(`^([0-9]{1,4})(?:\.([0-9]{1,3}))?$`)
)

// ParseMicro parses a positive decimal into millionths, exactly.
func ParseMicro(raw string) (int64, error) {
	v, ok := parseFixed(amountPattern, raw, 6)
	if !ok || v <= 0 {
		return 0, ErrInvalidAmount
	}
	return v, nil
}

func FormatMicro(micro int64) string { return formatFixed(micro, 6) }

// ParseGrams parses body weight in grams into milligrams.
func ParseGrams(raw string) (int64, error) {
	v, ok := parseFixed(gramsPattern, raw, 3)
	if !ok || v <= 0 || v > 5_000_000 {
		return 0, ErrInvalidWeight
	}
	return v, nil
}

func FormatGrams(milligrams int64) string { return formatFixed(milligrams, 3) }

func parseFixed(pattern *regexp.Regexp, raw string, decimals int) (int64, bool) {
	m := pattern.FindStringSubmatch(strings.TrimSpace(raw))
	if m == nil {
		return 0, false
	}
	whole, _ := strconv.ParseInt(m[1], 10, 64)
	fraction, _ := strconv.ParseInt(m[2]+strings.Repeat("0", decimals-len(m[2])), 10, 64)
	return whole*int64(math.Pow10(decimals)) + fraction, true
}

func formatFixed(v int64, decimals int) string {
	scale := int64(math.Pow10(decimals))
	if v%scale == 0 {
		return strconv.FormatInt(v/scale, 10)
	}
	return strings.TrimRight(fmt.Sprintf("%d.%0*d", v/scale, decimals, v%scale), "0")
}

type Unit string

const (
	Milligram                    Unit = "mg"
	Microgram                    Unit = "ug"
	Gram                         Unit = "g"
	Milliliter                   Unit = "mL"
	Microliter                   Unit = "uL"
	InternationalUnit            Unit = "IU"
	MilligramPerKilogram         Unit = "mg/kg"
	MicrogramPerKilogram         Unit = "ug/kg"
	InternationalUnitPerKilogram Unit = "IU/kg"
)

func ParseUnit(raw string) (Unit, error) {
	switch u := Unit(raw); u {
	case Milligram, Microgram, Gram, Milliliter, Microliter, InternationalUnit, MilligramPerKilogram, MicrogramPerKilogram, InternationalUnitPerKilogram:
		return u, nil
	}
	return "", ErrInvalidUnit
}

// PerBodyMass units scale with body weight and need a weight measurement.
func (u Unit) PerBodyMass() bool { return strings.HasSuffix(string(u), "/kg") }

type Route string

const (
	Oral                    Route = "ORAL"
	Intraperitoneal         Route = "INTRAPERITONEAL"
	Subcutaneous            Route = "SUBCUTANEOUS"
	Intravenous             Route = "INTRAVENOUS"
	Intramuscular           Route = "INTRAMUSCULAR"
	Intranasal              Route = "INTRANASAL"
	Intracerebroventricular Route = "INTRACEREBROVENTRICULAR"
	Topical                 Route = "TOPICAL"
	Inhalation              Route = "INHALATION"
)

func ParseRoute(raw string) (Route, error) {
	switch r := Route(raw); r {
	case Oral, Intraperitoneal, Subcutaneous, Intravenous, Intramuscular, Intranasal, Intracerebroventricular, Topical, Inhalation:
		return r, nil
	}
	return "", ErrInvalidRoute
}

// Dose stores its amount in millionths of Unit.
type Dose struct {
	Micro int64
	Unit  Unit
}

func ParseDose(amount, unit string) (Dose, error) {
	u, err := ParseUnit(unit)
	if err != nil {
		return Dose{}, err
	}
	micro, err := ParseMicro(amount)
	if err != nil {
		return Dose{}, err
	}
	return Dose{Micro: micro, Unit: u}, nil
}

func (d Dose) Amount() string { return FormatMicro(d.Micro) }

// Absolute multiplies a per-kilogram dose by body mass, rounding half up to the
// nearest millionth: amount x milligrams / 1,000,000. Other doses are unchanged.
func (d Dose) Absolute(bodyMilligrams int64) (Dose, error) {
	if !d.Unit.PerBodyMass() {
		return d, nil
	}
	if bodyMilligrams <= 0 {
		return Dose{}, ErrInvalidWeight
	}
	hi, lo := bits.Mul64(uint64(d.Micro), uint64(bodyMilligrams))
	lo, carry := bits.Add64(lo, microScale/2, 0)
	hi += carry
	if hi >= microScale {
		return Dose{}, ErrInvalidAmount
	}
	q, _ := bits.Div64(hi, lo, microScale)
	switch {
	case q == 0:
		return Dose{}, ErrAmountTooSmall
	case q > math.MaxInt64:
		return Dose{}, ErrInvalidAmount
	}
	return Dose{Micro: int64(q), Unit: Unit(strings.TrimSuffix(string(d.Unit), "/kg"))}, nil
}

type DiseaseModel struct {
	ID, Name, Description string
	CreatedAt, UpdatedAt  time.Time
}

func NewDiseaseModel(name, description string) (DiseaseModel, error) {
	n, d, err := catalogEntry(name, description)
	return DiseaseModel{Name: n, Description: d}, err
}

type Substance struct {
	ID, Name, Description string
	CreatedAt, UpdatedAt  time.Time
}

func NewSubstance(name, description string) (Substance, error) {
	n, d, err := catalogEntry(name, description)
	return Substance{Name: n, Description: d}, err
}

func catalogEntry(name, description string) (string, string, error) {
	n, err := NormalizeName(name)
	if err != nil {
		return "", "", err
	}
	d, err := NormalizeDescription(description)
	return n, d, err
}

// Plan is what a group is meant to receive. It never creates administrations.
type Plan struct {
	ID, ExperimentID, GroupID, PhaseID, SubstanceID string
	Dose                                            Dose
	Route                                           Route
	Schedule, Notes                                 string
	CreatedAt, UpdatedAt                            time.Time
}

func NewPlan(experimentID, groupID, phaseID, substanceID string, dose Dose, route Route, schedule, notes string) (Plan, error) {
	p := Plan{ExperimentID: experimentID, GroupID: groupID, PhaseID: phaseID, SubstanceID: substanceID, Dose: dose, Route: route}
	var err error
	if p.Schedule, err = NormalizeSchedule(schedule); err != nil {
		return Plan{}, err
	}
	if p.Notes, err = NormalizeNotes(notes); err != nil {
		return Plan{}, err
	}
	return p, nil
}

type Weight struct {
	ID, SubjectID string
	Milligrams    int64
	MeasuredAt    time.Time
	RecordedBy    string
	CreatedAt     time.Time
}

func NewWeight(subjectID, grams string, measuredAt, now time.Time, recordedBy string) (Weight, error) {
	mg, err := ParseGrams(grams)
	if err != nil {
		return Weight{}, err
	}
	if err := checkPast(measuredAt, now); err != nil {
		return Weight{}, err
	}
	return Weight{SubjectID: subjectID, Milligrams: mg, MeasuredAt: measuredAt, RecordedBy: recordedBy}, nil
}

type ConditionStatus string

const (
	Induced      ConditionStatus = "INDUCED"
	Confirmed    ConditionStatus = "CONFIRMED"
	NotConfirmed ConditionStatus = "NOT_CONFIRMED"
	Resolved     ConditionStatus = "RESOLVED"
)

func ParseStatus(raw string) (ConditionStatus, error) {
	switch s := ConditionStatus(raw); s {
	case Induced, Confirmed, NotConfirmed, Resolved:
		return s, nil
	}
	return "", ErrInvalidStatus
}

// Condition is one observation of a subject's disease-model status.
// DiseaseModelName is the name when the observation was recorded.
type Condition struct {
	ID, SubjectID, DiseaseModelID, DiseaseModelName, EnrollmentID string
	Status                                                        ConditionStatus
	ObservedAt                                                    time.Time
	Notes, RecordedBy                                             string
	CreatedAt                                                     time.Time
}

func NewCondition(subjectID, diseaseModelID, status string, observedAt, now time.Time, enrollmentID, notes, recordedBy string) (Condition, error) {
	c := Condition{SubjectID: subjectID, DiseaseModelID: diseaseModelID, EnrollmentID: enrollmentID, ObservedAt: observedAt, RecordedBy: recordedBy}
	var err error
	if c.Status, err = ParseStatus(status); err != nil {
		return Condition{}, err
	}
	if err = checkPast(observedAt, now); err != nil {
		return Condition{}, err
	}
	if c.Notes, err = NormalizeNotes(notes); err != nil {
		return Condition{}, err
	}
	return c, nil
}

// CurrentConditions returns the latest observation per disease model, ordered by
// observation time, then recording time, then id.
func CurrentConditions(history []Condition) map[string]Condition {
	current := map[string]Condition{}
	for _, c := range history {
		if prev, ok := current[c.DiseaseModelID]; !ok || later(c, prev) {
			current[c.DiseaseModelID] = c
		}
	}
	return current
}

func later(a, b Condition) bool {
	if !a.ObservedAt.Equal(b.ObservedAt) {
		return a.ObservedAt.After(b.ObservedAt)
	}
	if !a.CreatedAt.Equal(b.CreatedAt) {
		return a.CreatedAt.After(b.CreatedAt)
	}
	return a.ID > b.ID
}

// Administration is an actual, recorded administration. Substance name, dose,
// route and body weight are copied when recorded.
type Administration struct {
	ID, ExperimentID, EnrollmentID, SubjectID string
	SubstanceID, SubstanceName, PlanID        string
	WeightID                                  string
	BodyMilligrams                            int64
	Dose                                      Dose
	Route                                     Route
	AdministeredAt                            time.Time
	Notes, RecordedBy                         string
	CreatedAt                                 time.Time
}

// AbsoluteDose is the recorded dose in absolute units, when it can be derived
// from the recorded values alone.
func (a Administration) AbsoluteDose() (Dose, bool) {
	if a.Dose.Unit.PerBodyMass() && a.BodyMilligrams == 0 {
		return Dose{}, false
	}
	d, err := a.Dose.Absolute(a.BodyMilligrams)
	return d, err == nil
}

// CheckAdministration validates timing and the weight reference of a record.
func CheckAdministration(a Administration, enrolledAt time.Time, weight *Weight, now time.Time) error {
	switch {
	case a.AdministeredAt.IsZero():
		return ErrMissingTime
	case a.AdministeredAt.Before(enrolledAt):
		return ErrBeforeEnrollment
	case a.AdministeredAt.After(now.Add(ClockSkew)):
		return ErrFutureTime
	}
	if weight == nil {
		if a.Dose.Unit.PerBodyMass() {
			return ErrWeightRequired
		}
		return nil
	}
	if weight.SubjectID != a.SubjectID {
		return ErrWeightOtherSubject
	}
	if weight.MeasuredAt.After(a.AdministeredAt) || a.AdministeredAt.Sub(weight.MeasuredAt) > WeightWindow {
		return ErrWeightNotRelevant
	}
	return nil
}

func checkPast(t, now time.Time) error {
	if t.IsZero() {
		return ErrMissingTime
	}
	if t.After(now.Add(ClockSkew)) {
		return ErrFutureTime
	}
	return nil
}

func NormalizeName(raw string) (string, error) { return text(raw, 1, 120, false, ErrInvalidName) }

func NormalizeDescription(raw string) (string, error) {
	return text(raw, 0, 5000, true, ErrInvalidDescription)
}

func NormalizeSchedule(raw string) (string, error) {
	return text(raw, 1, 500, false, ErrInvalidSchedule)
}

func NormalizeNotes(raw string) (string, error) { return text(raw, 0, 2000, true, ErrInvalidNotes) }

func text(raw string, minChars, maxChars int, multiline bool, invalid error) (string, error) {
	s := strings.TrimSpace(raw)
	bad := func(r rune) bool {
		return unicode.IsControl(r) && !(multiline && (r == '\n' || r == '\r' || r == '\t'))
	}
	n := utf8.RuneCountInString(s)
	if !utf8.ValidString(s) || n < minChars || n > maxChars || strings.IndexFunc(s, bad) >= 0 {
		return "", invalid
	}
	return s, nil
}
