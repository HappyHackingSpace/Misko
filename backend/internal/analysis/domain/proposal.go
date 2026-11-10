package domain

import (
	"bytes"
	"encoding/json"
	"errors"
	"math"
	"time"
)

const (
	// MaxProposalFrameBytes keeps a base64 frame inside one API request.
	MaxProposalFrameBytes = 700 << 10
	// MaxProposalAttempts is how often an expired proposal lease is retried.
	MaxProposalAttempts = 3
	maxProposalResult   = 64 << 10
)

var (
	ErrInvalidProposal       = errors.New("a proposal needs a paradigm version, finite parameters and a JPEG or PNG frame of at most 700 KiB")
	ErrInvalidProposalResult = errors.New("a proposal result must be a JSON object of at most 64 KiB")
)

// Proposal asks a worker to detect the apparatus on one reference frame and
// propose calibration points. The researcher confirms them in the panel; a
// proposal never becomes a calibration by itself.
type Proposal struct {
	ID              string
	ParadigmKey     string
	ParadigmVersion int
	Parameters      map[string]float64
	Frame           []byte
	FrameType       string
	Status          Status
	Attempt         int
	WorkerID        string
	LeaseExpiresAt  *time.Time
	Result          json.RawMessage
	FailureReason   string
	CreatedBy       string
	CreatedAt       time.Time
	FinishedAt      *time.Time
}

// NewProposal validates a frame by its magic bytes, so the worker never
// decodes anything but a JPEG or a PNG.
func NewProposal(key string, version int, parameters map[string]float64, frame []byte, createdBy string) (Proposal, error) {
	frameType := ""
	switch {
	case bytes.HasPrefix(frame, []byte{0xFF, 0xD8, 0xFF}):
		frameType = "image/jpeg"
	case bytes.HasPrefix(frame, []byte("\x89PNG\r\n\x1a\n")):
		frameType = "image/png"
	}
	if frameType == "" || len(frame) > MaxProposalFrameBytes || key == "" || version < 1 || createdBy == "" {
		return Proposal{}, ErrInvalidProposal
	}
	params := make(map[string]float64, len(parameters))
	for k, v := range parameters {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return Proposal{}, ErrInvalidProposal
		}
		params[k] = v
	}
	return Proposal{ParadigmKey: key, ParadigmVersion: version, Parameters: params, Frame: frame, FrameType: frameType, Status: Queued, CreatedBy: createdBy}, nil
}

// Claim leases a queued proposal, or one whose lease expired with attempts left.
func (p Proposal) Claim(workerID string, now time.Time, lease time.Duration) (Proposal, error) {
	expired := p.Status == Running && p.LeaseExpiresAt != nil && !now.Before(*p.LeaseExpiresAt)
	if (p.Status != Queued && !expired) || p.Attempt >= MaxProposalAttempts {
		return Proposal{}, ErrNotClaimable
	}
	until := now.Add(lease)
	p.Status, p.Attempt, p.WorkerID, p.LeaseExpiresAt = Running, p.Attempt+1, workerID, &until
	return p, nil
}

// Complete stores the worker's answer for the attempt it holds.
func (p Proposal) Complete(workerID string, attempt int, result json.RawMessage, now time.Time) (Proposal, error) {
	if p.Status != Running || p.WorkerID != workerID || p.Attempt != attempt {
		return Proposal{}, ErrStaleAttempt
	}
	var object map[string]json.RawMessage
	if len(result) > maxProposalResult || json.Unmarshal(result, &object) != nil || object == nil {
		return Proposal{}, ErrInvalidProposalResult
	}
	p.Status, p.LeaseExpiresAt, p.Result, p.FinishedAt = Succeeded, nil, result, &now
	return p, nil
}
