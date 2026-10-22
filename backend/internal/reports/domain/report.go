// Package domain computes the study report: for every endpoint of an
// experiment, descriptive statistics per group at the endpoint's timepoint and
// across timepoints, and the standard comparisons derived from group roles
// (model validity, assay sensitivity, efficacy with rescue percent, dose
// response). Reports never change research records.
//
// The experimental unit is the animal. An animal contributes one value per
// timepoint, the mean of its accepted results there, and only accepted results
// of animals that are not excluded count. Pending, rejected and unanalysed
// results are counted so a half-reviewed report is never mistaken for a final
// one.
package domain

import (
	"cmp"
	"math"
	"slices"
	"strings"
	"time"
)

// Group roles.
const (
	HealthyControl  = "HEALTHY_CONTROL"
	Sham            = "SHAM"
	DiseaseControl  = "DISEASE_CONTROL"
	PositiveControl = "POSITIVE_CONTROL"
	Treatment       = "TREATMENT"
)

// Comparison kinds.
const (
	ModelValidity    = "MODEL_VALIDITY"
	AssaySensitivity = "ASSAY_SENSITIVITY"
	Efficacy         = "EFFICACY"
)

// LowerIsBetter is the endpoint direction whose improvement is a decrease.
const LowerIsBetter = "LOWER_IS_BETTER"

type Experiment struct {
	ID, Code, Title, Status string
	Blinded                 bool
}

// Group is an arm of the study with what defines it and how many animals it
// holds. DoseMicro is the dose of its TREATMENT regimen item in millionths of
// Unit, nil when it has none.
type Group struct {
	ID, Code, Name, Role             string
	DiseaseModelID, DiseaseModelName string
	SubstanceID, SubstanceName       string
	DoseMicro                        *int64
	Unit                             string
	Allocated, Excluded              int
}

type Timepoint struct {
	ID, Name           string
	StudyDay, Position int
}

type Endpoint struct {
	ID, TimepointID, BaselineTimepointID string
	ParadigmKey, MetricKey               string
	Kind, Direction                      string
}

// Measurement is one animal filmed in one test (a test subject) for the
// endpoint's paradigm. ResultStatus is empty until a run succeeded. HasMetric
// is set when the result is accepted, the animal is not excluded and the run
// published the metric; Value is nil when the metric is missing.
type Measurement struct {
	TestSubjectID, TestID, AllocationID string
	SubjectID, SubjectCode, GroupID     string
	TimepointID, Arena                  string
	ScheduledAt                         time.Time
	Excluded                            bool
	ResultStatus                        string
	ResultJobID                         string
	ParadigmVersion                     int
	HasMetric                           bool
	Unit                                string
	Value                               *float64
	MissingReason                       string
	MetricEngineVersion                 int
}

// Version is what must be equal for values to be compared.
type Version struct {
	ParadigmVersion, MetricEngineVersion int
	Unit                                 string
}

// Descriptives of per-animal values. SD and SEM need two animals.
type Descriptives struct {
	N                       int
	Mean, SD, SEM, Min, Max *float64
}

// GroupStats describes one group at one timepoint. Tested counts animals
// filmed there; Pending, AwaitingAnalysis and Rejected count test subjects of
// animals that are not excluded; Missing counts accepted results without the
// metric value; Excluded counts the group's excluded animals. Change is the
// change from the baseline timepoint per animal, when the endpoint has one.
type GroupStats struct {
	GroupID string
	Descriptives
	Tested, Pending, AwaitingAnalysis, Rejected, Missing, Excluded int
	Change                                                         *Descriptives
}

// Comparison of a group's mean with a reference group's mean. Effect is the
// difference signed so that a positive value is better in the endpoint's
// direction. RescuePercent is (group - disease control) / (healthy reference -
// disease control) x 100, where the healthy reference is a healthy control, or
// a sham of the same disease model when the study has no healthy control.
type Comparison struct {
	Kind, GroupID, ReferenceGroupID string
	HealthyReferenceGroupID         string
	Difference, PercentDifference   *float64
	Effect, RescuePercent           *float64
}

type DosePoint struct {
	GroupID       string
	DoseMicro     int64
	N             int
	Mean          *float64
	RescuePercent *float64
}

// DoseResponse lists the treatment groups of one substance and unit in dose order.
type DoseResponse struct {
	SubstanceID, SubstanceName, Unit string
	Points                           []DosePoint
}

type TimepointStats struct {
	TimepointID string
	Groups      []GroupStats
}

