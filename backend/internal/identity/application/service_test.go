package application

import (
	"context"
	"errors"
	"fmt"
	access "github.com/HappyHackingSpace/Misko/backend/internal/access/domain"
	"github.com/HappyHackingSpace/Misko/backend/internal/identity/domain"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestUserManagementRequiresUserManageForEveryRole(t *testing.T) {
	password := "Correct-Horse-9"
	operations := map[string]func(*Service, access.Actor, string) error{
		"list": func(s *Service, a access.Actor, _ string) error {
			_, err := s.ListUsers(ctx(), a, UserQuery{})
			return err
		},
		"get": func(s *Service, a access.Actor, id string) error { _, err := s.User(ctx(), a, id); return err },
		"create": func(s *Service, a access.Actor, _ string) error {
			_, err := s.CreateUser(ctx(), a, CreateUser{Email: "new@lab.io", Name: "New", Role: "VIEWER"})
			return err
		},
		"update": func(s *Service, a access.Actor, id string) error {
			name := "Renamed"
			_, err := s.UpdateUser(ctx(), a, id, UpdateUser{Name: &name})
			return err
		},
		"reset": func(s *Service, a access.Actor, id string) error {
			_, err := s.ResetPassword(ctx(), a, id, &password)
			return err
		},
		"delete": func(s *Service, a access.Actor, id string) error { return s.DeleteUser(ctx(), a, id) },
	}
	for _, role := range append(access.Roles(), access.Role("ROOT")) {
		for name, operation := range operations {
			t.Run(string(role)+"/"+name, func(t *testing.T) {
				s, store := newService(t)
				store.seed(t, "owner@lab.io", access.SuperAdmin)
				store.seed(t, "second@lab.io", access.LabManager)
				target := store.seed(t, "target@lab.io", access.Researcher)
				actor := access.Actor{UserID: store.seed(t, "actor@lab.io", role).ID, Role: role}
				before := store.snapshot()
				err := operation(s, actor, target.ID)
				if role.Privileged() {
					if err != nil {
						t.Fatalf("privileged role denied: %v", err)
					}
					return
				}
				if !errors.Is(err, access.ErrForbidden) {
					t.Fatalf("err=%v want forbidden", err)
				}
				if store.snapshot() != before {
					t.Fatal("denied operation changed state")
				}
			})
		}
	}
	s, store := newService(t)
	target := store.seed(t, "target@lab.io", access.Researcher)
	for name, operation := range operations {
		if err := operation(s, access.Actor{}, target.ID); !errors.Is(err, access.ErrUnauthenticated) {
			t.Errorf("anonymous %s: err=%v", name, err)
		}
	}
}

func TestAuthenticationLoadsCurrentRoleAndRejectsDeletedUsers(t *testing.T) {
	s, store := newService(t)
	admin := actorFor(store.seed(t, "admin@lab.io", access.SuperAdmin))
	created, err := s.CreateUser(ctx(), admin, CreateUser{Email: "Researcher@Lab.io", Name: "R", Role: "RESEARCHER"})
	if err != nil || created.GeneratedPassword == "" || created.User.Email != "researcher@lab.io" {
		t.Fatalf("create: %+v %v", created, err)
	}
	session, err := s.Login(ctx(), " RESEARCHER@lab.io", created.GeneratedPassword)
	if err != nil {
		t.Fatal(err)
	}
	principal, err := s.Authenticate(ctx(), session.Token.Value)
	if err != nil || principal.Actor.Role != access.Researcher || principal.User.ID != created.User.ID {
		t.Fatalf("authenticate: %+v %v", principal, err)
	}
	viewer := "VIEWER"
	if _, err := s.UpdateUser(ctx(), admin, created.User.ID, UpdateUser{Role: &viewer}); err != nil {
		t.Fatal(err)
	}
	principal, err = s.Authenticate(ctx(), session.Token.Value)
	if err != nil || principal.Actor.Role != access.Viewer {
		t.Fatalf("role change not effective on next request: %+v %v", principal, err)
	}
	if err := s.DeleteUser(ctx(), admin, created.User.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Authenticate(ctx(), session.Token.Value); !errors.Is(err, access.ErrUnauthenticated) {
		t.Fatalf("deleted user authenticated: %v", err)
	}
	for _, raw := range []string{"", "garbage", "tok:unknown:1"} {
		if _, err := s.Authenticate(ctx(), raw); !errors.Is(err, access.ErrUnauthenticated) {
			t.Errorf("token %q: err=%v", raw, err)
		}
	}
}

func TestLoginFailuresAreIndistinguishable(t *testing.T) {
	s, store := newService(t)
	store.seedWithPassword(t, "user@lab.io", access.Viewer, "Right-Password-1")
	for _, tc := range []struct{ email, password string }{
		{"user@lab.io", "Wrong-Password-1"},
		{"missing@lab.io", "Right-Password-1"},
		{"not-an-email", "Right-Password-1"},
		{"user@lab.io", "Right-Password-1" + strings.Repeat("x", 80)},
		{"user@lab.io", ""},
	} {
		comparisons := store.passwords.comparisons
		if _, err := s.Login(ctx(), tc.email, tc.password); !errors.Is(err, ErrInvalidCredentials) {
			t.Errorf("%q: err=%v", tc.email, err)
		}
		if store.passwords.comparisons == comparisons {
			t.Errorf("%q: no password comparison performed; timing reveals account existence", tc.email)
		}
	}
}

func TestPasswordResetAndChangeRevokeExistingTokens(t *testing.T) {
	s, store := newService(t)
	admin := actorFor(store.seed(t, "admin@lab.io", access.SuperAdmin))
	user := store.seedWithPassword(t, "user@lab.io", access.Technician, "Original-Pass-1")
	old, err := s.Login(ctx(), "user@lab.io", "Original-Pass-1")
	if err != nil {
		t.Fatal(err)
	}
	generated, err := s.ResetPassword(ctx(), admin, user.ID, nil)
	if err != nil || generated == "" {
		t.Fatalf("reset: %q %v", generated, err)
	}
	if _, err := s.Authenticate(ctx(), old.Token.Value); !errors.Is(err, access.ErrUnauthenticated) {
		t.Fatalf("token survived reset: %v", err)
	}
	current, err := s.Login(ctx(), "user@lab.io", generated)
	if err != nil {
		t.Fatal(err)
	}
	actor := actorFor(user)
	if _, err := s.ChangePassword(ctx(), actor, "wrong-current", "Next-Password-1"); !errors.Is(err, ErrCurrentPasswordMismatch) {
		t.Fatalf("wrong current password: %v", err)
	}
	if _, err := s.ChangePassword(ctx(), actor, generated, "short"); !errors.Is(err, domain.ErrPasswordTooShort) {
		t.Fatalf("weak new password: %v", err)
	}
	next, err := s.ChangePassword(ctx(), actor, generated, "Next-Password-1")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Authenticate(ctx(), current.Token.Value); !errors.Is(err, access.ErrUnauthenticated) {
		t.Fatalf("token survived password change: %v", err)
	}
	if _, err := s.Authenticate(ctx(), next.Token.Value); err != nil {
		t.Fatalf("replacement token rejected: %v", err)
	}
	if _, err := s.ChangePassword(ctx(), access.Actor{}, "Next-Password-1", "Another-Pass-1"); !errors.Is(err, access.ErrUnauthenticated) {
		t.Fatalf("anonymous change: %v", err)
	}
	supplied := "Chosen-By-Admin-1"
	if generated, err := s.ResetPassword(ctx(), admin, user.ID, &supplied); err != nil || generated != "" {
		t.Fatalf("supplied reset returned %q %v", generated, err)
	}
}

func TestDeletionAndDemotionGuards(t *testing.T) {
	s, store := newService(t)
	admin := store.seed(t, "admin@lab.io", access.SuperAdmin)
	actor := actorFor(admin)
	if err := s.DeleteUser(ctx(), actor, admin.ID); !errors.Is(err, domain.ErrSelfDeletion) {
		t.Fatalf("self deletion: %v", err)
	}
	researcher := "RESEARCHER"
	if _, err := s.UpdateUser(ctx(), actor, admin.ID, UpdateUser{Role: &researcher}); !errors.Is(err, domain.ErrLastPrivileged) {
		t.Fatalf("last admin demoted: %v", err)
	}
	manager := store.seed(t, "manager@lab.io", access.LabManager)
	if err := s.DeleteUser(ctx(), actorFor(manager), admin.ID); err != nil {
		t.Fatalf("one of two privileged: %v", err)
	}
	if _, err := s.UpdateUser(ctx(), actorFor(manager), manager.ID, UpdateUser{Role: &researcher}); !errors.Is(err, domain.ErrLastPrivileged) {
		t.Fatalf("remaining manager demoted: %v", err)
	}
	if err := s.DeleteUser(ctx(), actorFor(manager), "missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing user: %v", err)
	}
	if _, err := s.UpdateUser(ctx(), actorFor(manager), manager.ID, UpdateUser{}); !errors.Is(err, ErrNoChanges) {
		t.Fatalf("empty update: %v", err)
	}
}

func TestCreateUserValidation(t *testing.T) {
	s, store := newService(t)
	admin := actorFor(store.seed(t, "admin@lab.io", access.SuperAdmin))
	short := "short"
	chosen := "Chosen-Pass-1"
	for _, tc := range []struct {
		name  string
		input CreateUser
		want  error
	}{
		{"duplicate email ignoring case", CreateUser{Email: "ADMIN@lab.io", Name: "A", Role: "VIEWER"}, ErrEmailTaken},
		{"unknown role", CreateUser{Email: "a@lab.io", Name: "A", Role: "ADMIN"}, access.ErrUnknownRole},
		{"missing role", CreateUser{Email: "a@lab.io", Name: "A"}, access.ErrUnknownRole},
		{"invalid email", CreateUser{Email: "a", Name: "A", Role: "VIEWER"}, domain.ErrInvalidEmail},
		{"invalid name", CreateUser{Email: "a@lab.io", Name: " ", Role: "VIEWER"}, domain.ErrInvalidName},
		{"weak password", CreateUser{Email: "a@lab.io", Name: "A", Role: "VIEWER", Password: &short}, domain.ErrPasswordTooShort},
	} {
		if _, err := s.CreateUser(ctx(), admin, tc.input); !errors.Is(err, tc.want) {
			t.Errorf("%s: err=%v want %v", tc.name, err, tc.want)
		}
	}
	created, err := s.CreateUser(ctx(), admin, CreateUser{Email: "b@lab.io", Name: "B", Role: "TECHNICIAN", Password: &chosen})
	if err != nil || created.GeneratedPassword != "" {
		t.Fatalf("supplied password echoed or rejected: %+v %v", created, err)
	}
	if _, err := s.Login(ctx(), "b@lab.io", chosen); err != nil {
		t.Fatal(err)
	}
}

func TestListUsersAllowlistsQuery(t *testing.T) {
	s, store := newService(t)
	admin := actorFor(store.seed(t, "admin@lab.io", access.SuperAdmin))
	for _, query := range []UserQuery{
		{Sort: "password_hash"}, {Order: "sideways"}, {Role: "ADMIN"}, {Search: strings.Repeat("a", 101)}, {Page: -1}, {PageSize: -5},
	} {
		if _, err := s.ListUsers(ctx(), admin, query); !errors.Is(err, ErrInvalidQuery) {
			t.Errorf("%+v: err=%v", query, err)
		}
	}
	page, err := s.ListUsers(ctx(), admin, UserQuery{Search: "  adm ", Role: "SUPERADMIN", Sort: "email", PageSize: 500})
	if err != nil {
		t.Fatal(err)
	}
	want := UserFilter{Search: "adm", Role: access.SuperAdmin, Sort: "email", Limit: 100}
	if store.lastFilter != want || page.Page != 1 || page.PageSize != 100 || page.Total != 1 {
		t.Fatalf("filter=%+v page=%+v", store.lastFilter, page)
	}
	if _, err := s.ListUsers(ctx(), admin, UserQuery{Page: 3, PageSize: 20, Order: "asc"}); err != nil {
		t.Fatal(err)
	}
	if want := (UserFilter{Sort: "createdAt", Limit: 20, Offset: 40}); store.lastFilter != want {
		t.Fatalf("filter=%+v want %+v", store.lastFilter, want)
	}
	if _, err := s.ListUsers(ctx(), admin, UserQuery{}); err != nil || !store.lastFilter.Descending || store.lastFilter.Sort != "createdAt" {
		t.Fatalf("default sort: %+v %v", store.lastFilter, err)
	}
}

func TestBootstrapAdministratorCreatesOnlyOnce(t *testing.T) {
	s, store := newService(t)
	created, password, err := s.BootstrapAdministrator(ctx(), "Admin@Lab.io")
	if err != nil || !created || password == "" {
		t.Fatalf("first bootstrap: %v %q %v", created, password, err)
	}
	session, err := s.Login(ctx(), "admin@lab.io", password)
	if err != nil || session.User.Role != access.SuperAdmin {
		t.Fatalf("bootstrap admin login: %+v %v", session, err)
	}
	for _, email := range []string{"admin@lab.io", "other@lab.io"} {
		created, password, err := s.BootstrapAdministrator(ctx(), email)
		if err != nil || created || password != "" {
			t.Fatalf("repeat bootstrap with %s: %v %q %v", email, created, password, err)
		}
	}
	if n := store.snapshot(); !strings.HasPrefix(n, "1:") {
		t.Fatalf("repeated bootstrap created users: %s", n)
	}
	if _, _, err := s.BootstrapAdministrator(ctx(), "invalid"); !errors.Is(err, domain.ErrInvalidEmail) {
		t.Fatalf("invalid email: %v", err)
	}
}

func ctx() context.Context { return context.Background() }

func actorFor(u domain.User) access.Actor { return access.Actor{UserID: u.ID, Role: u.Role} }

func newService(t *testing.T) (*Service, *memoryStore) {
	t.Helper()
	store := &memoryStore{records: map[string]*record{}, passwords: &fakePasswords{}}
	return New(store, fakeTokens{}, store.passwords), store
}

type record struct {
	user    domain.User
	hash    string
	version int
}

// memoryStore is a behavioral fake: Serializable holds one lock for the whole
// callback. Real concurrency is verified against PostgreSQL in the adapter tests.
type memoryStore struct {
	mu         sync.Mutex
	inTx       bool
	records    map[string]*record
	next       int
	passwords  *fakePasswords
	lastFilter UserFilter
}

func (m *memoryStore) Serializable(_ context.Context, fn func(Store) error) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.inTx = true
	defer func() { m.inTx = false }()
	return fn(m)
}

