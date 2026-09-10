package domain

import (
	"math"
	"slices"
	"strings"
)

// Shared wording for trajectory-based definitions.
const (
	trackedPair     = "A counted interval is a pair of consecutive tracked samples with 0 < dt <= max_sample_gap_s."
	calibratedFrame = "Samples and calibrated zone shapes use the same coordinates in centimeters; zones are evaluated independently and include their boundary."
	entryRule       = "An entry is a counted interval whose earlier sample is outside the zone and whose later sample is inside."
	latencyRule     = "Time of the first tracked sample inside the zone, from the trial start."
	missingNotSeen  = "Missing (EVENT_NOT_OBSERVED) when no tracked sample is inside the zone; missing (NO_VALID_INTERVALS) without counted intervals."
)

var gapParameter = Parameter{Key: "max_sample_gap_s", Label: "Longest gap between samples that still counts as tracked", Unit: Second, Min: 0.01, Max: 5, Default: 0.5}

var trajectoryQC = []QCRule{
	{Key: "min_tracking_confidence", Operator: ">=", Value: 0.7, Unit: Ratio},
	{Key: "max_lost_frame_ratio", Operator: "<=", Value: 0.1, Unit: Ratio},
	{Key: "max_calibration_error_cm", Operator: "<=", Value: 2, Unit: Centimeter},
}

var scoringQC = []QCRule{{Key: "max_event_time_uncertainty_s", Operator: "<=", Value: 0.5, Unit: Second}}

type zone struct {
	key      string
	contains func(x, y float64) bool
}

func shapesZone(key string, shapes []Shape) zone {
	return zone{key: key, contains: func(x, y float64) bool { return inAny(shapes, x, y) }}
}

type zoneEntry struct {
	key  string
	atUs int64
}

// track summarizes counted intervals: pairs of consecutive tracked samples
// with 0 < dt <= the maximum gap. Zone time is attributed to the earlier
// sample. Intervals are named "in_<zone>" and entry points "<zone>_entry".
type track struct {
	durationUs    int64
	distance      float64
	zoneUs        map[string]int64
	entries       map[string]int
	firstInsideUs map[string]int64
	sequence      []zoneEntry
	intervals     []Interval
	points        []Point
}

// walk evaluates a trajectory; visit, when set, is called for every counted interval.
func walk(samples []Sample, tracked func(Sample) bool, zones []zone, maxGapUs int64, visit func(a, b Sample, dt int64)) track {
	t := track{zoneUs: map[string]int64{}, entries: map[string]int{}, firstInsideUs: map[string]int64{}}
	for _, s := range samples {
		if !tracked(s) {
			continue
		}
		for _, z := range zones {
			if _, seen := t.firstInsideUs[z.key]; !seen && z.contains(s.X, s.Y) {
				t.firstInsideUs[z.key] = s.TUs
			}
		}
	}
	open := map[string]*Interval{}
	closeRun := func(key string) {
		if run := open[key]; run != nil {
			t.intervals = append(t.intervals, *run)
			delete(open, key)
		}
	}
	for i := 1; i < len(samples); i++ {
		a, b := samples[i-1], samples[i]
		dt := b.TUs - a.TUs
		if !tracked(a) || !tracked(b) || dt > maxGapUs {
			for _, z := range zones {
				closeRun(z.key)
			}
			continue
		}
		t.durationUs += dt
		t.distance += math.Hypot(b.X-a.X, b.Y-a.Y)
		for _, z := range zones {
			if z.contains(a.X, a.Y) {
				t.zoneUs[z.key] += dt
				if run := open[z.key]; run != nil && run.EndUs == a.TUs {
					run.EndUs = b.TUs
				} else {
					closeRun(z.key)
					open[z.key] = &Interval{Type: "in_" + z.key, StartUs: a.TUs, EndUs: b.TUs}
				}
				continue
			}
			closeRun(z.key)
			if z.contains(b.X, b.Y) {
				t.entries[z.key]++
				t.sequence = append(t.sequence, zoneEntry{key: z.key, atUs: b.TUs})
				t.points = append(t.points, Point{Type: z.key + "_entry", AtUs: b.TUs})
			}
		}
		if visit != nil {
			visit(a, b, dt)
		}
	}
	for _, z := range zones {
		closeRun(z.key)
	}
	return t
}

// events keeps the occupancy intervals of intervalZones and the entry points of pointZones.
func (t track) events(intervalZones, pointZones []string) ([]Interval, []Point) {
	var intervals []Interval
	var points []Point
	for _, i := range t.intervals {
		if slices.Contains(intervalZones, strings.TrimPrefix(i.Type, "in_")) {
			intervals = append(intervals, i)
		}
	}
	for _, p := range t.points {
		if slices.Contains(pointZones, strings.TrimSuffix(p.Type, "_entry")) {
			points = append(points, p)
		}
	}
	return intervals, points
}

func occupancyEvents(zones ...string) []EventDefinition {
	var out []EventDefinition
	for _, z := range zones {
		out = append(out, EventDefinition{Type: "in_" + z, Kind: IntervalEvent, Definition: "Contiguous counted intervals whose earlier sample is in " + z + "."})
	}
	return out
}

func entryEvents(zones ...string) []EventDefinition {
	var out []EventDefinition
	for _, z := range zones {
		out = append(out, EventDefinition{Type: z + "_entry", Kind: PointEvent, Definition: "Later sample time of a counted interval entering " + z + "."})
	}
	return out
}

func validOnly(s Sample) bool { return s.Valid }

func gapUs(params map[string]float64) int64 {
	return int64(math.Round(params["max_sample_gap_s"] * 1e6))
}

func seconds(us int64) float64 { return float64(us) / 1e6 }

func newComputation() computation {
	return computation{values: map[string]float64{}, missing: map[string]string{}}
}

func param(key, label string, unit Unit, min, max, def float64) Parameter {
	return Parameter{Key: key, Label: label, Unit: unit, Min: min, Max: max, Default: def}
}

func integerParam(key, label string, unit Unit, min, max, def float64) Parameter {
	return Parameter{Key: key, Label: label, Unit: unit, Integer: true, Min: min, Max: max, Default: def}
}

// metric builds a definition with the tolerance used for its unit: 1e-9 for
// counts, booleans and ratios, 1e-6 otherwise.
func metric(key, label string, unit Unit, max float64, inputs []string, definition, formula, missingWhen string) MetricDefinition {
	m := MetricDefinition{Key: key, Label: label, Unit: unit, Max: max, Inputs: inputs, Definition: definition, Formula: formula, MissingWhen: missingWhen, Tolerance: 1e-6}
	switch unit {
	case Count, Boolean:
		m.Integer, m.Tolerance = true, 1e-9
	case Ratio:
		m.Tolerance = 1e-9
	}
	return m
}

func firstEvent(events []ObservedEvent, typ string) (ObservedEvent, bool) {
	i := slices.IndexFunc(events, func(e ObservedEvent) bool { return e.Type == typ })
	if i < 0 {
		return ObservedEvent{}, false
	}
	return events[i], true
}

func echoPoints(events []ObservedEvent, types ...string) []Point {
	var out []Point
	for _, e := range events {
		if slices.Contains(types, e.Type) {
			out = append(out, Point{Type: e.Type, AtUs: e.StartUs})
		}
	}
	return out
}
