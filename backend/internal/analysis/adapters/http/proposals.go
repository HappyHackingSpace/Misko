package analysishttp

import (
	"encoding/json"
	"net/http"
	"time"

	access "github.com/HappyHackingSpace/Misko/backend/internal/access/domain"
	"github.com/HappyHackingSpace/Misko/backend/internal/analysis/application"
	"github.com/HappyHackingSpace/Misko/backend/internal/analysis/domain"
)

type proposalRequest struct {
	ParadigmKey     string             `json:"paradigmKey"`
	ParadigmVersion int                `json:"paradigmVersion"`
	Parameters      map[string]float64 `json:"parameters"`
	// Frame is the reference frame as base64 JPEG or PNG; encoding/json decodes it.
	Frame []byte `json:"frame"`
}

// proposalJSON is what the panel polls: no frame, the worker's result once done.
type proposalJSON struct {
	ID              string          `json:"id"`
	ParadigmKey     string          `json:"paradigmKey"`
	ParadigmVersion int             `json:"paradigmVersion"`
	Status          domain.Status   `json:"status"`
	Result          json.RawMessage `json:"result"`
	FailureReason   *string         `json:"failureReason"`
	CreatedAt       time.Time       `json:"createdAt"`
	FinishedAt      *time.Time      `json:"finishedAt"`
}

func toProposal(p domain.Proposal) proposalJSON {
	return proposalJSON{ID: p.ID, ParadigmKey: p.ParadigmKey, ParadigmVersion: p.ParadigmVersion, Status: p.Status, Result: p.Result,
		FailureReason: nullable(p.FailureReason), CreatedAt: p.CreatedAt, FinishedAt: p.FinishedAt}
}

// claimedProposalJSON is what a worker needs to detect: the frame and the
// apparatus parameters, and the attempt that fences its answer.
type claimedProposalJSON struct {
	ID              string             `json:"id"`
	Attempt         int                `json:"attempt"`
	ParadigmKey     string             `json:"paradigmKey"`
	ParadigmVersion int                `json:"paradigmVersion"`
	Parameters      map[string]float64 `json:"parameters"`
	FrameType       string             `json:"frameType"`
	Frame           []byte             `json:"frame"`
}

func (h handler) requestProposal(w http.ResponseWriter, r *http.Request, actor access.Actor) {
	var body proposalRequest
	if !h.decode(w, r, &body) {
		return
	}
	p, err := h.service.RequestProposal(r.Context(), actor, body.ParadigmKey, body.ParadigmVersion, body.Parameters, body.Frame)
	h.respond(w, r, http.StatusAccepted, toProposal(p), err)
}

func (h handler) proposal(w http.ResponseWriter, r *http.Request, actor access.Actor) {
	p, err := h.service.Proposal(r.Context(), actor, r.PathValue("id"))
	h.respond(w, r, http.StatusOK, toProposal(p), err)
}

func (h handler) claimProposal(w http.ResponseWriter, r *http.Request, worker domain.Worker) {
	var body struct{}
	if !h.decode(w, r, &body) {
		return
	}
	p, err := h.service.ClaimProposal(r.Context(), worker)
	if err == application.ErrNoJob {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	h.respond(w, r, http.StatusOK, claimedProposalJSON{p.ID, p.Attempt, p.ParadigmKey, p.ParadigmVersion, p.Parameters, p.FrameType, p.Frame}, err)
}

func (h handler) completeProposal(w http.ResponseWriter, r *http.Request, worker domain.Worker) {
	var body struct {
		Attempt int             `json:"attempt"`
		Result  json.RawMessage `json:"result"`
	}
	if !h.decode(w, r, &body) {
		return
	}
	p, err := h.service.CompleteProposal(r.Context(), worker, r.PathValue("id"), body.Attempt, body.Result)
	h.respond(w, r, http.StatusOK, toProposal(p), err)
}
