package domain

import (
	"encoding/json"
	"errors"
	"math"
	"testing"
	"time"
)

var (
	jpeg = []byte{0xFF, 0xD8, 0xFF, 0xE0, 0, 0x10}
	png  = []byte("\x89PNG\r\n\x1a\n....")
)

func TestProposalsTakeOnlyJPEGOrPNGFrames(t *testing.T) {
	p, err := NewProposal("NOVEL_OBJECT", 1, map[string]float64{"arena_width_cm": 40}, png, "u")
	if err != nil || p.FrameType != "image/png" || p.Status != Queued {
		t.Fatalf("png: %+v %v", p, err)
	}
	for name, frame := range map[string][]byte{"gif": []byte("GIF89a"), "empty": nil, "too large": append(jpeg, make([]byte, MaxProposalFrameBytes)...)} {
		if _, err := NewProposal("NOVEL_OBJECT", 1, nil, frame, "u"); !errors.Is(err, ErrInvalidProposal) {
			t.Errorf("%s: %v", name, err)
		}
	}
	if _, err := NewProposal("NOVEL_OBJECT", 1, map[string]float64{"x": math.NaN()}, jpeg, "u"); !errors.Is(err, ErrInvalidProposal) {
		t.Errorf("NaN parameter: %v", err)
	}
}

// An expired lease is claimed again until the attempts run out; only the
// current attempt can answer, with a JSON object.
func TestProposalLeasesAndAnswers(t *testing.T) {
	at := time.Date(2026, 11, 10, 9, 0, 0, 0, time.UTC)
	p, _ := NewProposal("MWM", 1, nil, jpeg, "u")
	first, err := p.Claim("w1", at, time.Minute)
	if err != nil || first.Attempt != 1 || first.Status != Running {
		t.Fatalf("claim: %+v %v", first, err)
	}
	if _, err := first.Claim("w2", at.Add(30*time.Second), time.Minute); !errors.Is(err, ErrNotClaimable) {
		t.Fatalf("held lease claimed: %v", err)
	}
	second, err := first.Claim("w2", at.Add(time.Minute), time.Minute)
	if err != nil || second.Attempt != 2 || second.WorkerID != "w2" {
		t.Fatalf("expired lease: %+v %v", second, err)
	}
	if _, err := second.Complete("w1", 1, json.RawMessage(`{}`), at); !errors.Is(err, ErrStaleAttempt) {
		t.Fatalf("stale answer: %v", err)
	}
	for _, bad := range []string{`[]`, `null`, `"x"`, `{`} {
		if _, err := second.Complete("w2", 2, json.RawMessage(bad), at); !errors.Is(err, ErrInvalidProposalResult) {
			t.Errorf("result %s: %v", bad, err)
		}
	}
	done, err := second.Complete("w2", 2, json.RawMessage(`{"detected":false}`), at)
	if err != nil || done.Status != Succeeded || done.LeaseExpiresAt != nil || done.FinishedAt == nil {
		t.Fatalf("complete: %+v %v", done, err)
	}
	last := second
	last.Attempt = MaxProposalAttempts
	if _, err := last.Claim("w3", at.Add(time.Hour), time.Minute); !errors.Is(err, ErrNotClaimable) {
		t.Fatalf("out of attempts: %v", err)
	}
}