// Section is the report of one endpoint. Values of different paradigm
// versions, metric engine versions or units are never pooled: Versions lists
// what the accepted results came from and, when there is more than one,
// Comparable is false and no means or comparisons are computed.
type Section struct {
	Endpoint     Endpoint
	Versions     []Version
	Comparable   bool
	Groups       []GroupStats
	Comparisons  []Comparison
	DoseResponse []DoseResponse
	TimeCourse   []TimepointStats
}

type Report struct {
	Experiment Experiment
	Groups     []Group
	Timepoints []Timepoint
	Sections   []Section
}

// Analyse computes the section of one endpoint from every measurement of its
// paradigm and metric in the experiment.
func Analyse(e Endpoint, groups []Group, timepoints []Timepoint, rows []Measurement) Section {
	section := Section{Endpoint: e, Versions: versions(rows)}
	section.Comparable = len(section.Versions) <= 1
	section.Groups = stats(e.TimepointID, e.BaselineTimepointID, groups, rows, section.Comparable)
	if section.Comparable {
		section.Comparisons = compare(e, groups, section.Groups)
		section.DoseResponse = doseResponse(groups, section.Groups)
	}
	for _, tp := range timepoints {
		if !slices.ContainsFunc(rows, func(m Measurement) bool { return m.TimepointID == tp.ID }) {
			continue
		}
		section.TimeCourse = append(section.TimeCourse, TimepointStats{TimepointID: tp.ID, Groups: stats(tp.ID, "", groups, rows, section.Comparable)})
	}
	return section
}

func versions(rows []Measurement) []Version {
	var out []Version
	for _, m := range rows {
		if !m.HasMetric {
			continue
		}
		v := Version{m.ParadigmVersion, m.MetricEngineVersion, m.Unit}
		if !slices.Contains(out, v) {
			out = append(out, v)
		}
	}
	slices.SortFunc(out, func(a, b Version) int {
		return cmp.Or(cmp.Compare(a.ParadigmVersion, b.ParadigmVersion), cmp.Compare(a.MetricEngineVersion, b.MetricEngineVersion), strings.Compare(a.Unit, b.Unit))
	})
	return out
}

// animalValues returns each animal's mean accepted value at a timepoint.
func animalValues(timepoint string, rows []Measurement) map[string]float64 {
	sums, counts := map[string]float64{}, map[string]int{}
	for _, m := range rows {
		if m.TimepointID == timepoint && !m.Excluded && m.ResultStatus == "ACCEPTED" && m.HasMetric && m.Value != nil {
			sums[m.AllocationID] += *m.Value
			counts[m.AllocationID]++
		}
	}
	out := make(map[string]float64, len(sums))
	for id, sum := range sums {
		out[id] = sum / float64(counts[id])
	}
	return out
}

func stats(timepoint, baseline string, groups []Group, rows []Measurement, comparable bool) []GroupStats {
	values := animalValues(timepoint, rows)
	var before map[string]float64
	if baseline != "" {
		before = animalValues(baseline, rows)
	}
	out := make([]GroupStats, 0, len(groups))
	for _, g := range groups {
		s := GroupStats{GroupID: g.ID, Excluded: g.Excluded}
		tested := map[string]bool{}
		var own, change []float64
		seen := map[string]bool{}
		for _, m := range rows {
			if m.TimepointID != timepoint || m.GroupID != g.ID {
				continue
			}
			tested[m.AllocationID] = true
			if m.Excluded {
				continue
			}
			switch m.ResultStatus {
			case "":
				s.AwaitingAnalysis++
			case "PENDING":
				s.Pending++
			case "REJECTED":
				s.Rejected++
			case "ACCEPTED":
				if !m.HasMetric || m.Value == nil {
					s.Missing++
				}
			}
			if v, ok := values[m.AllocationID]; ok && !seen[m.AllocationID] {
				seen[m.AllocationID] = true
				own = append(own, v)
				if b, ok := before[m.AllocationID]; ok {
					change = append(change, v-b)
				}
			}
		}
		s.Tested = len(tested)
		s.N = len(own)
		if comparable {
			s.Descriptives = describe(own)
			if baseline != "" {
				d := describe(change)
				s.Change = &d
			}
		}
		out = append(out, s)
	}
	return out
}

