// Package application contains experiment design, enrollment and group
// assignment use cases. Design changes need study:write; reads need *:read.
package application

import (
	"context"
	"errors"
	access "github.com/HappyHackingSpace/Misko/backend/internal/access/domain"
	"github.com/HappyHackingSpace/Misko/backend/internal/experiments/domain"
	"slices"
	"strings"
	"time"
	"unicode/utf8"
)

var (
	ErrExperimentNotFound    = errors.New("experiment not found")
	ErrPhaseNotFound         = errors.New("phase not found in this experiment")
	ErrGroupNotFound         = errors.New("group not found in this experiment")
	ErrEnrollmentNotFound    = errors.New("enrollment not found in this experiment")
	ErrSubjectNotFound       = errors.New("subject not found")
	ErrCodeTaken             = errors.New("experiment code is already in use")
	ErrPhaseConflict         = errors.New("another phase of this experiment uses this name or position")
	ErrGroupNameTaken        = errors.New("another group of this experiment uses this name")
	ErrGroupInUse            = errors.New("group has assignments and cannot be deleted")
	ErrAlreadyEnrolled       = errors.New("subject is already enrolled in this experiment")
	ErrOverlappingAssignment = errors.New("assignment period overlaps another assignment")
	ErrInvalidTime           = errors.New("a timestamp is required")
	ErrNoChanges             = errors.New("no fields to update")
	ErrInvalidQuery          = errors.New("invalid list query")
)

// Changes types hold validated fields; nil leaves a field unchanged. An empty
// description clears it and a zero target size removes the target.
type ExperimentChanges struct {
	Code, Title, Description *string
	RequiresControl          *bool
}

type PhaseChanges struct {
	Name, Description *string
	Position          *int
}

type GroupChanges struct {
	Name, Description *string
	Role              *domain.GroupRole
	TargetSize        *int
}

type ExperimentFilter struct {
	Search        string
	Sort          string
	Descending    bool
	Limit, Offset int
}

type EnrollmentFilter struct {
	ExperimentID, SubjectID, GroupID string
	Descending                       bool
	Limit, Offset                    int
}

// GroupSize compares a group's target with the subjects assigned to it now.
type GroupSize struct {
	Group          domain.Group
	ActiveSubjects int
}

// EnrollmentSummary carries the group active now; CurrentGroupID is empty when none.
type EnrollmentSummary struct {
	Enrollment     domain.Enrollment
	CurrentGroupID string
}

// Store persists experiments. Lookups scoped by experiment return the matching
// not-found error for records of another experiment.
type Store interface {
	// Transaction runs fn atomically; LockEnrollment inside it serializes
	// group changes of that enrollment.
	Transaction(ctx context.Context, fn func(Store) error) error
	CreateExperiment(ctx context.Context, e domain.Experiment) (domain.Experiment, error)
	Experiment(ctx context.Context, id string) (domain.Experiment, error)
	ListExperiments(ctx context.Context, f ExperimentFilter) ([]domain.Experiment, int, error)
	UpdateExperiment(ctx context.Context, id string, c ExperimentChanges) (domain.Experiment, error)
	Phases(ctx context.Context, experimentID string) ([]domain.Phase, error)
	CreatePhase(ctx context.Context, p domain.Phase) (domain.Phase, error)
	UpdatePhase(ctx context.Context, experimentID, phaseID string, c PhaseChanges) (domain.Phase, error)
	DeletePhase(ctx context.Context, experimentID, phaseID string) error
	GroupSizes(ctx context.Context, experimentID string) ([]GroupSize, error)
	Group(ctx context.Context, experimentID, groupID string) (domain.Group, error)
	CreateGroup(ctx context.Context, g domain.Group) (domain.Group, error)
	UpdateGroup(ctx context.Context, experimentID, groupID string, c GroupChanges) (domain.Group, error)
	DeleteGroup(ctx context.Context, experimentID, groupID string) error
	CreateEnrollment(ctx context.Context, e domain.Enrollment) (domain.Enrollment, error)
	LockEnrollment(ctx context.Context, experimentID, enrollmentID string) (domain.Enrollment, error)
	Enrollment(ctx context.Context, experimentID, enrollmentID string) (domain.Enrollment, error)
	ListEnrollments(ctx context.Context, f EnrollmentFilter) ([]EnrollmentSummary, int, error)
	SubjectEnrollments(ctx context.Context, subjectID string) ([]EnrollmentSummary, error)
	Assignments(ctx context.Context, enrollmentID string) ([]domain.Assignment, error)
	CloseAssignment(ctx context.Context, assignmentID string, validTo time.Time) error
	CreateAssignment(ctx context.Context, a domain.Assignment) (domain.Assignment, error)
}

