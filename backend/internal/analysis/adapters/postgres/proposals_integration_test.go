//go:build integration

package postgres_test

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/HappyHackingSpace/Misko/backend/internal/analysis/adapters/postgres"
	"github.com/HappyHackingSpace/Misko/backend/internal/analysis/application"
	"github.com/HappyHackingSpace/Misko/backend/internal/analysis/domain"
	"github.com/HappyHackingSpace/Misko/backend/internal/platform/pgtest"
)

// Proposals queue by capability, a lease that expires is claimed again until
// the attempts run out, and old proposals are deleted.
func TestProposalsQueueLeaseAndExpire(t *testing.T) {
	pool := pgtest.Database(t)
	store := postgres.NewStore(pool)
	w := worker(t, pool, store)
	now := time.Now().UTC().Truncate(time.Microsecond)
	frame := []byte{0xFF, 0xD8, 0xFF, 0xE0}

	tank, _ := domain.NewProposal("MWM", 1, nil, frame, author)
	if _, err := store.CreateProposal(ctx, tank); err != nil {
		t.Fatal(err)
	}
	box, _ := domain.NewProposal("NOVEL_OBJECT", 1, map[string]float64{"arena_width_cm": 40}, frame, author)
	created, err := store.CreateProposal(ctx, box)
	if err != nil || created.Status != domain.Queued || created.Parameters["arena_width_cm"] != 40 {
		t.Fatalf("create: %+v %v", created, err)
	}

	claim := func(at time.Time) (domain.Proposal, error) {
		if err := store.FailExpiredProposals(ctx, at); err != nil {
			return domain.Proposal{}, err
		}
		p, err := store.ClaimNextProposal(ctx, w.Capabilities, at)
		if err != nil {
			return p, err
		}
		claimed, err := p.Claim(w.ID, at, time.Minute)
		if err != nil {
			return p, err
		}
		return store.UpdateProposal(ctx, claimed)
	}
	// Only the NOVEL_OBJECT proposal matches the worker; it is claimed three times as its leases expire.
	for attempt := 1; attempt <= domain.MaxProposalAttempts; attempt++ {
		p, err := claim(now.Add(time.Duration(attempt-1) * time.Minute))
		if err != nil || p.ID != created.ID || p.Attempt != attempt || string(p.Frame) != string(frame) {
			t.Fatalf("claim %d: %+v %v", attempt, p, err)
		}
	}
	if _, err := claim(now.Add(10 * time.Minute)); !errors.Is(err, application.ErrNoJob) {
		t.Fatalf("out of attempts: %v", err)
	}
	failed, err := store.Proposal(ctx, created.ID)
	if err != nil || failed.Status != domain.Failed || failed.FailureReason != "LEASE_EXPIRED" {
		t.Fatalf("expired: %+v %v", failed, err)
	}

	again, _ := store.CreateProposal(ctx, box)
	p, err := claim(now)
	if err != nil || p.ID != again.ID {
		t.Fatalf("claim: %+v %v", p, err)
	}
	done, err := p.Complete(w.ID, p.Attempt, json.RawMessage(`{"detected":false,"reason":"NO_ARENA_DETECTED"}`), now)
	if err != nil {
		t.Fatal(err)
	}
	stored, err := store.UpdateProposal(ctx, done)
	if err != nil || stored.Status != domain.Succeeded || stored.FinishedAt == nil {
		t.Fatalf("complete: %+v %v", stored, err)
	}
	var result map[string]any
	if json.Unmarshal(stored.Result, &result) != nil || result["reason"] != "NO_ARENA_DETECTED" {
		t.Fatalf("result: %s", stored.Result)
	}

	if err := store.DeleteProposalsBefore(ctx, time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Proposal(ctx, again.ID); !errors.Is(err, application.ErrProposalNotFound) {
		t.Fatalf("deleted: %v", err)
	}
}
