package application

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	access "github.com/HappyHackingSpace/Misko/backend/internal/access/domain"
	"github.com/HappyHackingSpace/Misko/backend/internal/analysis/domain"
)

var jpegFrame = []byte{0xFF, 0xD8, 0xFF, 0xE0}

type proposalStore struct {
	fakeStore
	proposal domain.Proposal
	queued   bool
	purged   time.Time
}

func (s *proposalStore) Transaction(_ context.Context, fn func(Store) error) error { return fn(s) }

func (s *proposalStore) CreateProposal(_ context.Context, p domain.Proposal) (domain.Proposal, error) {
	p.ID = "p-1"
	s.proposal, s.queued = p, true
	return p, nil
}

func (s *proposalStore) DeleteProposalsBefore(_ context.Context, before time.Time) error {
	s.purged = before
	return nil
}

func (s *proposalStore) Proposal(_ context.Context, id string) (domain.Proposal, error) {
	if id != s.proposal.ID {
		return domain.Proposal{}, ErrProposalNotFound
	}
	return s.proposal, nil
}

func (s *proposalStore) LockProposal(c context.Context, id string) (domain.Proposal, error) {
	return s.Proposal(c, id)
}

func (s *proposalStore) ClaimNextProposal(_ context.Context, _ []domain.Capability, _ time.Time) (domain.Proposal, error) {
	if !s.queued {
		return domain.Proposal{}, ErrNoJob
	}
	return s.proposal, nil
}

func (s *proposalStore) UpdateProposal(_ context.Context, p domain.Proposal) (domain.Proposal, error) {
	s.proposal, s.queued = p, false
	return p, nil
}

// Anyone who calibrates an environment, a test animal or a trial may ask for
// a proposal; a viewer may not.
func TestProposalsAreRequestedByThoseWhoCalibrate(t *testing.T) {
	for _, role := range access.Roles() {
		store := &proposalStore{}
		f := newFixture()
		s := New(Dependencies{Store: store, Objects: f.objects, Catalog: f.catalog, Tokens: fakeTokens{}}, settings, func() time.Time { return now })
		_, err := s.RequestProposal(ctx, access.Actor{UserID: "u", Role: role}, "MWM", 1, nil, jpegFrame)
		may := role.Allows(access.ApparatusWrite) || role.Allows(access.TestRun) || role.Allows(access.TrialRun)
		if may != (err == nil) || (!may && !errors.Is(err, access.ErrForbidden)) {
			t.Errorf("%s: %v", role, err)
		}
	}
}

func TestAProposalIsClaimedAndAnswered(t *testing.T) {
	store := &proposalStore{}
	f := newFixture()
	s := New(Dependencies{Store: store, Objects: f.objects, Catalog: f.catalog, Tokens: fakeTokens{}}, settings, func() time.Time { return now })
	manager := access.Actor{UserID: "m", Role: access.LabManager}
	if _, err := s.RequestProposal(ctx, manager, "UNKNOWN", 1, nil, jpegFrame); !errors.Is(err, domain.ErrInvalidProposal) {
		t.Fatalf("unknown paradigm: %v", err)
	}
	p, err := s.RequestProposal(ctx, manager, "MWM", 1, map[string]float64{"tank_diameter_cm": 120}, jpegFrame)
	if err != nil || p.Status != domain.Queued || !store.purged.Equal(now.Add(-proposalTTL)) {
		t.Fatalf("request: %+v %v", p, err)
	}
	claimed, err := s.ClaimProposal(ctx, worker)
	if err != nil || claimed.Attempt != 1 || claimed.WorkerID != worker.ID || string(claimed.Frame) != string(jpegFrame) {
		t.Fatalf("claim: %+v %v", claimed, err)
	}
	if _, err := s.ClaimProposal(ctx, worker); !errors.Is(err, ErrNoJob) {
		t.Fatalf("nothing left: %v", err)
	}
	done, err := s.CompleteProposal(ctx, worker, p.ID, 1, json.RawMessage(`{"detected":true}`))
	if err != nil || done.Status != domain.Succeeded {
		t.Fatalf("complete: %+v %v", done, err)
	}
	read, err := s.Proposal(ctx, access.Actor{UserID: "v", Role: access.Viewer}, p.ID)
	if err != nil || string(read.Result) != `{"detected":true}` {
		t.Fatalf("read: %+v %v", read, err)
	}
}