func (m *memoryStore) lock() func() {
	if m.inTx {
		return func() {}
	}
	m.mu.Lock()
	return m.mu.Unlock
}

func (m *memoryStore) CreateUser(_ context.Context, u NewUser) (domain.User, error) {
	defer m.lock()()
	for _, r := range m.records {
		if r.user.Email == u.Email {
			return domain.User{}, ErrEmailTaken
		}
	}
	m.next++
	user := domain.User{ID: fmt.Sprintf("u%d", m.next), Email: u.Email, Name: u.Name, Role: u.Role, CreatedAt: time.Unix(int64(m.next), 0)}
	m.records[user.ID] = &record{user: user, hash: u.PasswordHash, version: 1}
	return user, nil
}

func (m *memoryStore) User(ctx context.Context, id string) (domain.User, error) {
	a, err := m.AccountByID(ctx, id)
	return a.User, err
}

func (m *memoryStore) AccountByEmail(_ context.Context, email string) (Account, error) {
	defer m.lock()()
	for _, r := range m.records {
		if r.user.Email == email {
			return Account{User: r.user, PasswordHash: r.hash, SessionVersion: r.version}, nil
		}
	}
	return Account{}, ErrNotFound
}

func (m *memoryStore) AccountByID(_ context.Context, id string) (Account, error) {
	defer m.lock()()
	r, ok := m.records[id]
	if !ok {
		return Account{}, ErrNotFound
	}
	return Account{User: r.user, PasswordHash: r.hash, SessionVersion: r.version}, nil
}

