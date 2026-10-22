package domain

import (
	"errors"
	"time"
)

var ErrInvalidDate = errors.New("date must be a calendar date in the form YYYY-MM-DD")

// Frequency is how often a regimen item is given between its start and end
// study days.
type Frequency string

const (
	Once       Frequency = "ONCE"
	Daily      Frequency = "DAILY"
	TwiceDaily Frequency = "TWICE_DAILY"
	Weekly     Frequency = "WEEKLY"
	EveryNDays Frequency = "EVERY_N_DAYS"
)

// RegimenItem is what a group is planned to receive. Days are study days
// relative to day 0; IntervalDays is set only for EVERY_N_DAYS.
type RegimenItem struct {
	ID, ExperimentID, GroupID  string
	Purpose                    Purpose
	SubstanceID, SubstanceName string
	Dose                       Dose
	Route                      Route
	Frequency                  Frequency
	IntervalDays               int
	StartDay, EndDay           int
}

// DosesOn returns how many doses of the item are due on a study day: none
// outside [StartDay, EndDay], two a day for TWICE_DAILY, and for WEEKLY and
// EVERY_N_DAYS one on the start day and every 7 or IntervalDays days after.
func (r RegimenItem) DosesOn(studyDay int) int {
	if studyDay < r.StartDay || studyDay > r.EndDay {
		return 0
	}
	offset := studyDay - r.StartDay
	switch r.Frequency {
	case Once:
		if offset == 0 {
			return 1
		}
	case Daily:
		return 1
	case TwiceDaily:
		return 2
	case Weekly:
		if offset%7 == 0 {
			return 1
		}
	case EveryNDays:
		if r.IntervalDays > 0 && offset%r.IntervalDays == 0 {
			return 1
		}
	}
	return 0
}

// ParseDate parses a calendar date into midnight UTC.
func ParseDate(raw string) (time.Time, error) {
	d, err := time.Parse(time.DateOnly, raw)
	if err != nil {
		return time.Time{}, ErrInvalidDate
	}
	return d, nil
}

// StudyDay is the number of calendar days from day 0 to day; day 0 itself is
// study day 0 and the day before it is -1. Both are calendar dates.
func StudyDay(dayZero, day time.Time) int {
	a := time.Date(dayZero.Year(), dayZero.Month(), dayZero.Day(), 0, 0, 0, 0, time.UTC)
	b := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, time.UTC)
	return int(b.Sub(a).Hours() / 24)
}

// DueCandidate is one regimen item of one allocated animal's group, with the
// number of doses of it given on the day in question and the animal's latest
// weight measured by the end of that day.
type DueCandidate struct {
	Allocation  Allocation
	SubjectCode string
	GroupCode   string
	Cage        string
	Excluded    bool
	Weight      *Weight
	Item        RegimenItem
	Given       int
}

// DueDose is one regimen item due for an animal on a day. AbsoluteDose is the
// amount to give for the animal's latest weight; nil when a per-kilogram dose
// has no weight to scale it.
type DueDose struct {
	Item         RegimenItem
	Due, Given   int
	AbsoluteDose *Dose
}

// Complete reports whether every dose due was given.
func (d DueDose) Complete() bool { return d.Given >= d.Due }

// DueAnimal lists the doses due for one animal on a day.
type DueAnimal struct {
	Allocation  Allocation
	SubjectCode string
	GroupCode   string
	Cage        string
	Excluded    bool
	Weight      *Weight
	Doses       []DueDose
}

// Schedule is every dose due on one calendar day of a study.
type Schedule struct {
	Date     time.Time
	StudyDay int
	Animals  []DueAnimal
}

// BuildSchedule keeps the candidates with a dose due on day and groups them
// per animal in the order given. Animals with nothing due are left out.
func BuildSchedule(dayZero, day time.Time, candidates []DueCandidate) Schedule {
	s := Schedule{Date: day, StudyDay: StudyDay(dayZero, day), Animals: []DueAnimal{}}
	index := map[string]int{}
	for _, c := range candidates {
		due := c.Item.DosesOn(s.StudyDay)
		if due == 0 {
			continue
		}
		dose := DueDose{Item: c.Item, Due: due, Given: c.Given}
		if !c.Item.Dose.Unit.PerBodyMass() {
			abs := c.Item.Dose
			dose.AbsoluteDose = &abs
		} else if c.Weight != nil {
			if abs, err := c.Item.Dose.Absolute(c.Weight.Milligrams); err == nil {
				dose.AbsoluteDose = &abs
			}
		}
		i, ok := index[c.Allocation.ID]
		if !ok {
			i = len(s.Animals)
			index[c.Allocation.ID] = i
			s.Animals = append(s.Animals, DueAnimal{
				Allocation: c.Allocation, SubjectCode: c.SubjectCode, GroupCode: c.GroupCode,
				Cage: c.Cage, Excluded: c.Excluded, Weight: c.Weight,
			})
		}
		s.Animals[i].Doses = append(s.Animals[i].Doses, dose)
	}
	return s
}
