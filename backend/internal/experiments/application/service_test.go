package application

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	access "github.com/HappyHackingSpace/Misko/backend/internal/access/domain"
	"github.com/HappyHackingSpace/Misko/backend/internal/experiments/domain"
)

var (
	ctx        = context.Background()
	researcher = access.Actor{UserID: "u", Role: access.Researcher}
	now        = time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)
)

// memory is an in-memory Store with just enough behaviour for the use cases;
// the PostgreSQL store is covered by the integration tests. Methods it does
// not implement panic through the nil embedded Store.
type memory struct {
	Store
	experiment  domain.Experiment
	groups      []GroupFill
	items       []domain.RegimenItem
	timepoints  []domain.Timepoint
	endpoints   []domain.Endpoint
	paradigms   []ParadigmVersion
	allocations []domain.Allocation
	openTests   int
	calls       int
}

func (m *memory) Transaction(_ context.Context, fn func(Store) error) error { return fn(m) }

func (m *memory) LockExperiment(_ context.Context, id string) (domain.Experiment, error) {
	m.calls++
	if id != m.experiment.ID {
		return domain.Experiment{}, ErrExperimentNotFound
	}
	return m.experiment, nil
}

func (m *memory) Experiment(c context.Context, id string) (domain.Experiment, error) {
	return m.LockExperiment(c, id)
}

func (m *memory) UpdateExperiment(_ context.Context, e domain.Experiment) (domain.Experiment, error) {
	m.experiment = e
	return e, nil
}

func (m *memory) StartExperiment(_ context.Context, _ string, day time.Time) (domain.Experiment, error) {
	m.experiment.Status, m.experiment.DayZero = domain.Running, &day
	return m.experiment, nil
}

func (m *memory) CompleteExperiment(context.Context, string) (domain.Experiment, error) {
	m.experiment.Status = domain.Completed
	return m.experiment, nil
}

func (m *memory) OpenTests(context.Context, string) (int, error) { return m.openTests, nil }
func (m *memory) ProtocolParadigms(context.Context, string) ([]ParadigmVersion, error) {
	return m.paradigms, nil
}
func (m *memory) Groups(context.Context, string) ([]GroupFill, error) { return m.groups, nil }
func (m *memory) RegimenItems(context.Context, string) ([]domain.RegimenItem, error) {
	return m.items, nil
}
func (m *memory) Timepoints(context.Context, string) ([]domain.Timepoint, error) {
	return m.timepoints, nil
}
func (m *memory) Endpoints(context.Context, string) ([]domain.Endpoint, error) {
	return m.endpoints, nil
}

func (m *memory) CreateEndpoint(_ context.Context, e domain.Endpoint) (domain.Endpoint, error) {
	m.endpoints = append(m.endpoints, e)
	return e, nil
}

func (m *memory) CreateGroup(_ context.Context, g domain.Group) (domain.Group, error) { return g, nil }

func (m *memory) LockGroup(_ context.Context, _, id string) (domain.Group, error) {
	for _, f := range m.groups {
		if f.Group.ID == id {
			return f.Group, nil
		}
	}
	return domain.Group{}, ErrGroupNotFound
}

func (m *memory) CreateRegimenItem(_ context.Context, r domain.RegimenItem) (domain.RegimenItem, error) {
	m.items = append(m.items, r)
	return r, nil
}

func (m *memory) LockRegimenItem(_ context.Context, _, id string) (domain.RegimenItem, error) {
	for _, r := range m.items {
		if r.ID == id {
			return r, nil
		}
	}
	return domain.RegimenItem{}, ErrRegimenItemNotFound
}

func (m *memory) UpdateRegimenItem(_ context.Context, r domain.RegimenItem) (domain.RegimenItem, error) {
	return r, nil
}

func (m *memory) CreateAllocation(_ context.Context, a domain.Allocation) (domain.Allocation, error) {
	m.allocations = append(m.allocations, a)
	return a, nil
}

func (m *memory) LockAllocation(_ context.Context, _, id string) (domain.Allocation, error) {
	for _, a := range m.allocations {
		if a.ID == id {
			return a, nil
		}
	}
	return domain.Allocation{}, ErrAllocationNotFound
}

