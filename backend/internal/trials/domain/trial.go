// Package domain holds paradigm trials: a paradigm version tried on one video
// in one arena (an environment revision) outside any experiment, to see what
// the automatic analysis measures and how its ANALYSIS thresholds change the
// result. A trial has no groups, no review and no report, and none of its
// analyses ever reaches an experiment. Trials are deleted softly: their
// analyses never change, so the trial they belong to is kept, hidden.
package domain

import (
	"errors"
	"maps"
	"math"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

var (
	ErrInvalidName      = errors.New("a trial name must contain 1 to 120 printable characters")
	ErrInvalidNotes     = errors.New("notes must contain at most 2000 characters")
	ErrInvalidClip      = errors.New("clip start must be non-negative and the clip end after its start")
	ErrInvalidOverrides = errors.New("overrides must be ANALYSIS parameters of the paradigm version with finite values within range")
	ErrDeleted          = errors.New("the trial was deleted")
)

// Trial is one paradigm trial. VideoAssetID is its current video, empty
// until the first upload; a replacement upload takes its place.
type Trial struct {
	ID                    string
	Name                  string
	ParadigmKey           string
	ParadigmVersion       int
	EnvironmentRevisionID string
	VideoAssetID          string
	Clip                  Clip
	Notes                 string
	CreatedBy             string
	CreatedAt             time.Time
	UpdatedAt             time.Time
	DeletedAt             *time.Time
}

// Clip is the part of the video to analyse, in microseconds from its start.
// A nil EndUs runs to the end of the video.
type Clip struct {
	StartUs int64
	EndUs   *int64
}

// Validate checks the clip's bounds.
func (c Clip) Validate() error {
	if c.StartUs < 0 || (c.EndUs != nil && *c.EndUs <= c.StartUs) {
		return ErrInvalidClip
	}
	return nil
}

// NewTrial validates a trial of the paradigm version run in the revision.
func NewTrial(name, paradigmKey string, paradigmVersion int, revisionID, notes, createdBy string) (Trial, error) {
	n, err := normalizeName(name)
	if err != nil {
		return Trial{}, err
	}
	text := strings.TrimSpace(notes)
	if !utf8.ValidString(text) || utf8.RuneCountInString(text) > 2000 {
		return Trial{}, ErrInvalidNotes
	}
	return Trial{Name: n, ParadigmKey: paradigmKey, ParadigmVersion: paradigmVersion, EnvironmentRevisionID: revisionID, Notes: text,
		CreatedBy: createdBy}, nil
}

func normalizeName(raw string) (string, error) {
	text := strings.TrimSpace(raw)
	if text == "" || !utf8.ValidString(text) || utf8.RuneCountInString(text) > 120 || strings.IndexFunc(text, unicode.IsControl) >= 0 {
		return "", ErrInvalidName
	}
	return text, nil
}

// Active rejects changes to a deleted trial.
func (t Trial) Active() error {
	if t.DeletedAt != nil {
		return ErrDeleted
	}
	return nil
}

// AutomaticKey identifies the one automatic analysis of a trial video, so a
// repeated submission returns the first job.
func (t Trial) AutomaticKey() string {
	return "trial:" + t.ID + ":video:" + t.VideoAssetID
}

// CheckOverrides accepts only finite values; whether they are ANALYSIS
// parameters within range is the paradigm catalog's decision.
func CheckOverrides(overrides map[string]float64) error {
	if len(overrides) > 50 {
		return ErrInvalidOverrides
	}
	for key, v := range overrides {
		if key == "" || math.IsNaN(v) || math.IsInf(v, 0) {
			return ErrInvalidOverrides
		}
	}
	return nil
}

// Merge returns the revision's parameters with the overrides applied.
func Merge(parameters, overrides map[string]float64) map[string]float64 {
	out := maps.Clone(parameters)
	if out == nil {
		out = map[string]float64{}
	}
	maps.Copy(out, overrides)
	return out
}
