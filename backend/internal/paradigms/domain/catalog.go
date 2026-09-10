// Package domain defines the hardcoded paradigm catalog and metric engine.
// Definitions are code and versioned. There is no editable catalog: callers
// always receive copies, and a published version is never changed.
package domain

import (
	"cmp"
	"errors"
	"fmt"
	"maps"
	"math"
	"slices"
)

const (
	// MetricEngineVersion changes whenever an existing calculation changes, so
	// results computed by different engines are never silently compared.
	MetricEngineVersion = 1
	// ResultSchemaVersion changes whenever the shape of a result changes.
	ResultSchemaVersion = 1
)

// Reasons a metric is reported missing. Missing is never zero.
const (
	ReasonNoValidIntervals    = "NO_VALID_INTERVALS"
	ReasonNotObserved         = "EVENT_NOT_OBSERVED"
	ReasonNotScored           = "NOT_SCORED"
	ReasonZeroDenominator     = "ZERO_DENOMINATOR"
	ReasonInsufficientEntries = "INSUFFICIENT_ENTRIES"
	ReasonIncompleteRecording = "INCOMPLETE_RECORDING"
	ReasonNotApplicable       = "NOT_APPLICABLE"
	ReasonZoneNotProvided     = "ZONE_NOT_PROVIDED"
	ReasonBelowCriterion      = "BELOW_EXPLORATION_CRITERION"
)

var (
	ErrUnknownParadigm     = errors.New("unknown paradigm")
	ErrUnknownVersion      = errors.New("unknown paradigm version")
	ErrUnknownParameter    = errors.New("unknown parameter")
	ErrParameterOutOfRange = errors.New("parameter is outside its allowed range")
	ErrInvalidSample       = errors.New("samples need non-negative, strictly increasing timestamps and finite tracked coordinates")
	ErrInvalidEvent        = errors.New("observed events must be scored, declared, correctly shaped, within the recording and not repeated when single")
	ErrMissingZone         = errors.New("a required calibrated zone is missing")
	ErrUnknownZone         = errors.New("unknown calibrated zone")
	ErrInvalidZone         = errors.New("a zone shape needs either a positive finite radius or at least three finite vertices")
)

type Unit string

const (
	Centimeter          Unit = "cm"
	CentimeterPerSecond Unit = "cm_s"
	Second              Unit = "s"
	Count               Unit = "count"
	Ratio               Unit = "ratio"
	Rpm                 Unit = "rpm"
	Boolean             Unit = "boolean"
)

type Category string

const (
	LearningMemory Category = "LEARNING_MEMORY"
	Anxiety        Category = "ANXIETY"
	Motor          Category = "MOTOR"
	Social         Category = "SOCIAL"
)

// Parameter is a user-chosen physical or session value with an allowed range.
// Integer parameters accept whole numbers only; booleans are 0 or 1.
type Parameter struct {
	Key, Label        string
	Unit              Unit
	Integer           bool
	Min, Max, Default float64
}

// Zone is derived from parameters, or Calibrated when its shapes come with the
// input (for example from per-video calibration).
type Zone struct {
	Key, Role, Geometry  string
	Calibrated, Required bool
}

type EventKind string

const (
	IntervalEvent EventKind = "INTERVAL"
	PointEvent    EventKind = "POINT"
)

// EventDefinition describes an output event, or an observed input event.
// Single input events may occur at most once; Labels, when set, are required.
type EventDefinition struct {
	Type       string
	Kind       EventKind
	Labels     []string
	Single     bool
	Definition string
}

// MetricDefinition documents a metric's meaning, inputs, formula, missing-data
// rule and the absolute tolerance used when validating a result.
type MetricDefinition struct {
	Key, Label                       string
	Unit                             Unit
	Integer                          bool
	Definition, Formula, MissingWhen string
	Inputs                           []string
	Min, Max, Tolerance              float64
}

type QCRule struct {
	Key, Operator string
	Value         float64
	Unit          Unit
}

