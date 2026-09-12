package domain

import (
	"errors"
	access "github.com/HappyHackingSpace/Misko/backend/internal/access/domain"
	"strings"
	"testing"
	"time"
)

var (
	now      = time.Date(2026, 9, 11, 12, 0, 0, 0, time.UTC)
	enrolled = now.Add(-10 * 24 * time.Hour)
)

func planned() Test {
	t, err := NewTest(Context{
		ExperimentID: "exp", EnrollmentID: "enr", SubjectID: "sub", EnrolledAt: enrolled, GroupID: "group",
		ProtocolVersionID: "v1", StepPosition: 1, ParadigmKey: "OPEN_FIELD", ParadigmVersion: 1, EnvironmentRevisionID: "rev", PlannedTrials: 2,
	}, now.Add(-time.Hour), "", "user")
	if err != nil {
		panic(err)
	}
	return t
}

func at(minutes int) time.Time { return now.Add(time.Duration(minutes) * time.Minute) }

func TestNewTestKeepsItsContext(t *testing.T) {
	test := planned()
	if test.Status != Planned || test.SubjectID != "sub" || test.GroupID != "group" || test.PlannedTrials != 2 || test.CreatedBy != "user" {
		t.Fatalf("test: %+v", test)
	}
	base := Context{ExperimentID: "exp", EnrollmentID: "enr", SubjectID: "sub", EnrolledAt: enrolled, ProtocolVersionID: "v1", StepPosition: 1,
		ParadigmKey: "OPEN_FIELD", ParadigmVersion: 1, EnvironmentRevisionID: "rev", PlannedTrials: 1}
	for name, tc := range map[string]struct {
		scheduled time.Time
		notes     string
		want      error
	}{
		"no schedule":       {time.Time{}, "", ErrMissingTime},
		"before enrollment": {enrolled.Add(-time.Second), "", ErrBeforeEnrollment},
		"long notes":        {now, strings.Repeat("n", 2001), ErrInvalidNotes},
	} {
		if _, err := NewTest(base, tc.scheduled, tc.notes, "user"); !errors.Is(err, tc.want) {
			t.Errorf("%s: err=%v want %v", name, err, tc.want)
		}
	}
	if test, err := NewTest(base, now.Add(48*time.Hour), "", "user"); err != nil || test.GroupID != "" {
		t.Fatalf("a future schedule without a group is allowed: %+v %v", test, err)
	}
}

func TestStatusTransitions(t *testing.T) {
	started, err := planned().Start(at(-30), now, enrolled, "group")
	if err != nil || started.Status != InProgress || !started.StartedAt.Equal(at(-30)) {
		t.Fatalf("start: %+v %v", started, err)
	}
	for name, tc := range map[string]struct {
		run  func() error
		want error
	}{
		"start twice":          {func() error { return second(started.Start(at(-20), now, enrolled, "group")) }, ErrInvalidTransition},
		"start in the future":  {func() error { return second(planned().Start(at(10), now, enrolled, "group")) }, ErrFutureTime},
		"start without a time": {func() error { return second(planned().Start(time.Time{}, now, enrolled, "group")) }, ErrMissingTime},
		"start before enrolling": {func() error {
			return second(planned().Start(enrolled.Add(-time.Minute), now, enrolled, "group"))
		}, ErrBeforeEnrollment},
		"group changed since planning": {func() error { return second(planned().Start(at(-30), now, enrolled, "other")) }, ErrGroupChanged},
		"complete a planned test":      {func() error { return second(planned().Complete(at(-10), now, nil)) }, ErrInvalidTransition},
		"complete without trials":      {func() error { return second(started.Complete(at(-10), now, nil)) }, ErrNoTrials},
		"complete before start": {func() error {
			return second(started.Complete(at(-40), now, []Trial{{Number: 1, Repetition: 1, Attempt: 1, StartedAt: at(-29)}}))
		}, ErrCompletedBeforeTrial},
		"complete before the last trial ended": {func() error {
			end := at(-15)
			return second(started.Complete(at(-20), now, []Trial{{Number: 1, Repetition: 1, Attempt: 1, StartedAt: at(-29), EndedAt: &end}}))
		}, ErrCompletedBeforeTrial},
		"cancel without a reason": {func() error { return second(started.Cancel(" ", now)) }, ErrInvalidReason},
	} {
		if err := tc.run(); !errors.Is(err, tc.want) {
			t.Errorf("%s: err=%v want %v", name, err, tc.want)
		}
	}
	end := at(-15)
	completed, err := started.Complete(at(-10), now, []Trial{{Number: 1, Repetition: 1, Attempt: 1, StartedAt: at(-29), EndedAt: &end}})
	if err != nil || completed.Status != Completed || !completed.CompletedAt.Equal(at(-10)) {
		t.Fatalf("complete: %+v %v", completed, err)
	}
	for _, s := range []Test{completed} {
		if _, err := s.Cancel("mistake", now); !errors.Is(err, ErrInvalidTransition) {
			t.Errorf("cancel a completed test: %v", err)
		}
	}
	cancelled, err := planned().Cancel(" subject unwell ", now)
	if err != nil || cancelled.Status != Cancelled || cancelled.CancelReason != "subject unwell" || !cancelled.CancelledAt.Equal(now) {
		t.Fatalf("cancel: %+v %v", cancelled, err)
	}
	if _, err := cancelled.Start(at(-1), now, enrolled, "group"); !errors.Is(err, ErrInvalidTransition) {
		t.Errorf("start a cancelled test: %v", err)
	}
	for from, targets := range map[Status][]Status{Planned: {InProgress, Cancelled}, InProgress: {Completed, Cancelled}} {
		for _, to := range targets {
			if !CanTransition(from, to) {
				t.Errorf("%s -> %s should be allowed", from, to)
			}
		}
	}
	for _, pair := range [][2]Status{{Planned, Completed}, {InProgress, Planned}, {Completed, InProgress}, {Cancelled, Planned}, {Completed, Cancelled}} {
		if CanTransition(pair[0], pair[1]) {
			t.Errorf("%s -> %s should be rejected", pair[0], pair[1])
		}
	}
}

