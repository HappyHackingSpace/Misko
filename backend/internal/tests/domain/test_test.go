package domain

import (
	"errors"
	access "github.com/HappyHackingSpace/Misko/backend/internal/access/domain"
	"strings"
	"testing"
	"time"
)

var now = time.Date(2026, 9, 11, 12, 0, 0, 0, time.UTC)

var session = Context{ExperimentID: "exp", TimepointID: "day7", ProtocolVersionID: "v1", StepPosition: 1, ParadigmKey: "OPEN_FIELD", ParadigmVersion: 1, PlannedTrials: 2}

// planned returns a planned test with two animals whose subjects have ids s1 and s2.
func planned() Test {
	t, err := NewTest(session, now.Add(-time.Hour), "", "user", []Participant{{"a1", "arena1"}, {"a2", "arena2"}})
	if err != nil {
		panic(err)
	}
	t.ID = "test"
	t.Subjects[0].ID, t.Subjects[1].ID = "s1", "s2"
	return t
}

func at(minutes int) time.Time { return now.Add(time.Duration(minutes) * time.Minute) }

func TestNewTestKeepsItsContextAndAnimals(t *testing.T) {
	test := planned()
	if test.Status != Planned || test.TimepointID != "day7" || test.PlannedTrials != 2 || test.CreatedBy != "user" || len(test.Subjects) != 2 ||
		test.Subjects[1].AllocationID != "a2" || test.Subjects[1].EnvironmentRevisionID != "arena2" || test.Subjects[0].CreatedBy != "user" {
		t.Fatalf("test: %+v", test)
	}
	many := make([]Participant, MaxSubjects+1)
	for i := range many {
		many[i] = Participant{string(rune('a' + i)), string(rune('A' + i))}
	}
	one := []Participant{{"a1", "arena1"}}
	for name, tc := range map[string]struct {
		scheduled    time.Time
		notes        string
		participants []Participant
		want         error
	}{
		"no schedule":   {time.Time{}, "", one, ErrMissingTime},
		"long notes":    {now, strings.Repeat("n", 2001), one, ErrInvalidNotes},
		"no animals":    {now, "", nil, ErrNoSubjects},
		"too many":      {now, "", many, ErrTooManySubjects},
		"no allocation": {now, "", []Participant{{"", "arena1"}}, ErrMissingAllocation},
		"no arena":      {now, "", []Participant{{"a1", ""}}, ErrMissingArena},
		"animal twice":  {now, "", []Participant{{"a1", "arena1"}, {"a1", "arena2"}}, ErrDuplicateAnimal},
		"arena twice":   {now, "", []Participant{{"a1", "arena1"}, {"a2", "arena1"}}, ErrDuplicateArena},
	} {
		if _, err := NewTest(session, tc.scheduled, tc.notes, "user", tc.participants); !errors.Is(err, tc.want) {
			t.Errorf("%s: err=%v want %v", name, err, tc.want)
		}
	}
}

func TestAnimalsChangeOnlyWhilePlanned(t *testing.T) {
	test := planned()
	added, err := test.AddSubject(Participant{"a3", "arena3"}, "planner")
	if err != nil || added.TestID != "test" || added.AllocationID != "a3" || added.CreatedBy != "planner" {
		t.Fatalf("add: %+v %v", added, err)
	}
	if _, err := test.AddSubject(Participant{"a1", "arena3"}, "planner"); !errors.Is(err, ErrDuplicateAnimal) {
		t.Errorf("same animal: %v", err)
	}
	if _, err := test.AddSubject(Participant{"a3", "arena2"}, "planner"); !errors.Is(err, ErrDuplicateArena) {
		t.Errorf("occupied arena: %v", err)
	}
	if s, ok := test.Subject("s2"); !ok || s.AllocationID != "a2" {
		t.Errorf("subject lookup: %+v %v", s, ok)
	}
	if _, ok := test.Subject("missing"); ok {
		t.Error("unknown subject found")
	}
	if err := test.CanRemoveSubject(); err != nil {
		t.Errorf("remove one of two: %v", err)
	}
	alone := planned()
	alone.Subjects = alone.Subjects[:1]
	if err := alone.CanRemoveSubject(); !errors.Is(err, ErrLastSubject) {
		t.Errorf("remove the last animal: %v", err)
	}
	started, _ := planned().Start(at(-30), now)
	if _, err := started.AddSubject(Participant{"a3", "arena3"}, "planner"); !errors.Is(err, ErrNotPlanned) {
		t.Errorf("add after start: %v", err)
	}
	if err := started.CanRemoveSubject(); !errors.Is(err, ErrNotPlanned) {
		t.Errorf("remove after start: %v", err)
	}
}