type Service struct {
	store Store
	now   func() time.Time
}

func New(store Store) *Service { return &Service{store: store, now: time.Now} }

type ExperimentQuery struct {
	Search, Sort, Order string
	Page, PageSize      int
}

type ExperimentPage struct {
	Experiments           []domain.Experiment
	Total, Page, PageSize int
}

var experimentSorts = []string{"code", "title", "createdAt"}

func (s *Service) ListExperiments(ctx context.Context, actor access.Actor, q ExperimentQuery) (ExperimentPage, error) {
	if err := actor.Require(access.Read); err != nil {
		return ExperimentPage{}, err
	}
	f := ExperimentFilter{Search: strings.TrimSpace(q.Search), Sort: q.Sort}
	if f.Sort == "" {
		f.Sort = "createdAt"
	}
	if !utf8.ValidString(f.Search) || utf8.RuneCountInString(f.Search) > 100 || !slices.Contains(experimentSorts, f.Sort) {
		return ExperimentPage{}, ErrInvalidQuery
	}
	page := 0
	var err error
	if f.Descending, err = descending(q.Order, f.Sort == "createdAt"); err != nil {
		return ExperimentPage{}, err
	}
	if page, f.Limit, f.Offset, err = paging(q.Page, q.PageSize); err != nil {
		return ExperimentPage{}, err
	}
	experiments, total, err := s.store.ListExperiments(ctx, f)
	if err != nil {
		return ExperimentPage{}, err
	}
	return ExperimentPage{Experiments: experiments, Total: total, Page: page, PageSize: f.Limit}, nil
}

type ExperimentDetail struct {
	Experiment            domain.Experiment
	ControlRequirementMet bool
}

func (s *Service) Experiment(ctx context.Context, actor access.Actor, id string) (ExperimentDetail, error) {
	if err := actor.Require(access.Read); err != nil {
		return ExperimentDetail{}, err
	}
	e, err := s.store.Experiment(ctx, id)
	if err != nil {
		return ExperimentDetail{}, err
	}
	sizes, err := s.store.GroupSizes(ctx, id)
	if err != nil {
		return ExperimentDetail{}, err
	}
	groups := make([]domain.Group, 0, len(sizes))
	for _, size := range sizes {
		groups = append(groups, size.Group)
	}
	return ExperimentDetail{Experiment: e, ControlRequirementMet: domain.ControlRequirementMet(e.RequiresControl, groups)}, nil
}

type ExperimentInput struct {
	Code, Title, Description string
	RequiresControl          bool
}

func (s *Service) CreateExperiment(ctx context.Context, actor access.Actor, in ExperimentInput) (domain.Experiment, error) {
	if err := actor.Require(access.StudyWrite); err != nil {
		return domain.Experiment{}, err
	}
	e, err := domain.NewExperiment(in.Code, in.Title, in.Description, in.RequiresControl)
	if err != nil {
		return domain.Experiment{}, err
	}
	return s.store.CreateExperiment(ctx, e)
}

type ExperimentPatch struct {
	Code, Title, Description *string
	RequiresControl          *bool
}