// A repeated repetition becomes a new attempt; earlier trials are never replaced.
func TestTrialsAreAppendedAsAttempts(t *testing.T) {
	started, _ := planned().Start(at(-60), now, enrolled, "group")
	end := at(-50)
	first, err := NextTrial(started, nil, TrialInput{Repetition: 1, StartedAt: at(-55), EndedAt: &end}, "tech", now)
	if err != nil || first.Number != 1 || first.Attempt != 1 || first.RecordedBy != "tech" {
		t.Fatalf("first: %+v %v", first, err)
	}
	again, err := NextTrial(started, []Trial{first}, TrialInput{Repetition: 1, StartedAt: at(-45), Notes: " fell off "}, "tech", now)
	if err != nil || again.Number != 2 || again.Attempt != 2 || again.Repetition != 1 || again.Notes != "fell off" {
		t.Fatalf("repeat: %+v %v", again, err)
	}
	next, err := NextTrial(started, []Trial{first, again}, TrialInput{Repetition: 2, StartedAt: at(-40)}, "tech", now)
	if err != nil || next.Number != 3 || next.Attempt != 1 {
		t.Fatalf("second repetition: %+v %v", next, err)
	}
	early := at(-61)
	for name, tc := range map[string]struct {
		test Test
		in   TrialInput
		want error
	}{
		"planned test":          {planned(), TrialInput{Repetition: 1, StartedAt: at(-5)}, ErrTestNotInProgress},
		"repetition zero":       {started, TrialInput{Repetition: 0, StartedAt: at(-5)}, ErrInvalidRepetition},
		"beyond planned":        {started, TrialInput{Repetition: 3, StartedAt: at(-5)}, ErrInvalidRepetition},
		"no start":              {started, TrialInput{Repetition: 1}, ErrMissingTime},
		"before the test":       {started, TrialInput{Repetition: 1, StartedAt: early}, ErrTrialBeforeTest},
		"in the future":         {started, TrialInput{Repetition: 1, StartedAt: at(5)}, ErrFutureTime},
		"ends before it starts": {started, TrialInput{Repetition: 1, StartedAt: at(-5), EndedAt: &early}, ErrInvalidTrialEnd},
		"long notes":            {started, TrialInput{Repetition: 1, StartedAt: at(-5), Notes: strings.Repeat("n", 2001)}, ErrInvalidNotes},
	} {
		if _, err := NextTrial(tc.test, nil, tc.in, "tech", now); !errors.Is(err, tc.want) {
			t.Errorf("%s: err=%v want %v", name, err, tc.want)
		}
	}
}

func TestCommentRules(t *testing.T) {
	body, err := NormalizeComment("  line one\n\tline two\x00\x1b ")
	if err != nil || body != "line one\n\tline two" {
		t.Fatalf("normalized: %q %v", body, err)
	}
	for name, raw := range map[string]string{"empty": " \x00 ", "too long": strings.Repeat("é", 2001), "invalid UTF-8": "bad\xff"} {
		if _, err := NormalizeComment(raw); !errors.Is(err, ErrInvalidComment) {
			t.Errorf("%s: %v", name, err)
		}
	}
	if _, err := NormalizeComment(strings.Repeat("é", 2000)); err != nil {
		t.Errorf("2000 characters: %v", err)
	}
	c := Comment{AuthorID: "author"}
	for _, role := range access.Roles() {
		author, other := access.Actor{UserID: "author", Role: role}, access.Actor{UserID: "other", Role: role}
		if !c.EditableBy(author) || c.EditableBy(other) {
			t.Errorf("%s: only the author edits", role)
		}
		if !c.DeletableBy(author) || c.DeletableBy(other) != role.Privileged() {
			t.Errorf("%s: the author or a privileged role deletes", role)
		}
	}
	if c.DeletableBy(access.Actor{UserID: "other", Role: "ROOT"}) {
		t.Error("an unknown role cannot moderate")
	}
}

func second[T any](_ T, err error) error { return err }
