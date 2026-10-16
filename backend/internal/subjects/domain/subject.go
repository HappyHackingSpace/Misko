// Package domain holds laboratory animals and their body weights. A subject is
// registered on its own and later allocated to one group of one experiment;
// the allocation is owned by the experiments context and only read here.
package domain

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"
)

var (
	ErrInvalidCode    = errors.New("code must contain 1 to 64 ASCII letters, digits, '.', '_', '/' or '-'")
	ErrInvalidSpecies = errors.New("species must be MOUSE or RAT")
	ErrInvalidSex     = errors.New("sex must be FEMALE, MALE or UNKNOWN")
	ErrInvalidWeight  = errors.New("body weight must be between 0.001 and 5000 grams with at most 3 decimals")
	ErrMissingTime    = errors.New("a timestamp is required")
	ErrFutureTime     = errors.New("time cannot be in the future")
)

// ClockSkew tolerates small differences between client and server clocks.
const ClockSkew = 5 * time.Minute

type Species string

const (
	Mouse Species = "MOUSE"
	Rat   Species = "RAT"
)

type Sex string

const (
	Female     Sex = "FEMALE"
	Male       Sex = "MALE"
	UnknownSex Sex = "UNKNOWN"
)

type Subject struct {
	ID      string
	Code    string
	Species Species
	Sex     Sex
	// Allocation is nil until the animal is allocated to a group.
	Allocation *Allocation
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// Allocation summarises the one group an animal belongs to. It is read-only
// here; allocating, moving and excluding belong to the experiments context.
type Allocation struct {
	ExperimentID   string
	ExperimentCode string
	GroupID        string
	GroupCode      string
	GroupRole      string
	Excluded       bool
}

func NewSubject(code, species, sex string) (Subject, error) {
	var s Subject
	var err error
	if s.Code, err = NormalizeCode(code); err != nil {
		return Subject{}, err
	}
	if s.Species, err = ParseSpecies(species); err != nil {
		return Subject{}, err
	}
	if s.Sex, err = ParseSex(sex); err != nil {
		return Subject{}, err
	}
	return s, nil
}

// NormalizeCode trims a tag or cage code. Uniqueness is case-insensitive.
func NormalizeCode(raw string) (string, error) {
	code := strings.TrimSpace(raw)
	if code == "" || len(code) > 64 {
		return "", ErrInvalidCode
	}
	for _, r := range code {
		if r > unicode.MaxASCII || !(unicode.IsLetter(r) || unicode.IsDigit(r) || strings.ContainsRune("._/-", r)) {
			return "", ErrInvalidCode
		}
	}
	return code, nil
}

func ParseSpecies(raw string) (Species, error) {
	if s := Species(raw); s == Mouse || s == Rat {
		return s, nil
	}
	return "", ErrInvalidSpecies
}

func ParseSex(raw string) (Sex, error) {
	if s := Sex(raw); s == Female || s == Male || s == UnknownSex {
		return s, nil
	}
	return "", ErrInvalidSex
}

// Weight is one body weight measurement. Weights belong to the animal, not to
// a study, and never change once recorded.
type Weight struct {
	ID         string
	SubjectID  string
	Milligrams int64
	MeasuredAt time.Time
	RecordedBy string
	CreatedAt  time.Time
}

func NewWeight(subjectID, grams string, measuredAt, now time.Time, recordedBy string) (Weight, error) {
	mg, err := ParseGrams(grams)
	if err != nil {
		return Weight{}, err
	}
	if measuredAt.IsZero() {
		return Weight{}, ErrMissingTime
	}
	if measuredAt.After(now.Add(ClockSkew)) {
		return Weight{}, ErrFutureTime
	}
	return Weight{SubjectID: subjectID, Milligrams: mg, MeasuredAt: measuredAt, RecordedBy: recordedBy}, nil
}

var gramsPattern = regexp.MustCompile(`^([0-9]{1,4})(?:\.([0-9]{1,3}))?$`)

// ParseGrams parses a body weight in grams, exactly, into milligrams.
func ParseGrams(raw string) (int64, error) {
	m := gramsPattern.FindStringSubmatch(strings.TrimSpace(raw))
	if m == nil {
		return 0, ErrInvalidWeight
	}
	whole, _ := strconv.ParseInt(m[1], 10, 64)
	fraction, _ := strconv.ParseInt(m[2]+strings.Repeat("0", 3-len(m[2])), 10, 64)
	mg := whole*1000 + fraction
	if mg <= 0 || mg > 5_000_000 {
		return 0, ErrInvalidWeight
	}
	return mg, nil
}

// FormatGrams prints milligrams as grams without trailing zeros.
func FormatGrams(milligrams int64) string {
	if milligrams%1000 == 0 {
		return strconv.FormatInt(milligrams/1000, 10)
	}
	return strings.TrimRight(fmt.Sprintf("%d.%03d", milligrams/1000, milligrams%1000), "0")
}
