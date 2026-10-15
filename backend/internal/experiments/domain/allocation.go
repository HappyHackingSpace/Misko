package domain

import (
	"errors"
	"strings"
	"time"
)

var (
	ErrInvalidCage               = errors.New("cage must contain 1 to 32 printable characters")
	ErrInvalidExclusionReason    = errors.New("exclusion reason must be DIED, HUMANE_ENDPOINT, INDUCTION_FAILED, TECHNICAL or OTHER")
	ErrInvalidExclusionNote      = errors.New("exclusion note must contain at most 2000 characters")
	ErrAlreadyExcluded           = errors.New("the animal is already excluded")
	ErrExclusionBeforeAllocation = errors.New("an exclusion cannot precede the allocation")
	ErrFutureTime                = errors.New("time cannot be in the future")
	ErrMissingTime               = errors.New("a timestamp is required")
)

// ClockSkew tolerates small differences between client and server clocks.
const ClockSkew = 5 * time.Minute

type ExclusionReason string

const (
	Died            ExclusionReason = "DIED"
	HumaneEndpoint  ExclusionReason = "HUMANE_ENDPOINT"
	InductionFailed ExclusionReason = "INDUCTION_FAILED"
	Technical       ExclusionReason = "TECHNICAL"
	OtherReason     ExclusionReason = "OTHER"
)

func ParseExclusionReason(raw string) (ExclusionReason, error) {
	switch r := ExclusionReason(raw); r {
	case Died, HumaneEndpoint, InductionFailed, Technical, OtherReason:
		return r, nil
	}
	return "", ErrInvalidExclusionReason
}

// Allocation places an animal in one group, once and for good. While the study
// is a draft the animal may move or leave; once it runs only the cage and a
// single exclusion can be recorded.
type Allocation struct {
	ID              string
	ExperimentID    string
	GroupID         string
	SubjectID       string
	AllocatedAt     time.Time
	Cage            string
	ExcludedAt      *time.Time
	ExclusionReason ExclusionReason
	ExclusionNote   string
	CreatedBy       string
	CreatedAt       time.Time
}

func (a Allocation) Excluded() bool { return a.ExcludedAt != nil }

// NewAllocation checks a new allocation; a zero allocatedAt means now.
func NewAllocation(experimentID, groupID, subjectID, cage string, allocatedAt, now time.Time, createdBy string) (Allocation, error) {
	a := Allocation{ExperimentID: experimentID, GroupID: strings.TrimSpace(groupID), SubjectID: strings.TrimSpace(subjectID), CreatedBy: createdBy}
	var err error
	if a.Cage, err = NormalizeCage(cage); err != nil {
		return Allocation{}, err
	}
	if allocatedAt.IsZero() {
		allocatedAt = now
	}
	if allocatedAt.After(now.Add(ClockSkew)) {
		return Allocation{}, ErrFutureTime
	}
	a.AllocatedAt = Instant(allocatedAt)
	return a, nil
}

// Exclude records why an animal leaves the analysis. It happens once; a zero
// at means now.
func (a *Allocation) Exclude(reason, note string, at, now time.Time) error {
	if a.Excluded() {
		return ErrAlreadyExcluded
	}
	r, err := ParseExclusionReason(reason)
	if err != nil {
		return err
	}
	n, err := text(note, 2000, ErrInvalidExclusionNote)
	if err != nil {
		return err
	}
	if at.IsZero() {
		at = now
	}
	if at.After(now.Add(ClockSkew)) {
		return ErrFutureTime
	}
	at = Instant(at)
	if at.Before(a.AllocatedAt) {
		return ErrExclusionBeforeAllocation
	}
	a.ExcludedAt, a.ExclusionReason, a.ExclusionNote = &at, r, n
	return nil
}

// NormalizeCage returns "" for no cage.
func NormalizeCage(raw string) (string, error) {
	if strings.TrimSpace(raw) == "" {
		return "", nil
	}
	return line(raw, 32, ErrInvalidCage)
}

// Instant stores times in UTC at the database's microsecond precision.
func Instant(t time.Time) time.Time { return t.UTC().Truncate(time.Microsecond) }

// Capacity is how many more animals a group can take before reaching its
// target size.
type Capacity struct {
	GroupID string
	Free    int
}

// PlanRandomisation assigns each subject, in a random order, to a group with
// the most free places left, choosing at random among ties, so groups fill
// toward their targets evenly. intn(n) returns a uniform integer in [0, n).
// It returns nil when the free places cannot hold every subject.
func PlanRandomisation(subjects []string, groups []Capacity, intn func(int) int) map[string]string {
	free := 0
	left := make([]Capacity, 0, len(groups))
	for _, g := range groups {
		if g.Free > 0 {
			free += g.Free
			left = append(left, g)
		}
	}
	if free < len(subjects) {
		return nil
	}
	order := append([]string(nil), subjects...)
	for i := len(order) - 1; i > 0; i-- {
		j := intn(i + 1)
		order[i], order[j] = order[j], order[i]
	}
	plan := make(map[string]string, len(order))
	for _, subject := range order {
		most := 0
		for _, g := range left {
			most = max(most, g.Free)
		}
		var ties []int
		for i, g := range left {
			if g.Free == most {
				ties = append(ties, i)
			}
		}
		chosen := ties[intn(len(ties))]
		plan[subject] = left[chosen].GroupID
		left[chosen].Free--
	}
	return plan
}
