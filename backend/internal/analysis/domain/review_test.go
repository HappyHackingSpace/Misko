package domain

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestReviewDecisions(t *testing.T) {
	at := time.Date(2026, 10, 15, 9, 0, 0, 0, time.UTC)
	run := Run{ID: "run-1", Source: Source{TestSubjectID: "ts-1"}, Status: Succeeded}
	open := OpenResult(run)
	if open.Status != ResultPending || open.RunID != "run-1" || open.TestSubjectID != "ts-1" || open.ReviewedBy != "" {
		t.Fatalf("open: %+v", open)
	}

	accepted, err := open.Accept("reviewer", "  clean track ", at)
	if err != nil || accepted.Status != ResultAccepted || accepted.Note != "clean track" || accepted.ReviewedBy != "reviewer" || !accepted.ReviewedAt.Equal(at) {
		t.Fatalf("accept: %+v %v", accepted, err)
	}
	rejected, err := accepted.Reject("second", "TRACKING_LOST", "", at.Add(time.Hour))
	if err != nil || rejected.Status != ResultRejected || rejected.Reason != "TRACKING_LOST" || rejected.Note != "" || rejected.ReviewedBy != "second" {
		t.Fatalf("revise to reject: %+v %v", rejected, err)
	}
	again, err := rejected.Accept("third", "", at)
	if err != nil || again.Reason != "" || again.Status != ResultAccepted {
		t.Fatalf("accepting clears the reason: %+v %v", again, err)
	}
	for _, reason := range []string{"", "tracking_lost", "BORED"} {
		if _, err := open.Reject("r", reason, "", at); !errors.Is(err, ErrInvalidRejection) {
			t.Errorf("reason %q: %v", reason, err)
		}
	}
	if _, err := open.Accept("r", strings.Repeat("x", 2001), at); !errors.Is(err, ErrInvalidNote) {
		t.Errorf("long note: %v", err)
	}
	if _, err := open.Reject("r", "OTHER", "\xff", at); !errors.Is(err, ErrInvalidNote) {
		t.Errorf("invalid UTF-8 note: %v", err)
	}

	newer := Run{ID: "run-2", Source: Source{TestSubjectID: "ts-1"}, Status: Succeeded}
	chosen, err := again.Choose(newer)
	if err != nil || chosen.RunID != "run-2" || chosen.Status != ResultPending || chosen.ReviewedBy != "" || chosen.ReviewedAt != nil || chosen.Note != "" {
		t.Fatalf("choose: %+v %v", chosen, err)
	}
	for name, other := range map[string]Run{
		"another animal": {ID: "run-3", Source: Source{TestSubjectID: "ts-2"}, Status: Succeeded},
		"failed run":     {ID: "run-4", Source: Source{TestSubjectID: "ts-1"}, Status: Failed},
		"queued run":     {ID: "run-5", Source: Source{TestSubjectID: "ts-1"}, Status: Queued},
	} {
		if _, err := again.Choose(other); !errors.Is(err, ErrRunNotChosen) {
			t.Errorf("%s: %v", name, err)
		}
	}
}
