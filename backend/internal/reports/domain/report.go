// Package domain holds read-only report rows and the rules for comparing
// metric results between groups. Reports never change research records.
package domain

import (
	"cmp"
	"errors"
	"fmt"
	"math"
	"slices"
	"strings"
	"time"
)

var (
	ErrIncompatibleVersions = errors.New("metric results come from different paradigms, paradigm versions, metric engine versions or units")
	ErrMixedMetrics         = errors.New("a summary covers exactly one metric")
	ErrDuplicateTest        = errors.New("a test is reported more than once for one metric")
)

// Provenance identifies the experiment, subject, test and analysis run a row
// comes from. Group and phase are the ones stored on the test.
type Provenance struct {
	RunID               string
	Latest              bool
	Trigger             string
	ModelVersion        string
	MetricEngineVersion int
	ResultSchemaVersion int
	CalibrationID       string
	RecordingID         string
	SourceVideoID       string
	RunFinishedAt       time.Time
	TestID              string
	TestStatus          string
	ScheduledAt         time.Time
	TrialCount          int
	ExperimentID        string
	ExperimentCode      string
	SubjectID           string
	SubjectCode         string
	Species             string
	Sex                 string
	PhaseID             string
	PhaseName           string
	GroupID             string
	GroupName           string
	GroupRole           string
	ParadigmKey         string
	ParadigmVersion     int
	EnvironmentID       string
	EnvironmentName     string
	EnvironmentRevision int
}

// MetricRow is one published metric of a run. Value is nil exactly when the
// worker reported a missing reason.
type MetricRow struct {
	Provenance
	MetricKey     string
	Unit          string
	Value         *float64
	MissingReason string
}

// EventRow is one published event of a run with the trial it belongs to, if any.
type EventRow struct {
	Provenance
	EventID         string
	EventType       string
	Kind            string
	StartUs         int64
	EndUs           int64
	Confidence      float64
	TrialID         string
	TrialNumber     int
	TrialRepetition int
	TrialAttempt    int
}

// Version is what must be equal for metric values to be compared.
type Version struct {
	ParadigmKey         string
	ParadigmVersion     int
	MetricEngineVersion int
	Unit                string
}

func (v Version) String() string {
	return fmt.Sprintf("%s v%d engine %d (%s)", v.ParadigmKey, v.ParadigmVersion, v.MetricEngineVersion, v.Unit)
}

// IncompatibleError lists the versions found when they differ.
type IncompatibleError struct {
	Versions []Version
}

func (e IncompatibleError) Error() string {
	names := make([]string, 0, len(e.Versions))
	for _, v := range e.Versions {
		names = append(names, v.String())
	}
	return fmt.Sprintf("%s: %s; filter by paradigm version and metric engine version", ErrIncompatibleVersions, strings.Join(names, ", "))
}

func (e IncompatibleError) Unwrap() error { return ErrIncompatibleVersions }

// GroupSummary describes one group. Subjects is the n of the statistics: the
// number of subjects with at least one value.
type GroupSummary struct {
	GroupID         string
	GroupName       string
	GroupRole       string
	Tests           int
	Subjects        int
	MissingSubjects int
	MissingReasons  map[string]int
	Mean            *float64
	SD              *float64
	SEM             *float64
	Min             *float64
	Max             *float64
}

type Summary struct {
	MetricKey string
	Version   *Version
	Groups    []GroupSummary
}

// Summarize compares groups on one metric. Each subject contributes a single
// value, the mean of its tests, because repeated tests and trials of one animal
// are not independent observations. Missing results are counted per reason and
// never treated as zero.
func Summarize(metricKey string, rows []MetricRow) (Summary, error) {
	summary := Summary{MetricKey: metricKey, Groups: []GroupSummary{}}
	var versions []Version
	for _, r := range rows {
		if r.MetricKey != metricKey {
			return Summary{}, ErrMixedMetrics
		}
		v := Version{r.ParadigmKey, r.ParadigmVersion, r.MetricEngineVersion, r.Unit}
		if !slices.Contains(versions, v) {
			versions = append(versions, v)
		}
	}
	if len(versions) > 1 {
		slices.SortFunc(versions, func(a, b Version) int {
			return cmp.Or(strings.Compare(a.ParadigmKey, b.ParadigmKey), cmp.Compare(a.ParadigmVersion, b.ParadigmVersion),
				cmp.Compare(a.MetricEngineVersion, b.MetricEngineVersion), strings.Compare(a.Unit, b.Unit))
		})
		return Summary{}, IncompatibleError{Versions: versions}
	}
	if len(versions) == 1 {
		summary.Version = &versions[0]
	}

	type subject struct {
		sum      float64
		values   int
		observed bool
	}
	type group struct {
		GroupSummary
		subjects map[string]*subject
		order    []string
	}
	groups := map[string]*group{}
	tests := map[string]bool{}
	for _, r := range rows {
		// Checked after versions: one test may have a latest run per engine version.
		if tests[r.TestID] {
			return Summary{}, ErrDuplicateTest
		}
		tests[r.TestID] = true
		g := groups[r.GroupID]
		if g == nil {
			g = &group{GroupSummary: GroupSummary{GroupID: r.GroupID, GroupName: r.GroupName, GroupRole: r.GroupRole, MissingReasons: map[string]int{}}, subjects: map[string]*subject{}}
			groups[r.GroupID] = g
		}
		g.Tests++
		s := g.subjects[r.SubjectID]
		if s == nil {
			s = &subject{}
			g.subjects[r.SubjectID] = s
			g.order = append(g.order, r.SubjectID)
		}
		s.observed = true
		if r.Value == nil {
			g.MissingReasons[r.MissingReason]++
			continue
		}
		s.sum += *r.Value
		s.values++
	}
	for _, g := range groups {
		var values []float64
		for _, id := range g.order {
			s := g.subjects[id]
			if s.values == 0 {
				g.MissingSubjects++
				continue
			}
			values = append(values, s.sum/float64(s.values))
		}
		g.Subjects = len(values)
		describe(&g.GroupSummary, values)
		summary.Groups = append(summary.Groups, g.GroupSummary)
	}
	slices.SortFunc(summary.Groups, func(a, b GroupSummary) int {
		return cmp.Or(cmp.Compare(rank(a), rank(b)), strings.Compare(strings.ToLower(a.GroupName), strings.ToLower(b.GroupName)), strings.Compare(a.GroupID, b.GroupID))
	})
	return summary, nil
}

// rank orders control groups first and tests without a group last.
func rank(g GroupSummary) int {
	switch {
	case g.GroupID == "":
		return 2
	case g.GroupRole == "CONTROL":
		return 0
	}
	return 1
}

// describe sets the mean, sample standard deviation, standard error, minimum
// and maximum. The deviation needs at least two values.
func describe(g *GroupSummary, values []float64) {
	if len(values) == 0 {
		return
	}
	n := float64(len(values))
	sum, lo, hi := 0.0, values[0], values[0]
	for _, v := range values {
		sum += v
		lo, hi = min(lo, v), max(hi, v)
	}
	mean := sum / n
	g.Mean, g.Min, g.Max = &mean, &lo, &hi
	if len(values) < 2 {
		return
	}
	squares := 0.0
	for _, v := range values {
		squares += (v - mean) * (v - mean)
	}
	sd := math.Sqrt(squares / (n - 1))
	sem := sd / math.Sqrt(n)
	g.SD, g.SEM = &sd, &sem
}
