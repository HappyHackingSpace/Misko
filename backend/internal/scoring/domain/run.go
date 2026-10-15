package domain

import "time"

// MetricValue is a computed metric; a missing value has a reason instead.
type MetricValue struct {
	Key, Unit     string
	Value         *float64
	MissingReason string
}

// DerivedEvent is an event the engine derived, in microseconds from the clip start.
type DerivedEvent struct {
	Type           string
	Kind           Kind
	StartUs, EndUs int64
}

// Computed is the engine's result for one scoring.
type Computed struct {
	MetricEngineVersion, ResultSchemaVersion int
	Metrics                                  []MetricValue
	Events                                   []DerivedEvent
}

// Video is the test's current video with the clip the trial covers.
type Video struct {
	Clip
	AssetID    string
	Generation int64
	CRC32C     uint32
}

// Run is the SCORED analysis run a completion publishes: it pins the video
// the animal was scored on, the paradigm version and the parameters, like a
// run a worker analysed, and records who scored it.
type Run struct {
	ID              string
	ExperimentID    string
	TestID          string
	TestSubjectID   string
	Video           Video
	ParadigmKey     string
	ParadigmVersion int
	Parameters      map[string]float64
	Computed        Computed
	ScoredBy        string
	FinishedAt      time.Time
}
