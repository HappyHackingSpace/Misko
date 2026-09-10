package application

import (
	"context"
	"errors"
	access "github.com/HappyHackingSpace/Misko/backend/internal/access/domain"
	"github.com/HappyHackingSpace/Misko/backend/internal/experiments/domain"
	"testing"
	"time"
)

// Writes by roles without study:write must fail before any store call. The
// allowed paths run against PostgreSQL in the adapter and HTTP integration tests.
func TestExperimentPermissions(t *testing.T) {
	ctx := context.Background()
	name, position := "Renamed", 2
	writes := map[string]func(*Service, access.Actor) error{
		"create experiment": func(s *Service, a access.Actor) error {
			_, err := s.CreateExperiment(ctx, a, ExperimentInput{Code: "E", Title: "T"})
			return err
		},
		"update experiment": func(s *Service, a access.Actor) error {
			_, err := s.UpdateExperiment(ctx, a, "x", ExperimentPatch{Title: &name})
			return err
		},
		"create phase": func(s *Service, a access.Actor) error {
			_, err := s.CreatePhase(ctx, a, "x", PhaseInput{Name: "P", Position: 1})
			return err
		},
		"update phase": func(s *Service, a access.Actor) error {
			_, err := s.UpdatePhase(ctx, a, "x", "p", PhasePatch{Position: &position})
			return err
		},
		"delete phase": func(s *Service, a access.Actor) error { return s.DeletePhase(ctx, a, "x", "p") },
		"create group": func(s *Service, a access.Actor) error {
			_, err := s.CreateGroup(ctx, a, "x", GroupInput{Name: "G", Role: "CONTROL"})
			return err
		},
		"update group": func(s *Service, a access.Actor) error {
			_, err := s.UpdateGroup(ctx, a, "x", "g", GroupPatch{Name: &name})
			return err
		},
		"delete group": func(s *Service, a access.Actor) error { return s.DeleteGroup(ctx, a, "x", "g") },
		"enroll": func(s *Service, a access.Actor) error {
			_, err := s.Enroll(ctx, a, "x", EnrollInput{SubjectID: "s", EnrolledAt: time.Now()})
			return err
		},
		"assign": func(s *Service, a access.Actor) error {
			_, err := s.Assign(ctx, a, "x", "e", AssignInput{GroupID: "g", EffectiveFrom: time.Now()})
			return err
		},
	}
	reads := map[string]func(*Service, access.Actor) error{
		"list experiments": func(s *Service, a access.Actor) error {
			_, err := s.ListExperiments(ctx, a, ExperimentQuery{})
			return err
		},
		"get experiment": func(s *Service, a access.Actor) error { _, err := s.Experiment(ctx, a, "x"); return err },
		"phases":         func(s *Service, a access.Actor) error { _, err := s.Phases(ctx, a, "x"); return err },
		"groups":         func(s *Service, a access.Actor) error { _, err := s.Groups(ctx, a, "x"); return err },
		"list enrollments": func(s *Service, a access.Actor) error {
			_, err := s.ListEnrollments(ctx, a, "x", EnrollmentQuery{})
			return err
		},
		"get enrollment":      func(s *Service, a access.Actor) error { _, err := s.Enrollment(ctx, a, "x", "e"); return err },
		"subject enrollments": func(s *Service, a access.Actor) error { _, err := s.SubjectEnrollments(ctx, a, "s"); return err },
	}
	for _, role := range append(access.Roles(), "ROOT", "") {
		actor := access.Actor{UserID: "u", Role: role}
		if role == "" {
			actor = access.Actor{}
		}
		for name, write := range writes {
			if role.Allows(access.StudyWrite) {
				continue
			}
			if err := write(New(readOnlyStore{}), actor); !errors.Is(err, access.ErrForbidden) && !errors.Is(err, access.ErrUnauthenticated) {
				t.Errorf("%q %s: err=%v", role, name, err)
			}
		}
		for name, read := range reads {
			err := read(New(readOnlyStore{}), actor)
			if role.Allows(access.Read) != (err == nil) {
				t.Errorf("%q %s: err=%v", role, name, err)
			}
		}
	}
}

