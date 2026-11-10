package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/HappyHackingSpace/Misko/backend/internal/analysis/adapters/postgres/sqlcgen"
	"github.com/HappyHackingSpace/Misko/backend/internal/analysis/application"
	"github.com/HappyHackingSpace/Misko/backend/internal/analysis/domain"
	"github.com/HappyHackingSpace/Misko/backend/internal/platform/pgtx"
)

func (s *Store) CreateProposal(ctx context.Context, p domain.Proposal) (domain.Proposal, error) {
	params, err := json.Marshal(p.Parameters)
	if err != nil {
		return domain.Proposal{}, fmt.Errorf("encode proposal parameters: %w", err)
	}
	row, err := s.queries.InsertProposal(ctx, sqlcgen.InsertProposalParams{
		ParadigmKey: p.ParadigmKey, ParadigmVersion: int32(p.ParadigmVersion), Parameters: params, Frame: p.Frame, FrameType: p.FrameType, CreatedBy: p.CreatedBy,
	})
	if err != nil {
		return domain.Proposal{}, fmt.Errorf("insert proposal: %w", err)
	}
	return toProposal(row)
}

func (s *Store) DeleteProposalsBefore(ctx context.Context, before time.Time) error {
	if err := s.queries.DeleteProposalsBefore(ctx, before); err != nil {
		return fmt.Errorf("delete old proposals: %w", err)
	}
	return nil
}

func (s *Store) Proposal(ctx context.Context, id string) (domain.Proposal, error) {
	if !pgtx.ValidUUID(id) {
		return domain.Proposal{}, application.ErrProposalNotFound
	}
	row, err := s.queries.GetProposal(ctx, id)
	if err != nil {
		return domain.Proposal{}, translate(err, application.ErrProposalNotFound, "get proposal")
	}
	return toProposal(row)
}

func (s *Store) LockProposal(ctx context.Context, id string) (domain.Proposal, error) {
	if !pgtx.ValidUUID(id) {
		return domain.Proposal{}, application.ErrProposalNotFound
	}
	row, err := s.queries.LockProposal(ctx, id)
	if err != nil {
		return domain.Proposal{}, translate(err, application.ErrProposalNotFound, "lock proposal")
	}
	return toProposal(row)
}

func (s *Store) FailExpiredProposals(ctx context.Context, now time.Time) error {
	if err := s.queries.FailExpiredProposals(ctx, now); err != nil {
		return fmt.Errorf("fail expired proposals: %w", err)
	}
	return nil
}

func (s *Store) ClaimNextProposal(ctx context.Context, capabilities []domain.Capability, now time.Time) (domain.Proposal, error) {
	row, err := s.queries.ClaimNextProposal(ctx, sqlcgen.ClaimNextProposalParams{Now: now, Capabilities: capabilityKeys(capabilities)})
	if err != nil {
		return domain.Proposal{}, translate(err, application.ErrNoJob, "claim proposal")
	}
	return toProposal(row)
}

func (s *Store) UpdateProposal(ctx context.Context, p domain.Proposal) (domain.Proposal, error) {
	var result []byte
	if p.Result != nil {
		result = p.Result
	}
	row, err := s.queries.UpdateProposal(ctx, sqlcgen.UpdateProposalParams{
		ID: p.ID, Status: string(p.Status), Attempt: int32(p.Attempt), WorkerID: optional(p.WorkerID), LeaseExpiresAt: p.LeaseExpiresAt,
		Result: result, FailureReason: optional(p.FailureReason), FinishedAt: p.FinishedAt,
	})
	if err != nil {
		return domain.Proposal{}, translate(err, application.ErrProposalNotFound, "update proposal")
	}
	return toProposal(row)
}

func toProposal(r sqlcgen.MiskoCalibrationProposal) (domain.Proposal, error) {
	params := map[string]float64{}
	if err := json.Unmarshal(r.Parameters, &params); err != nil {
		return domain.Proposal{}, fmt.Errorf("decode proposal parameters: %w", err)
	}
	p := domain.Proposal{
		ID: r.ID, ParadigmKey: r.ParadigmKey, ParadigmVersion: int(r.ParadigmVersion), Parameters: params, Frame: r.Frame, FrameType: r.FrameType,
		Status: domain.Status(r.Status), Attempt: int(r.Attempt), LeaseExpiresAt: r.LeaseExpiresAt, CreatedBy: r.CreatedBy, CreatedAt: r.CreatedAt, FinishedAt: r.FinishedAt,
	}
	if r.WorkerID != nil {
		p.WorkerID = *r.WorkerID
	}
	if r.FailureReason != nil {
		p.FailureReason = *r.FailureReason
	}
	if r.Result != nil {
		p.Result = json.RawMessage(r.Result)
	}
	return p, nil
}