func (m *memory) UpdateAllocation(_ context.Context, a domain.Allocation) (domain.Allocation, error) {
	for i := range m.allocations {
		if m.allocations[i].ID == a.ID {
			m.allocations[i] = a
		}
	}
	return a, nil
}

func (m *memory) DeleteAllocation(context.Context, string, string) error { return nil }

type metrics map[string][]string

func (c metrics) Metrics(key string, version int) ([]string, error) { return c[key], nil }

func service(m *memory) *Service {
	return New(m, metrics{"MWM": {"path_length_cm", "escape_latency_s"}}, func() time.Time { return now }, func(n int) int { return n - 1 })
}

// Writes by roles without study:write fail before the store is touched.
func TestPermissions(t *testing.T) {
	text := "x"
	writes := map[string]func(*Service, access.Actor) error{
		"create experiment": func(s *Service, a access.Actor) error {
			_, err := s.CreateExperiment(ctx, a, domain.ExperimentFields{Code: "E", Title: "T"})
			return err
		},
		"update experiment": func(s *Service, a access.Actor) error {
			_, err := s.UpdateExperiment(ctx, a, "e", ExperimentPatch{Title: &text})
			return err
		},
		"start":    func(s *Service, a access.Actor) error { _, err := s.Start(ctx, a, "e", "2026-10-15"); return err },
		"complete": func(s *Service, a access.Actor) error { _, err := s.Complete(ctx, a, "e"); return err },
		"create group": func(s *Service, a access.Actor) error {
			_, err := s.CreateGroup(ctx, a, "e", domain.GroupFields{Code: "G", Name: "G", Role: "HEALTHY_CONTROL"})
			return err
		},
		"update group": func(s *Service, a access.Actor) error {
			_, err := s.UpdateGroup(ctx, a, "e", "g", GroupPatch{Name: &text})
			return err
		},
		"delete group": func(s *Service, a access.Actor) error { return s.DeleteGroup(ctx, a, "e", "g") },
		"create regimen item": func(s *Service, a access.Actor) error {
			_, err := s.CreateRegimenItem(ctx, a, "e", domain.RegimenFields{})
			return err
		},
		"update regimen item": func(s *Service, a access.Actor) error {
			_, err := s.UpdateRegimenItem(ctx, a, "e", "r", RegimenPatch{Amount: &text})
			return err
		},
		"delete regimen item": func(s *Service, a access.Actor) error { return s.DeleteRegimenItem(ctx, a, "e", "r") },
		"create timepoint": func(s *Service, a access.Actor) error {
			_, err := s.CreateTimepoint(ctx, a, "e", domain.TimepointFields{Name: "T", Position: 1})
			return err
		},
		"update timepoint": func(s *Service, a access.Actor) error {
			_, err := s.UpdateTimepoint(ctx, a, "e", "t", TimepointPatch{Name: &text})
			return err
		},
		"delete timepoint": func(s *Service, a access.Actor) error { return s.DeleteTimepoint(ctx, a, "e", "t") },
		"create endpoint": func(s *Service, a access.Actor) error {
			_, err := s.CreateEndpoint(ctx, a, "e", domain.EndpointFields{})
			return err
		},
		"update endpoint": func(s *Service, a access.Actor) error {
			_, err := s.UpdateEndpoint(ctx, a, "e", "p", EndpointPatch{Kind: &text})
			return err
		},
		"delete endpoint": func(s *Service, a access.Actor) error { return s.DeleteEndpoint(ctx, a, "e", "p") },
		"allocate": func(s *Service, a access.Actor) error {
			_, err := s.Allocate(ctx, a, "e", AllocateInput{SubjectID: "s", GroupID: "g"})
			return err
		},
		"update allocation": func(s *Service, a access.Actor) error {
			_, err := s.UpdateAllocation(ctx, a, "e", "a", AllocationPatch{Cage: &text})
			return err
		},
		"remove allocation": func(s *Service, a access.Actor) error { return s.RemoveAllocation(ctx, a, "e", "a") },
		"exclude": func(s *Service, a access.Actor) error {
			_, err := s.Exclude(ctx, a, "e", "a", ExcludeInput{Reason: "DIED"})
			return err
		},
		"randomise": func(s *Service, a access.Actor) error {
			_, err := s.Randomise(ctx, a, "e", RandomiseInput{SubjectIDs: []string{"s"}})
			return err
		},
	}
	reads := map[string]func(*Service, access.Actor) error{
		"list experiments": func(s *Service, a access.Actor) error {
			_, err := s.ListExperiments(ctx, a, ExperimentQuery{})
			return err
		},
		"experiment":     func(s *Service, a access.Actor) error { _, err := s.Experiment(ctx, a, "e"); return err },
		"readiness":      func(s *Service, a access.Actor) error { _, err := s.Readiness(ctx, a, "e"); return err },
		"metric options": func(s *Service, a access.Actor) error { _, err := s.MetricOptions(ctx, a, "e"); return err },
		"groups":         func(s *Service, a access.Actor) error { _, err := s.Groups(ctx, a, "e"); return err },
		"regimen items":  func(s *Service, a access.Actor) error { _, err := s.RegimenItems(ctx, a, "e"); return err },
		"timepoints":     func(s *Service, a access.Actor) error { _, err := s.Timepoints(ctx, a, "e"); return err },
		"endpoints":      func(s *Service, a access.Actor) error { _, err := s.Endpoints(ctx, a, "e"); return err },
		"allocations": func(s *Service, a access.Actor) error {
			_, err := s.Allocations(ctx, a, "e", AllocationQuery{})
			return err
		},
	}
	for _, role := range append(access.Roles(), "ROOT", "") {
		actor := access.Actor{UserID: "u", Role: role}
		if role == "" {
			actor = access.Actor{}
		}
		check := func(name string, op func(*Service, access.Actor) error, permission access.Permission) {
			if role.Allows(permission) {
				return
			}
			m := &memory{}
			if err := op(service(m), actor); !errors.Is(err, access.ErrForbidden) && !errors.Is(err, access.ErrUnauthenticated) {
				t.Errorf("%q %s: err=%v", role, name, err)
			}
			if m.calls != 0 {
				t.Errorf("%q %s touched the store", role, name)
			}
		}
		for name, op := range writes {
			check(name, op, access.StudyWrite)
		}
		for name, op := range reads {
			check(name, op, access.Read)
		}
	}
}