func (m *memoryStore) ListUsers(_ context.Context, f UserFilter) ([]domain.User, int, error) {
	defer m.lock()()
	m.lastFilter = f
	var users []domain.User
	for _, r := range m.records {
		if (f.Role == "" || r.user.Role == f.Role) && strings.Contains(r.user.Email, f.Search) {
			users = append(users, r.user)
		}
	}
	return users, len(users), nil
}

func (m *memoryStore) UpdateUser(_ context.Context, id string, c UserChange) (domain.User, error) {
	defer m.lock()()
	r, ok := m.records[id]
	if !ok {
		return domain.User{}, ErrNotFound
	}
	if c.Name != nil {
		r.user.Name = *c.Name
	}
	if c.Role != nil {
		r.user.Role = *c.Role
	}
	return r.user, nil
}

func (m *memoryStore) SetPasswordHash(_ context.Context, id, hash string) (int, error) {
	defer m.lock()()
	r, ok := m.records[id]
	if !ok {
		return 0, ErrNotFound
	}
	r.hash, r.version = hash, r.version+1
	return r.version, nil
}

func (m *memoryStore) DeleteUser(_ context.Context, id string) error {
	defer m.lock()()
	if _, ok := m.records[id]; !ok {
		return ErrNotFound
	}
	delete(m.records, id)
	return nil
}