func TestStatusTransitions(t *testing.T) {
	started, err := planned().Start(at(-30), now)
	if err != nil || started.Status != InProgress || !started.StartedAt.Equal(at(-30)) {
		t.Fatalf("start: %+v %v", started, err)
	}
	for name, tc := range map[string]struct {
		run  func() error
		want error
	}{
		"start twice":          {func() error { return second(started.Start(at(-20), now)) }, ErrInvalidTransition},
		"start in the future":  {func() error { return second(planned().Start(at(10), now)) }, ErrFutureTime},
		"start without a time": {func() error { return second(planned().Start(time.Time{}, now)) }, ErrMissingTime},
		"start with an excluded animal": {func() error {
			test := planned()
			test.Subjects[1].Excluded = true
			return second(test.Start(at(-30), now))
		}, ErrSubjectExcluded},
		"complete a planned test": {func() error { return second(planned().Complete(at(-10), now, nil)) }, ErrInvalidTransition},
		"complete without trials": {func() error { return second(started.Complete(at(-10), now, nil)) }, ErrNoTrials},
		"complete with an animal without trials": {func() error {
			return second(started.Complete(at(-10), now, []Trial{{TestSubjectID: "s1", Number: 1, Repetition: 1, Attempt: 1, StartedAt: at(-29)}}))
		}, ErrNoTrials},
		"complete before start": {func() error {
			return second(started.Complete(at(-40), now, []Trial{{TestSubjectID: "s1", Number: 1, Repetition: 1, Attempt: 1, StartedAt: at(-29)}}))
		}, ErrCompletedBeforeTrial},
		"complete before the last trial ended": {func() error {
			end := at(-15)
			return second(started.Complete(at(-20), now, []Trial{{TestSubjectID: "s1", Number: 1, Repetition: 1, Attempt: 1, StartedAt: at(-29), EndedAt: &end}}))
		}, ErrCompletedBeforeTrial},
		"cancel without a reason": {func() error { return second(started.Cancel(" ", now)) }, ErrInvalidReason},
	} {
		if err := tc.run(); !errors.Is(err, tc.want) {
			t.Errorf("%s: err=%v want %v", name, err, tc.want)
		}
	}
	end := at(-15)
	completed, err := started.Complete(at(-10), now, []Trial{
		{TestSubjectID: "s1", Number: 1, Repetition: 1, Attempt: 1, StartedAt: at(-29), EndedAt: &end},
		{TestSubjectID: "s2", Number: 1, Repetition: 1, Attempt: 1, StartedAt: at(-29)},
	})
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
	if _, err := cancelled.Start(at(-1), now); !errors.Is(err, ErrInvalidTransition) {
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

// A repeated repetition becomes a new attempt; earlier trials are never
// replaced. Each animal numbers its own trials.
func TestTrialsAreAppendedAsAttemptsPerAnimal(t *testing.T) {
	started, _ := planned().Start(at(-60), now)
	end := at(-50)
	first, err := NextTrial(started, "s1", nil, TrialInput{Repetition: 1, StartedAt: at(-55), EndedAt: &end}, "tech", now)
	if err != nil || first.Number != 1 || first.Attempt != 1 || first.RecordedBy != "tech" || first.TestSubjectID != "s1" || first.TestID != "test" {
		t.Fatalf("first: %+v %v", first, err)
	}
	again, err := NextTrial(started, "s1", []Trial{first}, TrialInput{Repetition: 1, StartedAt: at(-45), Notes: " fell off "}, "tech", now)
	if err != nil || again.Number != 2 || again.Attempt != 2 || again.Repetition != 1 || again.Notes != "fell off" {
		t.Fatalf("repeat: %+v %v", again, err)
	}
	next, err := NextTrial(started, "s1", []Trial{first, again}, TrialInput{Repetition: 2, StartedAt: at(-40)}, "tech", now)
	if err != nil || next.Number != 3 || next.Attempt != 1 {
		t.Fatalf("second repetition: %+v %v", next, err)
	}
	other, err := NextTrial(started, "s2", []Trial{first, again, next}, TrialInput{Repetition: 1, StartedAt: at(-40)}, "tech", now)
	if err != nil || other.Number != 1 || other.Attempt != 1 || other.TestSubjectID != "s2" {
		t.Fatalf("other animal: %+v %v", other, err)
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
		if _, err := NextTrial(tc.test, "s1", nil, tc.in, "tech", now); !errors.Is(err, tc.want) {
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