func readyDesign() *memory {
	return &memory{
		experiment: domain.Experiment{ID: "e", Code: "E1", Title: "Study", Status: domain.Draft},
		groups: []GroupFill{
			{Group: domain.Group{ID: "healthy", Role: domain.HealthyControl, TargetSize: 2}},
			{Group: domain.Group{ID: "disease", Role: domain.DiseaseControl, DiseaseModelID: "mptp", TargetSize: 2}},
		},
		items: []domain.RegimenItem{
			{GroupID: "disease", Purpose: domain.Induction},
			{GroupID: "disease", Purpose: domain.Vehicle},
		},
		timepoints: []domain.Timepoint{{ID: "t"}},
		endpoints:  []domain.Endpoint{{ID: "p", Kind: domain.Primary, ParadigmKey: "MWM", MetricKey: "path_length_cm"}},
		paradigms:  []ParadigmVersion{{"MWM", 1}},
	}
}

func TestStartChecksTheRulesAndSetsDayZero(t *testing.T) {
	m := readyDesign()
	m.paradigms = nil
	_, err := service(m).Start(ctx, researcher, "e", "2026-10-20")
	var notReady *NotReadyError
	if !errors.As(err, &notReady) || !errors.Is(err, ErrNotReady) || len(notReady.Problems) != 1 || notReady.Problems[0].Rule != domain.RuleEndpointMetricMissing {
		t.Fatalf("start without protocols: %v %+v", err, notReady)
	}
	if m.experiment.Status != domain.Draft {
		t.Fatal("started despite unmet rules")
	}
	m.paradigms = []ParadigmVersion{{"MWM", 1}}
	if _, err := service(m).Start(ctx, researcher, "e", "20-10-2026"); !errors.Is(err, ErrInvalidDayZero) {
		t.Fatalf("bad date: %v", err)
	}
	e, err := service(m).Start(ctx, researcher, "e", "2026-10-20")
	if err != nil || e.Status != domain.Running || e.DayZero.Format(time.DateOnly) != "2026-10-20" {
		t.Fatalf("start: %+v %v", e, err)
	}
	if _, err := service(m).Start(ctx, researcher, "e", "2026-10-20"); !errors.Is(err, ErrNotDraft) {
		t.Fatalf("second start: %v", err)
	}
	if _, err := service(m).Start(ctx, researcher, "x", "2026-10-20"); !errors.Is(err, ErrExperimentNotFound) {
		t.Fatalf("unknown experiment: %v", err)
	}
}