func (m *memoryStore) CountPrivileged(context.Context) (int, error) {
	defer m.lock()()
	n := 0
	for _, r := range m.records {
		if r.user.Role.Privileged() {
			n++
		}
	}
	return n, nil
}

func (m *memoryStore) seed(t *testing.T, email string, role access.Role) domain.User {
	return m.seedWithPassword(t, email, role, "Seeded-Password-1")
}

func (m *memoryStore) seedWithPassword(t *testing.T, email string, role access.Role, password string) domain.User {
	t.Helper()
	hash, _ := m.passwords.Hash(password)
	u, err := m.CreateUser(ctx(), NewUser{Email: email, Name: email, Role: role, PasswordHash: hash})
	if err != nil {
		t.Fatal(err)
	}
	return u
}

func (m *memoryStore) snapshot() string {
	defer m.lock()()
	var rows []string
	for _, r := range m.records {
		rows = append(rows, fmt.Sprintf("%+v/%s/%d", r.user, r.hash, r.version))
	}
	slices.Sort(rows)
	return fmt.Sprintf("%d:%s", len(rows), strings.Join(rows, ","))
}

type fakePasswords struct {
	mu          sync.Mutex
	comparisons int
	generated   int
}

func (p *fakePasswords) Hash(password string) (string, error) { return "hash:" + password, nil }

func (p *fakePasswords) Matches(hash, password string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.comparisons++
	return hash != "" && len(password) <= domain.MaxPasswordBytes && hash == "hash:"+password
}

func (p *fakePasswords) Generate() (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.generated++
	return fmt.Sprintf("Generated-Password-%d", p.generated), nil
}

type fakeTokens struct{}

func (fakeTokens) Issue(c Claims) (Token, error) {
	return Token{Value: fmt.Sprintf("tok:%s:%d", c.UserID, c.SessionVersion), ExpiresAt: time.Unix(1, 0)}, nil
}

func (fakeTokens) Verify(raw string) (Claims, error) {
	var c Claims
	parts := strings.Split(raw, ":")
	if len(parts) != 3 || parts[0] != "tok" {
		return Claims{}, errors.New("malformed")
	}
	c.UserID = parts[1]
	if _, err := fmt.Sscan(parts[2], &c.SessionVersion); err != nil {
		return Claims{}, err
	}
	return c, nil
}