func (s *Service) UpdateExperiment(ctx context.Context, actor access.Actor, id string, p ExperimentPatch) (domain.Experiment, error) {
	if err := actor.Require(access.StudyWrite); err != nil {
		return domain.Experiment{}, err
	}
	if p == (ExperimentPatch{}) {
		return domain.Experiment{}, ErrNoChanges
	}
	c := ExperimentChanges{RequiresControl: p.RequiresControl}
	var err error
	if c.Code, err = optional(p.Code, domain.NormalizeCode); err != nil {
		return domain.Experiment{}, err
	}
	if c.Title, err = optional(p.Title, domain.NormalizeTitle); err != nil {
		return domain.Experiment{}, err
	}
	if c.Description, err = optional(p.Description, domain.NormalizeDescription); err != nil {
		return domain.Experiment{}, err
	}
	return s.store.UpdateExperiment(ctx, id, c)
}

func (s *Service) Phases(ctx context.Context, actor access.Actor, experimentID string) ([]domain.Phase, error) {
	if err := s.readExperiment(ctx, actor, experimentID); err != nil {
		return nil, err
	}
	return s.store.Phases(ctx, experimentID)
}

type PhaseInput struct {
	Name, Description string
	Position          int
}

func (s *Service) CreatePhase(ctx context.Context, actor access.Actor, experimentID string, in PhaseInput) (domain.Phase, error) {
	if err := actor.Require(access.StudyWrite); err != nil {
		return domain.Phase{}, err
	}
	p, err := domain.NewPhase(experimentID, in.Name, in.Position, in.Description)
	if err != nil {
		return domain.Phase{}, err
	}
	return s.store.CreatePhase(ctx, p)
}

type PhasePatch struct {
	Name, Description *string
	Position          *int
}

func (s *Service) UpdatePhase(ctx context.Context, actor access.Actor, experimentID, phaseID string, p PhasePatch) (domain.Phase, error) {
	if err := actor.Require(access.StudyWrite); err != nil {
		return domain.Phase{}, err
	}
	if p == (PhasePatch{}) {
		return domain.Phase{}, ErrNoChanges
	}
	c := PhaseChanges{Position: p.Position}
	var err error
	if c.Name, err = optional(p.Name, domain.NormalizeName); err != nil {
		return domain.Phase{}, err
	}
	if c.Description, err = optional(p.Description, domain.NormalizeDescription); err != nil {
		return domain.Phase{}, err
	}
	if p.Position != nil {
		if err := domain.ValidatePosition(*p.Position); err != nil {
			return domain.Phase{}, err
		}
	}
	return s.store.UpdatePhase(ctx, experimentID, phaseID, c)
}

func (s *Service) DeletePhase(ctx context.Context, actor access.Actor, experimentID, phaseID string) error {
	if err := actor.Require(access.StudyWrite); err != nil {
		return err
	}
	return s.store.DeletePhase(ctx, experimentID, phaseID)
}

func (s *Service) Groups(ctx context.Context, actor access.Actor, experimentID string) ([]GroupSize, error) {
	if err := s.readExperiment(ctx, actor, experimentID); err != nil {
		return nil, err
	}
	return s.store.GroupSizes(ctx, experimentID)
}

type GroupInput struct {
	Name, Role, Description string
	TargetSize              int
}

func (s *Service) CreateGroup(ctx context.Context, actor access.Actor, experimentID string, in GroupInput) (domain.Group, error) {
	if err := actor.Require(access.StudyWrite); err != nil {
		return domain.Group{}, err
	}
	g, err := domain.NewGroup(experimentID, in.Name, in.Role, in.TargetSize, in.Description)
	if err != nil {
		return domain.Group{}, err
	}
	return s.store.CreateGroup(ctx, g)
}

type GroupPatch struct {
	Name, Role, Description *string
	TargetSize              *int
}

func (s *Service) UpdateGroup(ctx context.Context, actor access.Actor, experimentID, groupID string, p GroupPatch) (domain.Group, error) {
	if err := actor.Require(access.StudyWrite); err != nil {
		return domain.Group{}, err
	}
	if p == (GroupPatch{}) {
		return domain.Group{}, ErrNoChanges
	}
	c := GroupChanges{TargetSize: p.TargetSize}
	var err error
	if c.Name, err = optional(p.Name, domain.NormalizeName); err != nil {
		return domain.Group{}, err
	}
	if c.Description, err = optional(p.Description, domain.NormalizeDescription); err != nil {
		return domain.Group{}, err
	}
	if c.Role, err = optional(p.Role, domain.ParseRole); err != nil {
		return domain.Group{}, err
	}
	if p.TargetSize != nil {
		if err := domain.ValidateTargetSize(*p.TargetSize); err != nil {
			return domain.Group{}, err
		}
	}
	return s.store.UpdateGroup(ctx, experimentID, groupID, c)
}

