// Package domain holds what actually happens to the animals of a running
// study: every dose given (disease induction, vehicle and treatment alike),
// the schedule of doses due from each group's regimen, and whether an animal's
// disease model was confirmed. A regimen item is a plan; only an
// administration records that a dose was given.
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
	ErrInvalidUnit        = errors.New("unit must be one of mg, ug, g, mL, uL, IU, mg/kg, ug/kg, mL/kg, IU/kg")
	ErrInvalidRoute       = errors.New("route is not supported")
	ErrInvalidPurpose     = errors.New("purpose must be INDUCTION, VEHICLE or TREATMENT")
	ErrInvalidWeight      = errors.New("body weight must be between 0.001 and 5000 grams")
	ErrInvalidNotes       = errors.New("notes must contain at most 2000 characters")
	ErrInvalidOutcome     = errors.New("status must be CONFIRMED or NOT_CONFIRMED")
	ErrMissingTime        = errors.New("a timestamp is required")
	ErrFutureTime         = errors.New("time cannot be in the future")
	ErrBeforeAllocation   = errors.New("the record cannot precede the animal's allocation")
	ErrAfterExclusion     = errors.New("the animal was excluded before this time")
	ErrWeightRequired     = errors.New("a body weight measured before the dose is required for per-kilogram doses")
	ErrWeightOtherSubject = errors.New("weight measurement belongs to another animal")
	ErrWeightNotRelevant  = errors.New("weight must be measured within 7 days before the administration")
)

const (
	// ClockSkew tolerates small differences between client and server clocks.
	ClockSkew = 5 * time.Minute
	// WeightWindow is how old a weight may be to scale a per-kilogram dose.
	WeightWindow = 7 * 24 * time.Hour
	microScale   = 1_000_000
)

var amountPattern = regexp.MustCompile(`^([0-9]{1,9})(?:\.([0-9]{1,6}))?$`)

// ParseMicro parses a positive decimal into millionths, exactly.
func ParseMicro(raw string) (int64, error) {
	m := amountPattern.FindStringSubmatch(strings.TrimSpace(raw))
	if m == nil {
		return 0, ErrInvalidAmount
	}
	whole, _ := strconv.ParseInt(m[1], 10, 64)
	fraction, _ := strconv.ParseInt(m[2]+strings.Repeat("0", 6-len(m[2])), 10, 64)
	v := whole*microScale + fraction
	if v <= 0 {
		return 0, ErrInvalidAmount
	}
	return v, nil
}

func FormatMicro(micro int64) string { return formatFixed(micro, 6) }

// FormatGrams formats a body weight stored in milligrams as grams.
func FormatGrams(milligrams int64) string { return formatFixed(milligrams, 3) }

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
	MilliliterPerKilogram        Unit = "mL/kg"
	InternationalUnitPerKilogram Unit = "IU/kg"
)

func ParseUnit(raw string) (Unit, error) {
	switch u := Unit(raw); u {
	case Milligram, Microgram, Gram, Milliliter, Microliter, InternationalUnit,
		MilligramPerKilogram, MicrogramPerKilogram, MilliliterPerKilogram, InternationalUnitPerKilogram:
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

// Purpose says why a substance is given: to induce the disease model, as the
// vehicle control or as the treatment.
type Purpose string

const (
	Induction Purpose = "INDUCTION"
	Vehicle   Purpose = "VEHICLE"
	Treatment Purpose = "TREATMENT"
)

func ParsePurpose(raw string) (Purpose, error) {
	switch p := Purpose(raw); p {
	case Induction, Vehicle, Treatment:
		return p, nil
	}
	return "", ErrInvalidPurpose
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

// Weight is a body weight measurement of one animal.
type Weight struct {
	ID, SubjectID string
	Milligrams    int64
	MeasuredAt    time.Time
}

// Administration is one dose actually given to an allocated animal.
// RegimenItemID is empty for an unplanned dose. WeightID and BodyMilligrams
// name the weight a per-kilogram dose was computed from.
type Administration struct {
	ID, ExperimentID, AllocationID, GroupID, SubjectID string
	RegimenItemID                                      string
	Purpose                                            Purpose
	SubstanceID, SubstanceName                         string
	Dose                                               Dose
	Route                                              Route
	WeightID                                           string
	BodyMilligrams                                     int64
	AdministeredAt                                     time.Time
	Notes, RecordedBy                                  string
	CreatedAt                                          time.Time
}

// AbsoluteDose is the amount actually given in absolute units, when it can be
// derived from the recorded values.
func (a Administration) AbsoluteDose() (Dose, bool) {
	if a.Dose.Unit.PerBodyMass() && a.BodyMilligrams == 0 {
		return Dose{}, false
	}
	d, err := a.Dose.Absolute(a.BodyMilligrams)
	return d, err == nil
}

// Allocation is the part of an animal's allocation a record is checked against.
type Allocation struct {
	ID, ExperimentID, GroupID, SubjectID string
	AllocatedAt                          time.Time
	ExcludedAt                           *time.Time
}

// CheckTime validates when something happened to an allocated animal: it is
// set, not in the future, not before the allocation and not after exclusion.
func CheckTime(at time.Time, a Allocation, now time.Time) error {
	switch {
	case at.IsZero():
		return ErrMissingTime
	case at.After(now.Add(ClockSkew)):
		return ErrFutureTime
	case at.Before(a.AllocatedAt):
		return ErrBeforeAllocation
	case a.ExcludedAt != nil && !at.Before(*a.ExcludedAt):
		return ErrAfterExclusion
	}
	return nil
}

// CheckWeight validates the weight a dose is computed from: same animal,
// measured before the dose and at most WeightWindow earlier. A per-kilogram
// dose needs one.
func CheckWeight(a Administration, weight *Weight) error {
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

// OutcomeStatus says whether an animal's disease model was confirmed.
type OutcomeStatus string

const (
	Confirmed    OutcomeStatus = "CONFIRMED"
	NotConfirmed OutcomeStatus = "NOT_CONFIRMED"
)

func ParseOutcomeStatus(raw string) (OutcomeStatus, error) {
	switch s := OutcomeStatus(raw); s {
	case Confirmed, NotConfirmed:
		return s, nil
	}
	return "", ErrInvalidOutcome
}

// InductionOutcome is the single assessment of whether an animal's disease
// model took. The induction itself is an administration.
type InductionOutcome struct {
	AllocationID, ExperimentID string
	Status                     OutcomeStatus
	AssessedAt                 time.Time
	Note, RecordedBy           string
	CreatedAt                  time.Time
}

func NormalizeNotes(raw string) (string, error) {
	s := strings.TrimSpace(raw)
	bad := func(r rune) bool { return unicode.IsControl(r) && r != '\n' && r != '\r' && r != '\t' }
	if !utf8.ValidString(s) || utf8.RuneCountInString(s) > 2000 || strings.IndexFunc(s, bad) >= 0 {
		return "", ErrInvalidNotes
	}
	return s, nil
}