// describe returns n, mean, sample standard deviation, standard error,
// minimum and maximum. Values are sorted first so the result does not depend
// on row order.
func describe(values []float64) Descriptives {
	d := Descriptives{N: len(values)}
	if len(values) == 0 {
		return d
	}
	values = slices.Sorted(slices.Values(values))
	n := float64(len(values))
	sum := 0.0
	for _, v := range values {
		sum += v
	}
	mean, lo, hi := sum/n, values[0], values[len(values)-1]
	d.Mean, d.Min, d.Max = &mean, &lo, &hi
	if len(values) < 2 {
		return d
	}
	squares := 0.0
	for _, v := range values {
		squares += (v - mean) * (v - mean)
	}
	sd := math.Sqrt(squares / (n - 1))
	sem := sd / math.Sqrt(n)
	d.SD, d.SEM = &sd, &sem
	return d
}

func compare(e Endpoint, groups []Group, stats []GroupStats) []Comparison {
	mean := map[string]*float64{}
	for _, s := range stats {
		mean[s.GroupID] = s.Mean
	}
	sign := 1.0
	if e.Direction == LowerIsBetter {
		sign = -1
	}
	var out []Comparison
	add := func(kind string, g, ref Group, healthy *Group) {
		c := Comparison{Kind: kind, GroupID: g.ID, ReferenceGroupID: ref.ID}
		a, b := mean[g.ID], mean[ref.ID]
		if a != nil && b != nil {
			diff, effect := *a-*b, sign*(*a-*b)
			c.Difference, c.Effect = &diff, &effect
			if *b != 0 {
				pct := diff / math.Abs(*b) * 100
				c.PercentDifference = &pct
			}
		}
		if healthy != nil {
			c.HealthyReferenceGroupID = healthy.ID
			c.RescuePercent = rescue(a, b, mean[healthy.ID])
		}
		out = append(out, c)
	}
	for _, d := range groups {
		if d.Role != DiseaseControl {
			continue
		}
		for _, h := range groups {
			if h.Role == HealthyControl || (h.Role == Sham && h.DiseaseModelID == d.DiseaseModelID) {
				add(ModelValidity, d, h, nil)
			}
		}
	}
	for _, kind := range []struct{ role, comparison string }{{PositiveControl, AssaySensitivity}, {Treatment, Efficacy}} {
		for _, g := range groups {
			if g.Role != kind.role {
				continue
			}
			for _, d := range groups {
				if d.Role == DiseaseControl && d.DiseaseModelID == g.DiseaseModelID {
					add(kind.comparison, g, d, healthyReference(groups, d.DiseaseModelID))
				}
			}
		}
	}
	return out
}

// healthyReference is the first healthy control, else the first sham of the
// disease model.
func healthyReference(groups []Group, diseaseModel string) *Group {
	for _, role := range []string{HealthyControl, Sham} {
		for i, g := range groups {
			if g.Role == role && (role == HealthyControl || g.DiseaseModelID == diseaseModel) {
				return &groups[i]
			}
		}
	}
	return nil
}

func rescue(treated, disease, healthy *float64) *float64 {
	if treated == nil || disease == nil || healthy == nil || *healthy == *disease {
		return nil
	}
	r := (*treated - *disease) / (*healthy - *disease) * 100
	return &r
}

func doseResponse(groups []Group, stats []GroupStats) []DoseResponse {
	byGroup := map[string]GroupStats{}
	for _, s := range stats {
		byGroup[s.GroupID] = s
	}
	var out []DoseResponse
	index := map[[2]string]int{}
	for _, g := range groups {
		if g.Role != Treatment || g.SubstanceID == "" || g.DoseMicro == nil {
			continue
		}
		key := [2]string{g.SubstanceID, g.Unit}
		i, ok := index[key]
		if !ok {
			i = len(out)
			index[key] = i
			out = append(out, DoseResponse{SubstanceID: g.SubstanceID, SubstanceName: g.SubstanceName, Unit: g.Unit})
		}
		s := byGroup[g.ID]
		point := DosePoint{GroupID: g.ID, DoseMicro: *g.DoseMicro, N: s.N, Mean: s.Mean}
		for _, d := range groups {
			if d.Role == DiseaseControl && d.DiseaseModelID == g.DiseaseModelID {
				if h := healthyReference(groups, d.DiseaseModelID); h != nil {
					point.RescuePercent = rescue(s.Mean, byGroup[d.ID].Mean, byGroup[h.ID].Mean)
				}
				break
			}
		}
		out[i].Points = append(out[i].Points, point)
	}
	// A dose response needs at least two dose levels of one substance.
	out = slices.DeleteFunc(out, func(d DoseResponse) bool { return len(d.Points) < 2 })
	for i := range out {
		slices.SortStableFunc(out[i].Points, func(a, b DosePoint) int { return cmp.Compare(a.DoseMicro, b.DoseMicro) })
	}
	return out
}