func (s *Service) DeleteGroup(ctx context.Context, actor access.Actor, experimentID, groupID string) error {
	if err := actor.Require(access.StudyWrite); err != nil {
		return err
	}
	return s.store.DeleteGroup(ctx, experimentID, groupID)
}

// EnrollInput optionally assigns an initial group starting at enrollment.
type EnrollInput struct {
	SubjectID, GroupID string
	EnrolledAt         time.Time
}

func (s *Service) Enroll(ctx context.Context, actor access.Actor, experimentID string, in EnrollInput) (EnrollmentSummary, error) {
	if err := actor.Require(access.StudyWrite); err != nil {
		return EnrollmentSummary{}, err
	}
	if in.EnrolledAt.IsZero() {
		return EnrollmentSummary{}, ErrInvalidTime
	}
	if strings.TrimSpace(in.SubjectID) == "" {
		return EnrollmentSummary{}, ErrSubjectNotFound
	}
	at := instant(in.EnrolledAt)
	var summary EnrollmentSummary
	err := s.store.Transaction(ctx, func(tx Store) error {
		e, err := tx.CreateEnrollment(ctx, domain.Enrollment{ExperimentID: experimentID, SubjectID: in.SubjectID, EnrolledAt: at})
		if err != nil {
			return err
		}
		summary = EnrollmentSummary{Enrollment: e}
		if in.GroupID == "" {
			return nil
		}
		a, err := s.assign(ctx, tx, e, nil, in.GroupID, at)
		if err == nil && a.ActiveAt(s.now()) {
			summary.CurrentGroupID = a.GroupID
		}
		return err
	})
	if err != nil {
		return EnrollmentSummary{}, err
	}
	return summary, nil
}

type EnrollmentQuery struct {
	SubjectID, GroupID, Order string
	Page, PageSize            int
}

type EnrollmentPage struct {
	Enrollments           []EnrollmentSummary
	Total, Page, PageSize int
}

func (s *Service) ListEnrollments(ctx context.Context, actor access.Actor, experimentID string, q EnrollmentQuery) (EnrollmentPage, error) {
	if err := actor.Require(access.Read); err != nil {
		return EnrollmentPage{}, err
	}
	f := EnrollmentFilter{ExperimentID: experimentID, SubjectID: q.SubjectID, GroupID: q.GroupID}
	page := 0
	var err error
	if f.Descending, err = descending(q.Order, true); err != nil {
		return EnrollmentPage{}, err
	}
	if page, f.Limit, f.Offset, err = paging(q.Page, q.PageSize); err != nil {
		return EnrollmentPage{}, err
	}
	if _, err := s.store.Experiment(ctx, experimentID); err != nil {
		return EnrollmentPage{}, err
	}
	enrollments, total, err := s.store.ListEnrollments(ctx, f)
	if err != nil {
		return EnrollmentPage{}, err
	}
	return EnrollmentPage{Enrollments: enrollments, Total: total, Page: page, PageSize: f.Limit}, nil
}

// EnrollmentDetail includes the full assignment history, oldest first.
type EnrollmentDetail struct {
	Enrollment     domain.Enrollment
	Assignments    []domain.Assignment
	CurrentGroupID string
}

func (s *Service) Enrollment(ctx context.Context, actor access.Actor, experimentID, enrollmentID string) (EnrollmentDetail, error) {
	if err := actor.Require(access.Read); err != nil {
		return EnrollmentDetail{}, err
	}
	e, err := s.store.Enrollment(ctx, experimentID, enrollmentID)
	if err != nil {
		return EnrollmentDetail{}, err
	}
	history, err := s.store.Assignments(ctx, e.ID)
	if err != nil {
		return EnrollmentDetail{}, err
	}
	current, _ := domain.GroupAt(history, s.now())
	return EnrollmentDetail{Enrollment: e, Assignments: history, CurrentGroupID: current}, nil
}