func TestInputsAreValidatedBeforeTheStore(t *testing.T) {
	ctx := context.Background()
	s := New(readOnlyStore{})
	researcher := access.Actor{UserID: "u", Role: access.Researcher}
	blank := " "
	for name, err := range map[string]error{
		"zero enrollment time": second(s.Enroll(ctx, researcher, "x", EnrollInput{SubjectID: "s"})),
		"zero assignment time": second(s.Assign(ctx, researcher, "x", "e", AssignInput{GroupID: "g"})),
		"missing group":        second(s.Assign(ctx, researcher, "x", "e", AssignInput{EffectiveFrom: time.Now()})),
		"invalid phase":        second(s.CreatePhase(ctx, researcher, "x", PhaseInput{Name: "P", Position: 0})),
		"invalid group role":   second(s.CreateGroup(ctx, researcher, "x", GroupInput{Name: "G", Role: "PLACEBO"})),
		"empty patch":          second(s.UpdateGroup(ctx, researcher, "x", "g", GroupPatch{})),
		"blank title":          second(s.UpdateExperiment(ctx, researcher, "x", ExperimentPatch{Title: &blank})),
		"unknown sort":         second(s.ListExperiments(ctx, researcher, ExperimentQuery{Sort: "description"})),
		"unknown order":        second(s.ListEnrollments(ctx, researcher, "x", EnrollmentQuery{Order: "random"})),
	} {
		switch {
		case err == nil:
			t.Errorf("%s accepted", name)
		case errors.Is(err, errUnexpectedStoreCall):
			t.Errorf("%s reached the store", name)
		}
	}
	if _, err := s.CreateGroup(ctx, researcher, "x", GroupInput{Name: "G", Role: "CONTROL", TargetSize: -1}); !errors.Is(err, domain.ErrInvalidTargetSize) {
		t.Errorf("negative target: %v", err)
	}
}

var errUnexpectedStoreCall = errors.New("unexpected store call")

// readOnlyStore answers reads with empty data and fails every write.
type readOnlyStore struct{}

func (readOnlyStore) Transaction(context.Context, func(Store) error) error {
	return errUnexpectedStoreCall
}
func (readOnlyStore) CreateExperiment(context.Context, domain.Experiment) (domain.Experiment, error) {
	return domain.Experiment{}, errUnexpectedStoreCall
}
func (readOnlyStore) Experiment(context.Context, string) (domain.Experiment, error) {
	return domain.Experiment{}, nil
}
func (readOnlyStore) ListExperiments(context.Context, ExperimentFilter) ([]domain.Experiment, int, error) {
	return nil, 0, nil
}
func (readOnlyStore) UpdateExperiment(context.Context, string, ExperimentChanges) (domain.Experiment, error) {
	return domain.Experiment{}, errUnexpectedStoreCall
}
func (readOnlyStore) Phases(context.Context, string) ([]domain.Phase, error) { return nil, nil }
func (readOnlyStore) CreatePhase(context.Context, domain.Phase) (domain.Phase, error) {
	return domain.Phase{}, errUnexpectedStoreCall
}
func (readOnlyStore) UpdatePhase(context.Context, string, string, PhaseChanges) (domain.Phase, error) {
	return domain.Phase{}, errUnexpectedStoreCall
}
func (readOnlyStore) DeletePhase(context.Context, string, string) error {
	return errUnexpectedStoreCall
}
func (readOnlyStore) GroupSizes(context.Context, string) ([]GroupSize, error) {
	return nil, nil
}
func (readOnlyStore) Group(context.Context, string, string) (domain.Group, error) {
	return domain.Group{}, nil
}
func (readOnlyStore) CreateGroup(context.Context, domain.Group) (domain.Group, error) {
	return domain.Group{}, errUnexpectedStoreCall
}
func (readOnlyStore) UpdateGroup(context.Context, string, string, GroupChanges) (domain.Group, error) {
	return domain.Group{}, errUnexpectedStoreCall
}
func (readOnlyStore) DeleteGroup(context.Context, string, string) error {
	return errUnexpectedStoreCall
}
func (readOnlyStore) CreateEnrollment(context.Context, domain.Enrollment) (domain.Enrollment, error) {
	return domain.Enrollment{}, errUnexpectedStoreCall
}
func (readOnlyStore) LockEnrollment(context.Context, string, string) (domain.Enrollment, error) {
	return domain.Enrollment{}, errUnexpectedStoreCall
}
func (readOnlyStore) Enrollment(context.Context, string, string) (domain.Enrollment, error) {
	return domain.Enrollment{}, nil
}
func (readOnlyStore) ListEnrollments(context.Context, EnrollmentFilter) ([]EnrollmentSummary, int, error) {
	return nil, 0, nil
}
func (readOnlyStore) SubjectEnrollments(context.Context, string) ([]EnrollmentSummary, error) {
	return nil, nil
}
func (readOnlyStore) Assignments(context.Context, string) ([]domain.Assignment, error) {
	return nil, nil
}
func (readOnlyStore) CloseAssignment(context.Context, string, time.Time) error {
	return errUnexpectedStoreCall
}
func (readOnlyStore) CreateAssignment(context.Context, domain.Assignment) (domain.Assignment, error) {
	return domain.Assignment{}, errUnexpectedStoreCall
}

func second[T any](_ T, err error) error { return err }
