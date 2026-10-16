package domain

import (
	"errors"
	"slices"
	"strings"
	"time"
	"unicode/utf8"
)

var (
	ErrInvalidRejection = errors.New("a rejection needs one of TRACKING_LOST, WRONG_ANIMAL, VIDEO_DEFECT, PROTOCOL_DEVIATION or OTHER")
	ErrInvalidNote      = errors.New("a review note must contain 1 to 2000 characters")
	ErrRunNotChosen     = errors.New("only a succeeded run of the same animal in the same test can be chosen")
)

// ResultStatus is the human decision on one animal's result. Only ACCEPTED
// results enter statistics.
type ResultStatus string

const (
	ResultPending  ResultStatus = "PENDING"
	ResultAccepted ResultStatus = "ACCEPTED"
	ResultRejected ResultStatus = "REJECTED"
)

// RejectionReasons are the reasons a reviewer may reject a result with.
func RejectionReasons() []string {
	return []string{"TRACKING_LOST", "WRONG_ANIMAL", "VIDEO_DEFECT", "PROTOCOL_DEVIATION", "OTHER"}
}

// SubjectResult is the run that counts for one animal of a test and the
// reviewer's decision on it. The first successful run opens it as PENDING.
type SubjectResult struct {
	TestSubjectID string
	RunID         string
	Status        ResultStatus
	Reason        string
	Note          string
	ReviewedBy    string
	ReviewedAt    *time.Time
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// OpenResult is the PENDING result a first successful run creates.
func OpenResult(run Run) SubjectResult {
	return SubjectResult{TestSubjectID: run.TestSubjectID, RunID: run.ID, Status: ResultPending}
}

// Accept records that the result counts. A decision may be revised while the
// experiment runs: accepting a rejected result clears its reason.
func (r SubjectResult) Accept(reviewer, note string, at time.Time) (SubjectResult, error) {
	text, err := normalizeNote(note)
	if err != nil {
		return SubjectResult{}, err
	}
	r.Status, r.Reason, r.Note, r.ReviewedBy, r.ReviewedAt = ResultAccepted, "", text, reviewer, &at
	return r, nil
}

// Reject keeps the measurement visible but out of statistics.
func (r SubjectResult) Reject(reviewer, reason, note string, at time.Time) (SubjectResult, error) {
	if !slices.Contains(RejectionReasons(), reason) {
		return SubjectResult{}, ErrInvalidRejection
	}
	text, err := normalizeNote(note)
	if err != nil {
		return SubjectResult{}, err
	}
	r.Status, r.Reason, r.Note, r.ReviewedBy, r.ReviewedAt = ResultRejected, reason, text, reviewer, &at
	return r, nil
}

// Choose makes another succeeded run of the same animal the one under review
// and sends the result back to PENDING. Choosing the current run reopens it.
func (r SubjectResult) Choose(run Run) (SubjectResult, error) {
	if run.TestSubjectID != r.TestSubjectID || run.Status != Succeeded {
		return SubjectResult{}, ErrRunNotChosen
	}
	r.RunID, r.Status, r.Reason, r.Note, r.ReviewedBy, r.ReviewedAt = run.ID, ResultPending, "", "", "", nil
	return r, nil
}

func normalizeNote(raw string) (string, error) {
	text := strings.TrimSpace(raw)
	if !utf8.ValidString(text) || utf8.RuneCountInString(text) > 2000 {
		return "", ErrInvalidNote
	}
	return text, nil
}