func TestStartNeedsAHealthyControl(t *testing.T) {
	m := readyDesign()
	m.groups = m.groups[1:]
	_, err := service(m).Start(ctx, researcher, "e", "2026-10-20")
	var notReady *NotReadyError
	if !errors.As(err, &notReady) || len(notReady.Problems) != 1 || notReady.Problems[0].Rule != domain.RuleHealthyControlMissing {
		t.Fatalf("start without a healthy control: %v %+v", err, notReady)
	}
	r, err := service(m).Readiness(ctx, researcher, "e")
	if err != nil || len(r.Problems) != 1 || r.Problems[0].Rule != domain.RuleHealthyControlMissing {
		t.Fatalf("readiness: %+v %v", r, err)
	}
}

func TestRegimenPurposeMustSuitTheGroupRole(t *testing.T) {
	item := func(group, purpose string) domain.RegimenFields {
		return domain.RegimenFields{GroupID: group, Purpose: purpose, SubstanceID: "s", Amount: "1", Unit: "mg/kg", Route: "ORAL",
			Frequency: "ONCE", StartDay: 0, EndDay: 0}
	}
	tests := []struct {
		group, purpose string
		want           error
	}{
		{"healthy", "VEHICLE", nil},
		{"healthy", "INDUCTION", domain.ErrPurposeNotAllowed},
		{"healthy", "TREATMENT", domain.ErrPurposeNotAllowed},
		{"sham", "VEHICLE", nil},
		{"sham", "INDUCTION", domain.ErrPurposeNotAllowed},
		{"sham", "TREATMENT", domain.ErrPurposeNotAllowed},
		{"disease", "INDUCTION", nil},
		{"disease", "VEHICLE", nil},
		{"disease", "TREATMENT", domain.ErrPurposeNotAllowed},
		{"positive", "TREATMENT", nil},
		{"treated", "INDUCTION", nil},
		{"treated", "VEHICLE", nil},
		{"treated", "TREATMENT", nil},
		{"elsewhere", "VEHICLE", ErrGroupNotInExperiment},
	}
	for _, tt := range tests {
		t.Run(tt.group+"/"+tt.purpose, func(t *testing.T) {
			m := readyDesign()
			m.groups = append(m.groups,
				GroupFill{Group: domain.Group{ID: "sham", Role: domain.Sham, DiseaseModelID: "mptp"}},
				GroupFill{Group: domain.Group{ID: "positive", Role: domain.PositiveControl, DiseaseModelID: "mptp"}},
				GroupFill{Group: domain.Group{ID: "treated", Role: domain.Treatment, DiseaseModelID: "mptp"}})
			before := len(m.items)
			_, err := service(m).CreateRegimenItem(ctx, researcher, "e", item(tt.group, tt.purpose))
			if !errors.Is(err, tt.want) || (err == nil) != (len(m.items) == before+1) {
				t.Fatalf("create: err=%v want=%v items=%d", err, tt.want, len(m.items))
			}
		})
	}

	// Changing the purpose or moving the item to another group is checked too.
	m := readyDesign()
	m.items[1] = domain.RegimenItem{ID: "vehicle", GroupID: "disease", Purpose: domain.Vehicle, SubstanceID: "s", DoseMicro: 1_000_000,
		Unit: "mL/kg", Route: "ORAL", Frequency: domain.Once}
	induction, healthy := "INDUCTION", "healthy"
	if _, err := service(m).UpdateRegimenItem(ctx, researcher, "e", "vehicle", RegimenPatch{GroupID: &healthy}); err != nil {
		t.Fatalf("move a vehicle to the healthy control: %v", err)
	}
	m.items[1].GroupID = "healthy"
	if _, err := service(m).UpdateRegimenItem(ctx, researcher, "e", "vehicle", RegimenPatch{Purpose: &induction}); !errors.Is(err, domain.ErrPurposeNotAllowed) {
		t.Fatalf("induction for the healthy control: %v", err)
	}
}

