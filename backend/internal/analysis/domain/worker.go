package domain

import (
	"errors"
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

var (
	ErrInvalidWorkerName = errors.New("worker name must contain 1 to 120 printable characters")
	ErrInvalidCapability = errors.New("a worker needs 1 to 50 distinct paradigm versions")
)

var paradigmKey = regexp.MustCompile(`^[A-Z][A-Z_]{0,39}$`)

// Capability is a paradigm version a worker can analyze automatically.
type Capability struct {
	ParadigmKey     string
	ParadigmVersion int
}

// Worker is an analysis service identity, separate from user roles. It may act
// only on runs it has claimed.
type Worker struct {
	ID           string
	Name         string
	ModelVersion string
	Capabilities []Capability
	DisabledAt   *time.Time
	CreatedBy    string
	CreatedAt    time.Time
}

func NewWorker(name, modelVersion string, capabilities []Capability) (Worker, error) {
	text := strings.TrimSpace(name)
	if text == "" || !utf8.ValidString(text) || utf8.RuneCountInString(text) > 120 || strings.IndexFunc(text, unicode.IsControl) >= 0 {
		return Worker{}, ErrInvalidWorkerName
	}
	model, err := normalizeModel(modelVersion)
	if err != nil {
		return Worker{}, err
	}
	if len(capabilities) == 0 || len(capabilities) > 50 {
		return Worker{}, ErrInvalidCapability
	}
	seen := map[Capability]bool{}
	for _, c := range capabilities {
		if !paradigmKey.MatchString(c.ParadigmKey) || c.ParadigmVersion < 1 || seen[c] {
			return Worker{}, ErrInvalidCapability
		}
		seen[c] = true
	}
	return Worker{Name: text, ModelVersion: model, Capabilities: append([]Capability{}, capabilities...)}, nil
}

func (w Worker) Can(key string, version int) bool {
	for _, c := range w.Capabilities {
		if c.ParadigmKey == key && c.ParadigmVersion == version {
			return true
		}
	}
	return false
}
