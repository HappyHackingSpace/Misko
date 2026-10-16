package domain

import (
	"math"
	"slices"
	"strings"
	"unicode"
)

// Shared wording for trajectory-based definitions.
var (
	trackedPair = T("A counted interval is a pair of consecutive tracked samples separated by 0 < dt <= max_sample_gap_s.",
		"Sayılan aralık, 0 < dt <= max_sample_gap_s olan iki ardışık izlenmiş örnekten oluşur.")
	missingNoPairs = T("Missing (NO_VALID_INTERVALS) when there are no counted intervals.",
		"Sayılan aralık yoksa eksik (NO_VALID_INTERVALS).")
)

var gapParameter = Parameter{
	Key: "max_sample_gap_s", Scope: AnalysisScope, Unit: Second, Min: 0.01, Max: 5, Default: 0.5,
	Label: Text{"Maximum tracking gap", "İzleme boşluğu toleransı"},
	Help: Text{
		"Longest tracking dropout that is bridged; movement across a longer gap is excluded. Increase only for low-frame-rate videos.",
		"Hayvan bundan daha uzun süre kaybolursa, aradaki hareket hesaba katılmaz. Yalnızca düşük kare hızlı videolarda artırın.",
	},
}

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

// track summarizes counted intervals: pairs of consecutive tracked samples
// with 0 < dt <= the maximum gap. Zone time is attributed to the earlier
// sample. Intervals are named "in_<zone>" and entry points "<zone>_entry".
type track struct {
	durationUs int64
	zoneUs     map[string]int64
	entries    map[string]int
	intervals  []Interval
	points     []Point
}

// walk evaluates a trajectory; visit, when set, is called for every counted interval.
func walk(samples []Sample, tracked func(Sample) bool, zones []zone, maxGapUs int64, visit func(a, b Sample, dt int64)) track {
	t := track{zoneUs: map[string]int64{}, entries: map[string]int{}}
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

// occupancyEvents and entryEvents describe the derived events of the named
// zones, labelled after the zone.
func occupancyEvents(zones []Zone, keys ...string) []EventDefinition {
	var out []EventDefinition
	for _, k := range keys {
		z := zoneByKey(zones, k)
		out = append(out, EventDefinition{
			Type: "in_" + k, Kind: IntervalEvent,
			Definition: T("Run of contiguous counted intervals whose earlier sample lies in the "+strings.ToLower(z.Label.EN)+".",
				"Önceki örneği "+lowerTR(z.Label.TR)+" içinde olan bitişik sayılan aralıklar."),
			Label: T(z.Label.EN+" visit", z.Label.TR+" ziyareti"),
		})
	}
	return out
}

func entryEvents(zones []Zone, keys ...string) []EventDefinition {
	var out []EventDefinition
	for _, k := range keys {
		z := zoneByKey(zones, k)
		out = append(out, EventDefinition{
			Type: k + "_entry", Kind: PointEvent,
			Definition: T("Time of the later sample of a counted interval that enters the "+strings.ToLower(z.Label.EN)+".",
				capitalTR(z.Label.TR)+" bölgesine giren sayılan aralığın sonraki örneğinin zamanı."),
			Label: T(z.Label.EN+" entry", z.Label.TR+" girişi"),
		})
	}
	return out
}

// lowerTR lowercases Turkish wording for use inside a sentence (I -> ı, İ -> i).
func lowerTR(s string) string {
	return strings.ToLowerSpecial(unicode.TurkishCase, s)
}

// capitalTR keeps the wording as written but starts it in upper case, for
// the start of a sentence.
func capitalTR(s string) string {
	if s == "" {
		return s
	}
	r := []rune(s)
	return strings.ToUpperSpecial(unicode.TurkishCase, string(r[0])) + string(r[1:])
}

func zoneByKey(zones []Zone, key string) Zone {
	i := slices.IndexFunc(zones, func(z Zone) bool { return z.Key == key })
	if i < 0 {
		panic("paradigm catalog: unknown zone " + key)
	}
	return zones[i]
}

func gapUs(params map[string]float64) int64 {
	return int64(math.Round(params["max_sample_gap_s"] * 1e6))
}

func seconds(us int64) float64 { return float64(us) / 1e6 }

func newComputation() computation {
	return computation{values: map[string]float64{}, missing: map[string]string{}}
}

// T builds a Text from its English and Turkish wording.
func T(en, tr string) Text { return Text{EN: en, TR: tr} }

func param(key string, scope Scope, label, help Text, unit Unit, min, max, def float64) Parameter {
	return Parameter{Key: key, Scope: scope, Label: label, Help: help, Unit: unit, Min: min, Max: max, Default: def}
}

func integerParam(key string, scope Scope, label, help Text, unit Unit, min, max, def float64) Parameter {
	return Parameter{Key: key, Scope: scope, Label: label, Help: help, Unit: unit, Integer: true, Min: min, Max: max, Default: def}
}

func trialType(key string, label Text) TrialType { return TrialType{Key: key, Label: label} }

// metric builds a definition with the tolerance used for its unit: 1e-9 for
// counts, booleans and ratios, 1e-6 otherwise.
func metric(key string, label, description Text, unit Unit, max float64, inputs []string, definition Text, formula string, missingWhen Text) MetricDefinition {
	m := MetricDefinition{Key: key, Label: label, Description: description, Unit: unit, Max: max, Inputs: inputs, Definition: definition, Formula: formula, MissingWhen: missingWhen, Tolerance: 1e-6}
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