func TestCompleteNeedsARunningExperimentWithoutOpenTests(t *testing.T) {
	m := readyDesign()
	if _, err := service(m).Complete(ctx, researcher, "e"); !errors.Is(err, ErrNotRunning) {
		t.Fatalf("complete a draft: %v", err)
	}
	m.experiment.Status, m.openTests = domain.Running, 1
	if _, err := service(m).Complete(ctx, researcher, "e"); !errors.Is(err, ErrOpenTests) {
		t.Fatalf("complete with open tests: %v", err)
	}
	m.openTests = 0
	if e, err := service(m).Complete(ctx, researcher, "e"); err != nil || e.Status != domain.Completed {
		t.Fatalf("complete: %+v %v", e, err)
	}
}

func TestDesignChangesOnlyInADraft(t *testing.T) {
	m := readyDesign()
	m.experiment.Status = domain.Running
	if _, err := service(m).CreateGroup(ctx, researcher, "e", domain.GroupFields{Code: "G9", Name: "Late", Role: "HEALTHY_CONTROL"}); !errors.Is(err, ErrDesignFrozen) {
		t.Fatalf("group in running study: %v", err)
	}
	blinded, title := true, "Renamed"
	if _, err := service(m).UpdateExperiment(ctx, researcher, "e", ExperimentPatch{Blinded: &blinded}); !errors.Is(err, ErrDesignFrozen) {
		t.Fatalf("blinding a running study: %v", err)
	}
	if e, err := service(m).UpdateExperiment(ctx, researcher, "e", ExperimentPatch{Title: &title}); err != nil || e.Title != title {
		t.Fatalf("renaming a running study: %+v %v", e, err)
	}
	m.experiment.Status = domain.Completed
	if _, err := service(m).UpdateExperiment(ctx, researcher, "e", ExperimentPatch{Title: &title}); !errors.Is(err, ErrExperimentCompleted) {
		t.Fatalf("renaming a completed study: %v", err)
	}
}

func TestEndpointMetricMustComeFromTheProtocols(t *testing.T) {
	m := readyDesign()
	m.endpoints = nil
	f := domain.EndpointFields{TimepointID: "t", ParadigmKey: "MWM", MetricKey: "freezing_s", Kind: "SECONDARY", Direction: "LOWER_IS_BETTER"}
	if _, err := service(m).CreateEndpoint(ctx, researcher, "e", f); !errors.Is(err, ErrMetricUnavailable) {
		t.Fatalf("unknown metric: %v", err)
	}
	f.ParadigmKey, f.MetricKey = "POLE", "path_length_cm"
	if _, err := service(m).CreateEndpoint(ctx, researcher, "e", f); !errors.Is(err, ErrMetricUnavailable) {
		t.Fatalf("paradigm without protocol step: %v", err)
	}
	f.ParadigmKey, f.MetricKey = "MWM", "escape_latency_s"
	if _, err := service(m).CreateEndpoint(ctx, researcher, "e", f); err != nil {
		t.Fatalf("available metric: %v", err)
	}
}