type AssignInput struct {
	GroupID       string
	EffectiveFrom time.Time
}

// Assign moves an enrollment into a group from EffectiveFrom, closing the
// open assignment at that instant. Earlier periods are kept as history.
func (s *Service) Assign(ctx context.Context, actor access.Actor, experimentID, enrollmentID string, in AssignInput) (domain.Assignment, error) {
	if err := actor.Require(access.StudyWrite); err != nil {
		return domain.Assignment{}, err
	}
	if in.EffectiveFrom.IsZero() {
		return domain.Assignment{}, ErrInvalidTime
	}
	if in.GroupID == "" {
		return domain.Assignment{}, domain.ErrGroupNotInExperiment
	}
	var created domain.Assignment
	err := s.store.Transaction(ctx, func(tx Store) error {
		e, err := tx.LockEnrollment(ctx, experimentID, enrollmentID)
		if err != nil {
			return err
		}
		history, err := tx.Assignments(ctx, e.ID)
		if err != nil {
			return err
		}
		created, err = s.assign(ctx, tx, e, domain.OpenAssignment(history), in.GroupID, instant(in.EffectiveFrom))
		return err
	})
	if err != nil {
		return domain.Assignment{}, err
	}
	return created, nil
}

func (s *Service) assign(ctx context.Context, tx Store, e domain.Enrollment, current *domain.Assignment, groupID string, from time.Time) (domain.Assignment, error) {
	g, err := tx.Group(ctx, e.ExperimentID, groupID)
	if errors.Is(err, ErrGroupNotFound) {
		return domain.Assignment{}, domain.ErrGroupNotInExperiment
	}
	if err != nil {
		return domain.Assignment{}, err
	}
	if err := domain.PlanAssignment(e, current, g, from); err != nil {
		return domain.Assignment{}, err
	}
	if current != nil {
		if err := tx.CloseAssignment(ctx, current.ID, from); err != nil {
			return domain.Assignment{}, err
		}
	}
	return tx.CreateAssignment(ctx, domain.Assignment{ExperimentID: e.ExperimentID, EnrollmentID: e.ID, GroupID: g.ID, ValidFrom: from})
}

// SubjectEnrollments lists every experiment a subject is enrolled in.
func (s *Service) SubjectEnrollments(ctx context.Context, actor access.Actor, subjectID string) ([]EnrollmentSummary, error) {
	if err := actor.Require(access.Read); err != nil {
		return nil, err
	}
	return s.store.SubjectEnrollments(ctx, subjectID)
}

func (s *Service) readExperiment(ctx context.Context, actor access.Actor, id string) error {
	if err := actor.Require(access.Read); err != nil {
		return err
	}
	_, err := s.store.Experiment(ctx, id)
	return err
}

// instant stores times in UTC at the database's microsecond precision.
func instant(t time.Time) time.Time { return t.UTC().Truncate(time.Microsecond) }

func optional[T any](raw *string, parse func(string) (T, error)) (*T, error) {
	if raw == nil {
		return nil, nil
	}
	value, err := parse(*raw)
	if err != nil {
		return nil, err
	}
	return &value, nil
}

func descending(order string, fallback bool) (bool, error) {
	switch order {
	case "":
		return fallback, nil
	case "asc":
		return false, nil
	case "desc":
		return true, nil
	}
	return false, ErrInvalidQuery
}

// paging defaults to 10 rows per page and caps the page size at 100.
func paging(page, size int) (current, limit, offset int, err error) {
	if page < 0 || page > 1<<20 || size < 0 {
		return 0, 0, 0, ErrInvalidQuery
	}
	current, limit = max(page, 1), min(size, 100)
	if limit == 0 {
		limit = 10
	}
	return current, limit, (current - 1) * limit, nil
}
