package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	access "github.com/HappyHackingSpace/Misko/backend/internal/access/domain"
	"github.com/HappyHackingSpace/Misko/backend/internal/analysis/domain"
)

const (
	// Detection on one frame takes seconds; a worker that holds a proposal
	// longer has stopped.
	proposalLease = 2 * time.Minute
	// Proposals are suggestions for a form being filled now; older ones are deleted.
	proposalTTL = 24 * time.Hour
)

// RequestProposal queues detection on a reference frame for anyone who may
// calibrate an environment, a test animal or a trial.
func (s *Service) RequestProposal(ctx context.Context, actor access.Actor, key string, version int, parameters map[string]float64, frame []byte) (domain.Proposal, error) {
	if actor.Require(access.ApparatusWrite) != nil && actor.Require(access.TestRun) != nil {
		if err := actor.Require(access.TrialRun); err != nil {
			return domain.Proposal{}, err
		}
	}
	p, err := domain.NewProposal(key, version, parameters, frame, actor.UserID)
	if err != nil {
		return domain.Proposal{}, err
	}
	if _, err := s.catalog.Contract(key, version); errors.Is(err, ErrUnknownCapability) {
		return domain.Proposal{}, domain.ErrInvalidProposal
	} else if err != nil {
		return domain.Proposal{}, err
	}
	if err := s.store.DeleteProposalsBefore(ctx, s.clock().Add(-proposalTTL)); err != nil {
		return domain.Proposal{}, err
	}
	return s.store.CreateProposal(ctx, p)
}

func (s *Service) Proposal(ctx context.Context, actor access.Actor, id string) (domain.Proposal, error) {
	if err := actor.Require(access.Read); err != nil {
		return domain.Proposal{}, err
	}
	return s.store.Proposal(ctx, id)
}

// ClaimProposal leases the oldest proposal for a paradigm version the worker supports.
func (s *Service) ClaimProposal(ctx context.Context, w domain.Worker) (domain.Proposal, error) {
	now := s.clock()
	var out domain.Proposal
	err := s.store.Transaction(ctx, func(tx Store) error {
		p, err := tx.ClaimNextProposal(ctx, w.Capabilities, now)
		if err != nil {
			return err
		}
		claimed, err := p.Claim(w.ID, now, proposalLease)
		if err != nil {
			return err
		}
		out, err = tx.UpdateProposal(ctx, claimed)
		return err
	})
	return out, err
}

// CompleteProposal stores the answer of the worker holding the attempt.
func (s *Service) CompleteProposal(ctx context.Context, w domain.Worker, id string, attempt int, result json.RawMessage) (domain.Proposal, error) {
	now := s.clock()
	var out domain.Proposal
	err := s.store.Transaction(ctx, func(tx Store) error {
		p, err := tx.LockProposal(ctx, id)
		if err != nil {
			return err
		}
		done, err := p.Complete(w.ID, attempt, result, now)
		if err != nil {
			return err
		}
		if out, err = tx.UpdateProposal(ctx, done); err != nil {
			return fmt.Errorf("store proposal result: %w", err)
		}
		return nil
	})
	return out, err
}