type Paradigm struct {
	Key, Name                              string
	Category                               Category
	Species, TrialTypes                    []string
	Version                                int
	ApparatusParameters, SessionParameters []Parameter
	Zones                                  []Zone
	// InputEvents are observations the engine consumes; Events are what it derives.
	InputEvents []EventDefinition
	Events      []EventDefinition
	Metrics     []MetricDefinition
	QC          []QCRule
}

func (p Paradigm) clone() Paradigm {
	p.Species, p.TrialTypes = slices.Clone(p.Species), slices.Clone(p.TrialTypes)
	p.ApparatusParameters, p.SessionParameters = slices.Clone(p.ApparatusParameters), slices.Clone(p.SessionParameters)
	p.Zones, p.QC = slices.Clone(p.Zones), slices.Clone(p.QC)
	p.InputEvents, p.Events = cloneEvents(p.InputEvents), cloneEvents(p.Events)
	p.Metrics = slices.Clone(p.Metrics)
	for i := range p.Metrics {
		p.Metrics[i].Inputs = slices.Clone(p.Metrics[i].Inputs)
	}
	return p
}

func cloneEvents(events []EventDefinition) []EventDefinition {
	out := slices.Clone(events)
	for i := range out {
		out[i].Labels = slices.Clone(out[i].Labels)
	}
	return out
}

// Sample is one tracked position in centimeters. TUs is microseconds from the
// trial start. Lost frames have Valid false and their coordinates are ignored.
type Sample struct {
	TUs   int64
	X, Y  float64
	Valid bool
}

type Vertex struct{ X, Y float64 }

type Circle struct{ X, Y, Radius float64 }

// Shape is either a circle or a polygon, in the same coordinates as the samples.
type Shape struct {
	Circle  *Circle
	Polygon []Vertex
}

// ObservedEvent is a scored observation in microseconds from the trial start.
// Point events have EndUs equal to StartUs; intervals cover [StartUs, EndUs).
type ObservedEvent struct {
	Type, Label    string
	StartUs, EndUs int64
}

type Input struct {
	// Parameters override defaults; unknown keys are rejected.
	Parameters map[string]float64
	Samples    []Sample
	// Zones holds calibrated zone shapes by zone key.
	Zones  map[string][]Shape
	Events []ObservedEvent
	// ScoredEvents lists the event types that were scored. An unscored type
	// yields missing metrics, not zero counts.
	ScoredEvents []string
	// DurationUs is the recorded trial length; 0 means unknown.
	DurationUs int64
}

type MetricValue struct {
	Key     string
	Unit    Unit
	Value   float64
	Missing bool
	Reason  string
}

// Interval is an event over [StartUs, EndUs) in microseconds from the trial start.
type Interval struct {
	Type           string
	StartUs, EndUs int64
}

type Point struct {
	Type string
	AtUs int64
}

type Result struct {
	Paradigm                                                  string
	ParadigmVersion, MetricEngineVersion, ResultSchemaVersion int
	Parameters                                                map[string]float64
	Metrics                                                   []MetricValue
	Intervals                                                 []Interval
	Points                                                    []Point
}

func (r Result) Metric(key string) (MetricValue, bool) {
	i := slices.IndexFunc(r.Metrics, func(m MetricValue) bool { return m.Key == key })
	if i < 0 {
		return MetricValue{}, false
	}
	return r.Metrics[i], true
}

// evaluation is a validated input handed to a paradigm's engine.
type evaluation struct {
	params     map[string]float64
	samples    []Sample
	zones      map[string][]Shape
	events     []ObservedEvent
	scored     map[string]bool
	durationUs int64
}

// computation holds an engine's output. A metric absent from values is missing
// with its reason from missing, or reason when none is given.
type computation struct {
	values    map[string]float64
	missing   map[string]string
	reason    string
	intervals []Interval
	points    []Point
}

type definition struct {
	paradigm Paradigm
	// validate checks rules that span parameters; checkEvents checks ordering
	// rules between observed events.
	validate    func(params map[string]float64) error
	checkEvents func(events []ObservedEvent) error
	evaluate    func(e evaluation) computation
}