func TestRandomiseRespectsTargetsAndAllocatesAllOrNothing(t *testing.T) {
	m := readyDesign()
	m.groups[0].Allocated = 1
	m.groups = append(m.groups, GroupFill{Group: domain.Group{ID: "untargeted", Role: domain.HealthyControl}})
	if _, err := service(m).Randomise(ctx, researcher, "e", RandomiseInput{SubjectIDs: []string{"a", "b", "c", "d"}}); !errors.Is(err, ErrNotEnoughPlaces) {
		t.Fatalf("too many animals: %v", err)
	}
	if len(m.allocations) != 0 {
		t.Fatal("allocated part of the animals")
	}
	for _, in := range []RandomiseInput{{}, {SubjectIDs: []string{"a", "a"}}, {SubjectIDs: []string{""}}} {
		if _, err := service(m).Randomise(ctx, researcher, "e", in); !errors.Is(err, ErrInvalidSubjects) {
			t.Errorf("%v: %v", in, err)
		}
	}
	if _, err := service(m).Randomise(ctx, researcher, "e", RandomiseInput{SubjectIDs: []string{"a"}, GroupIDs: []string{"other"}}); !errors.Is(err, ErrGroupNotInExperiment) {
		t.Fatalf("foreign group: %v", err)
	}
	created, err := service(m).Randomise(ctx, researcher, "e", RandomiseInput{SubjectIDs: []string{"a", "b", "c"}, Cage: "C1"})
	if err != nil || len(created) != 3 {
		t.Fatalf("randomise: %v %v", created, err)
	}
	count := map[string]int{}
	for _, a := range created {
		count[a.GroupID]++
		if a.Cage != "C1" || a.CreatedBy != "u" || !a.AllocatedAt.Equal(now) {
			t.Errorf("allocation %+v", a)
		}
	}
	if count["healthy"] != 1 || count["disease"] != 2 {
		t.Fatalf("counts=%v", count)
	}
	m.experiment.Status = domain.Completed
	if _, err := service(m).Randomise(ctx, researcher, "e", RandomiseInput{SubjectIDs: []string{"z"}}); !errors.Is(err, ErrAllocationFrozen) {
		t.Fatalf("completed study: %v", err)
	}
}

func TestAllocationsKeepTheirGroupOnceRunning(t *testing.T) {
	m := readyDesign()
	m.allocations = []domain.Allocation{{ID: "a", ExperimentID: "e", GroupID: "healthy", AllocatedAt: now.Add(-time.Hour)}}
	other, cage := "disease", "C2"
	if a, err := service(m).UpdateAllocation(ctx, researcher, "e", "a", AllocationPatch{GroupID: &other}); err != nil || a.GroupID != other {
		t.Fatalf("move in draft: %+v %v", a, err)
	}
	m.experiment.Status = domain.Running
	back := "healthy"
	if _, err := service(m).UpdateAllocation(ctx, researcher, "e", "a", AllocationPatch{GroupID: &back}); !errors.Is(err, ErrAllocationFrozen) {
		t.Fatalf("move while running: %v", err)
	}
	if err := service(m).RemoveAllocation(ctx, researcher, "e", "a"); !errors.Is(err, ErrAllocationFrozen) {
		t.Fatalf("remove while running: %v", err)
	}
	if a, err := service(m).UpdateAllocation(ctx, researcher, "e", "a", AllocationPatch{GroupID: &other, Cage: &cage}); err != nil || a.Cage != cage {
		t.Fatalf("cage while running: %+v %v", a, err)
	}
	if a, err := service(m).Exclude(ctx, researcher, "e", "a", ExcludeInput{Reason: "HUMANE_ENDPOINT"}); err != nil || !a.Excluded() {
		t.Fatalf("exclude: %+v %v", a, err)
	}
	if _, err := service(m).Exclude(ctx, researcher, "e", "a", ExcludeInput{Reason: "DIED"}); !errors.Is(err, domain.ErrAlreadyExcluded) {
		t.Fatalf("second exclusion: %v", err)
	}
	if _, err := service(m).UpdateAllocation(ctx, researcher, "e", "missing", AllocationPatch{Cage: &cage}); !errors.Is(err, ErrAllocationNotFound) {
		t.Fatalf("unknown allocation: %v", err)
	}
}

func TestListQueriesAreValidated(t *testing.T) {
	m := readyDesign()
	for _, q := range []ExperimentQuery{{Status: "PAUSED"}, {Sort: "status"}, {Order: "up"}, {Page: -1}} {
		if _, err := service(m).ListExperiments(ctx, researcher, q); !errors.Is(err, ErrInvalidQuery) {
			t.Errorf("%+v: %v", q, err)
		}
	}
	if _, err := service(m).Allocations(ctx, researcher, "e", AllocationQuery{Excluded: "maybe"}); !errors.Is(err, ErrInvalidQuery) {
		t.Errorf("excluded filter: %v", err)
	}
	options, err := service(m).MetricOptions(ctx, researcher, "e")
	if err != nil || len(options) != 1 || !slices.Contains(options[0].Metrics, "path_length_cm") {
		t.Fatalf("metric options: %v %v", options, err)
	}
}
