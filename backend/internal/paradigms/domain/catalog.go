// Package domain defines the hardcoded paradigm catalog and metric engine.
// Definitions are code and versioned. There is no editable catalog: callers
// always receive copies, and a published version is never changed.
package domain

import (
	"cmp"
	"errors"
	"fmt"
	"math"
	"slices"
)

const (
	// MetricEngineVersion changes whenever a calculation changes, so results
	// computed by different engines are never silently compared.
	MetricEngineVersion = 1
	// ResultSchemaVersion changes whenever the shape of a result changes.
	ResultSchemaVersion = 1
	// ReasonNoValidIntervals marks metrics that could not be computed because no
	// pair of consecutive tracked samples qualified. Missing is never zero.
	ReasonNoValidIntervals = "NO_VALID_INTERVALS"
)

var (
	ErrUnknownParadigm     = errors.New("unknown paradigm")
	ErrUnknownVersion      = errors.New("unknown paradigm version")
	ErrUnknownParameter    = errors.New("unknown parameter")
	ErrParameterOutOfRange = errors.New("parameter is outside its allowed range")
	ErrInvalidSample       = errors.New("samples need non-negative, strictly increasing timestamps and finite tracked coordinates")
)

type Unit string

const (
	Centimeter          Unit = "cm"
	CentimeterPerSecond Unit = "cm_s"
	Second              Unit = "s"
	Count               Unit = "count"
	Ratio               Unit = "ratio"
)

type Category string

const (
	LearningMemory Category = "LEARNING_MEMORY"
	Anxiety        Category = "ANXIETY"
	Motor          Category = "MOTOR"
	Social         Category = "SOCIAL"
)

// Parameter is a user-chosen physical or session value with an allowed range.
type Parameter struct {
	Key, Label        string
	Unit              Unit
	Min, Max, Default float64
}

type Zone struct {
	Key, Role, Geometry string
}

type EventKind string

const (
	IntervalEvent EventKind = "INTERVAL"
	PointEvent    EventKind = "POINT"
)

type EventDefinition struct {
	Type       string
	Kind       EventKind
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
	Events                                 []EventDefinition
	Metrics                                []MetricDefinition
	QC                                     []QCRule
}

func (p Paradigm) clone() Paradigm {
	p.Species, p.TrialTypes = slices.Clone(p.Species), slices.Clone(p.TrialTypes)
	p.ApparatusParameters, p.SessionParameters = slices.Clone(p.ApparatusParameters), slices.Clone(p.SessionParameters)
	p.Zones, p.Events, p.QC = slices.Clone(p.Zones), slices.Clone(p.Events), slices.Clone(p.QC)
	p.Metrics = slices.Clone(p.Metrics)
	for i := range p.Metrics {
		p.Metrics[i].Inputs = slices.Clone(p.Metrics[i].Inputs)
	}
	return p
}

// Sample is one tracked position in arena-local centimeters. TUs is
// microseconds from the recording start. Lost frames have Valid false and
// their coordinates are ignored.
type Sample struct {
	TUs   int64
	X, Y  float64
	Valid bool
}

type Input struct {
	// Parameters override defaults; unknown keys are rejected.
	Parameters map[string]float64
	Samples    []Sample
}

type MetricValue struct {
	Key     string
	Unit    Unit
	Value   float64
	Missing bool
	Reason  string
}

// Interval is an event over [StartUs, EndUs) in microseconds from the recording start.
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

// computation holds an evaluator's output. Metrics absent from values are
// reported missing with reason.
type computation struct {
	values    map[string]float64
	reason    string
	intervals []Interval
	points    []Point
}

type definition struct {
	paradigm Paradigm
	evaluate func(params map[string]float64, samples []Sample) computation
}

// registry lists every published version of each paradigm, oldest first.
var registry = map[string][]definition{
	"OPEN_FIELD": {openFieldV1()},
}

func Keys() []string {
	keys := make([]string, 0, len(registry))
	for key := range registry {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
}

// All returns the latest version of every paradigm.
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

// Evaluate computes a paradigm version's metrics and events from samples.
func Evaluate(key string, version int, in Input) (Result, error) {
	d, err := lookup(key, version)
	if err != nil {
		return Result{}, err
	}
	params, err := resolveParameters(d.paradigm, in.Parameters)
	if err != nil {
		return Result{}, err
	}
	if err := validateSamples(in.Samples); err != nil {
		return Result{}, err
	}
	c := d.evaluate(maps(params), in.Samples)
	result := Result{
		Paradigm: key, ParadigmVersion: version, MetricEngineVersion: MetricEngineVersion, ResultSchemaVersion: ResultSchemaVersion,
		Parameters: params, Intervals: c.intervals, Points: c.points,
	}
	for _, m := range d.paradigm.Metrics {
		v, ok := c.values[m.Key]
		if ok {
			result.Metrics = append(result.Metrics, MetricValue{Key: m.Key, Unit: m.Unit, Value: v})
		} else {
			result.Metrics = append(result.Metrics, MetricValue{Key: m.Key, Unit: m.Unit, Missing: true, Reason: c.reason})
		}
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
		if math.IsNaN(v) || v < all[i].Min || v > all[i].Max {
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

func finite(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }

func maps(m map[string]float64) map[string]float64 {
	out := make(map[string]float64, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}