// registry lists every published version of each paradigm, oldest first.
var registry = map[string][]definition{
	"BARNES_MAZE":   {barnesMazeV1()},
	"EPM":           {elevatedPlusMazeV1()},
	"LIGHT_DARK":    {lightDarkV1()},
	"MWM":           {morrisWaterMazeV1()},
	"NOVEL_OBJECT":  {novelObjectV1()},
	"OPEN_FIELD":    {openFieldV1()},
	"POLE":          {poleTestV1()},
	"ROTAROD":       {rotarodV1()},
	"THREE_CHAMBER": {threeChamberV1()},
	"TREADMILL":     {treadmillV1()},
	"Y_MAZE":        {yMazeV1()},
}

func Keys() []string {
	keys := make([]string, 0, len(registry))
	for key := range registry {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
}

// All returns the latest version of every paradigm, ordered by key.
func All() []Paradigm {
	out := make([]Paradigm, 0, len(registry))
	for _, key := range Keys() {
		versions := registry[key]
		out = append(out, versions[len(versions)-1].paradigm.clone())
	}
	return out
}

func Versions(key string) ([]int, error) {
	versions, ok := registry[key]
	if !ok {
		return nil, ErrUnknownParadigm
	}
	out := make([]int, 0, len(versions))
	for _, d := range versions {
		out = append(out, d.paradigm.Version)
	}
	return out, nil
}

func Latest(key string) (Paradigm, error) {
	versions, ok := registry[key]
	if !ok {
		return Paradigm{}, ErrUnknownParadigm
	}
	return versions[len(versions)-1].paradigm.clone(), nil
}

func Version(key string, version int) (Paradigm, error) {
	d, err := lookup(key, version)
	return d.paradigm.clone(), err
}

func lookup(key string, version int) (definition, error) {
	versions, ok := registry[key]
	if !ok {
		return definition{}, ErrUnknownParadigm
	}
	for _, d := range versions {
		if d.paradigm.Version == version {
			return d, nil
		}
	}
	return definition{}, ErrUnknownVersion
}

// Evaluate validates an input against a paradigm version and computes its
// metrics and events.
func Evaluate(key string, version int, in Input) (Result, error) {
	d, err := lookup(key, version)
	if err != nil {
		return Result{}, err
	}
	params, err := resolveParameters(d.paradigm, in.Parameters)
	if err != nil {
		return Result{}, err
	}
	if d.validate != nil {
		if err := d.validate(params); err != nil {
			return Result{}, err
		}
	}
	if err := validateSamples(in.Samples); err != nil {
		return Result{}, err
	}
	if err := validateZones(d.paradigm, in.Zones); err != nil {
		return Result{}, err
	}
	scored, err := validateEvents(d.paradigm, in)
	if err != nil {
		return Result{}, err
	}
	events := slices.Clone(in.Events)
	slices.SortStableFunc(events, func(a, b ObservedEvent) int { return cmp.Compare(a.StartUs, b.StartUs) })
	if d.checkEvents != nil {
		if err := d.checkEvents(events); err != nil {
			return Result{}, err
		}
	}
	c := d.evaluate(evaluation{params: maps.Clone(params), samples: in.Samples, zones: in.Zones, events: events, scored: scored, durationUs: in.DurationUs})
	result := Result{
		Paradigm: key, ParadigmVersion: version, MetricEngineVersion: MetricEngineVersion, ResultSchemaVersion: ResultSchemaVersion,
		Parameters: params, Intervals: c.intervals, Points: c.points,
	}
	for _, m := range d.paradigm.Metrics {
		if v, ok := c.values[m.Key]; ok {
			result.Metrics = append(result.Metrics, MetricValue{Key: m.Key, Unit: m.Unit, Value: v})
			continue
		}
		reason := c.missing[m.Key]
		if reason == "" {
			reason = c.reason
		}
		result.Metrics = append(result.Metrics, MetricValue{Key: m.Key, Unit: m.Unit, Missing: true, Reason: reason})
	}
	slices.SortFunc(result.Intervals, func(a, b Interval) int {
		return cmp.Or(cmp.Compare(a.StartUs, b.StartUs), cmp.Compare(a.EndUs, b.EndUs), cmp.Compare(a.Type, b.Type))
	})
	slices.SortFunc(result.Points, func(a, b Point) int { return cmp.Or(cmp.Compare(a.AtUs, b.AtUs), cmp.Compare(a.Type, b.Type)) })
	return result, nil
}

func resolveParameters(p Paradigm, values map[string]float64) (map[string]float64, error) {
	all := append(slices.Clone(p.ApparatusParameters), p.SessionParameters...)
	out := make(map[string]float64, len(all))
	for _, param := range all {
		out[param.Key] = param.Default
	}
	for key, v := range values {
		i := slices.IndexFunc(all, func(param Parameter) bool { return param.Key == key })
		if i < 0 {
			return nil, fmt.Errorf("%w: %s", ErrUnknownParameter, key)
		}
		if math.IsNaN(v) || v < all[i].Min || v > all[i].Max || (all[i].Integer && v != math.Trunc(v)) {
			return nil, fmt.Errorf("%w: %s", ErrParameterOutOfRange, key)
		}
		out[key] = v
	}
	return out, nil
}

func validateSamples(samples []Sample) error {
	for i, s := range samples {
		switch {
		case s.TUs < 0, i > 0 && s.TUs <= samples[i-1].TUs:
			return fmt.Errorf("%w: sample %d time", ErrInvalidSample, i)
		case s.Valid && (!finite(s.X) || !finite(s.Y)):
			return fmt.Errorf("%w: sample %d coordinates", ErrInvalidSample, i)
		}
	}
	return nil
}

func validateZones(p Paradigm, zones map[string][]Shape) error {
	for key, shapes := range zones {
		if !slices.ContainsFunc(p.Zones, func(z Zone) bool { return z.Key == key && z.Calibrated }) {
			return fmt.Errorf("%w: %s", ErrUnknownZone, key)
		}
		for _, s := range shapes {
			if !s.valid() {
				return fmt.Errorf("%w: %s", ErrInvalidZone, key)
			}
		}
	}
	for _, z := range p.Zones {
		if z.Calibrated && z.Required && len(zones[z.Key]) == 0 {
			return fmt.Errorf("%w: %s", ErrMissingZone, z.Key)
		}
	}
	return nil
}

func validateEvents(p Paradigm, in Input) (map[string]bool, error) {
	if in.DurationUs < 0 {
		return nil, fmt.Errorf("%w: negative recording length", ErrInvalidEvent)
	}
	declared := func(typ string) int {
		return slices.IndexFunc(p.InputEvents, func(e EventDefinition) bool { return e.Type == typ })
	}
	scored := map[string]bool{}
	for _, typ := range in.ScoredEvents {
		if declared(typ) < 0 {
			return nil, fmt.Errorf("%w: undeclared type %s", ErrInvalidEvent, typ)
		}
		scored[typ] = true
	}
	counts := map[string]int{}
	for _, e := range in.Events {
		i := declared(e.Type)
		if i < 0 || !scored[e.Type] {
			return nil, fmt.Errorf("%w: %s is not a scored event type", ErrInvalidEvent, e.Type)
		}
		def := p.InputEvents[i]
		counts[e.Type]++
		switch {
		case e.StartUs < 0,
			def.Kind == PointEvent && e.EndUs != e.StartUs,
			def.Kind == IntervalEvent && e.EndUs <= e.StartUs,
			in.DurationUs > 0 && e.EndUs > in.DurationUs,
			len(def.Labels) == 0 && e.Label != "",
			len(def.Labels) > 0 && !slices.Contains(def.Labels, e.Label),
			def.Single && counts[e.Type] > 1:
			return nil, fmt.Errorf("%w: %s", ErrInvalidEvent, e.Type)
		}
	}
	return scored, nil
}

func finite(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }
