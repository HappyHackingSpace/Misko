package application

import (
	"context"
	"errors"
	access "github.com/HappyHackingSpace/Misko/backend/internal/access/domain"
	"github.com/HappyHackingSpace/Misko/backend/internal/laboratory/domain"
	"testing"
)

func TestReadIsOpenToEveryRoleAndUpdateNeedsLabConfigure(t *testing.T) {
	name := "Renamed Lab"
	for _, role := range append(access.Roles(), access.Role("ROOT")) {
		t.Run(string(role), func(t *testing.T) {
			s, store := initialized(t)
			actor := access.Actor{UserID: "u", Role: role}
			_, readErr := s.Laboratory(context.Background(), actor)
			_, updateErr := s.Update(context.Background(), actor, Update{Name: &name})
			known := role != "ROOT"
			if known != (readErr == nil) {
				t.Errorf("read err=%v", readErr)
			}
			if role.Privileged() {
				if updateErr != nil || store.lab.Name != name {
					t.Fatalf("privileged update: %v %+v", updateErr, store.lab)
				}
				return
			}
			if !errors.Is(updateErr, access.ErrForbidden) || store.lab.Name != "Lab" {
				t.Fatalf("unprivileged update: %v %+v", updateErr, store.lab)
			}
		})
	}
	s, _ := initialized(t)
	if _, err := s.Laboratory(context.Background(), access.Actor{}); !errors.Is(err, access.ErrUnauthenticated) {
		t.Fatalf("anonymous read: %v", err)
	}
}

func TestUpdateValidatesOnlyProvidedFields(t *testing.T) {
	s, store := initialized(t)
	admin := access.Actor{UserID: "admin", Role: access.LabManager}
	blank, bad, code := " ", "Mars/Base", " HHS-2 "
	for _, tc := range []struct {
		input Update
		want  error
	}{
		{Update{}, ErrNoChanges},
		{Update{Name: &blank}, domain.ErrInvalidName},
		{Update{Timezone: &bad}, domain.ErrInvalidTimezone},
		{Update{Code: &blank, Timezone: &bad}, domain.ErrInvalidTimezone},
	} {
		if _, err := s.Update(context.Background(), admin, tc.input); !errors.Is(err, tc.want) {
			t.Errorf("%+v: err=%v want %v", tc.input, err, tc.want)
		}
	}
	lab, err := s.Update(context.Background(), admin, Update{Code: &code})
	if err != nil || lab.Code != "HHS-2" || lab.Name != "Lab" || lab.Timezone != "UTC" {
		t.Fatalf("partial update: %+v %v", lab, err)
	}
	empty := ""
	if lab, err := s.Update(context.Background(), admin, Update{Code: &empty}); err != nil || lab.Code != "" || store.lab.Code != "" {
		t.Fatalf("clearing code: %+v %v", lab, err)
	}
}

func TestInitializeIsIdempotent(t *testing.T) {
	s := New(&memoryStore{})
	if _, err := s.PublicName(context.Background()); !errors.Is(err, ErrNotInitialized) {
		t.Fatalf("uninitialized name: %v", err)
	}
	if _, err := s.Initialize(context.Background(), " ", "", "UTC"); !errors.Is(err, domain.ErrInvalidName) {
		t.Fatalf("invalid initialization: %v", err)
	}
	created, err := s.Initialize(context.Background(), "First Lab", "", "Europe/Istanbul")
	if err != nil || !created {
		t.Fatalf("first: %v %v", created, err)
	}
	created, err = s.Initialize(context.Background(), "Second Lab", "", "UTC")
	if err != nil || created {
		t.Fatalf("second initialization created=%v err=%v", created, err)
	}
	if name, err := s.PublicName(context.Background()); err != nil || name != "First Lab" {
		t.Fatalf("singleton replaced: %q %v", name, err)
	}
}

func initialized(t *testing.T) (*Service, *memoryStore) {
	t.Helper()
	store := &memoryStore{}
	s := New(store)
	if _, err := s.Initialize(context.Background(), "Lab", "", "UTC"); err != nil {
		t.Fatal(err)
	}
	return s, store
}

type memoryStore struct{ lab *domain.Laboratory }

func (m *memoryStore) Laboratory(context.Context) (domain.Laboratory, error) {
	if m.lab == nil {
		return domain.Laboratory{}, ErrNotInitialized
	}
	return *m.lab, nil
}

func (m *memoryStore) Create(_ context.Context, lab domain.Laboratory) (bool, error) {
	if m.lab != nil {
		return false, nil
	}
	m.lab = &lab
	return true, nil
}

func (m *memoryStore) Update(_ context.Context, c Change) (domain.Laboratory, error) {
	if m.lab == nil {
		return domain.Laboratory{}, ErrNotInitialized
	}
	if c.Name != nil {
		m.lab.Name = *c.Name
	}
	if c.Code != nil {
		m.lab.Code = *c.Code
	}
	if c.Timezone != nil {
		m.lab.Timezone = *c.Timezone
	}
	return *m.lab, nil
}
