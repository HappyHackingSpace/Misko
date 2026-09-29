//go:build integration

package postgres_test

import (
	"context"
	"errors"
	access "github.com/HappyHackingSpace/Misko/backend/internal/access/domain"
	"github.com/HappyHackingSpace/Misko/backend/internal/identity/adapters/password"
	"github.com/HappyHackingSpace/Misko/backend/internal/identity/adapters/postgres"
	"github.com/HappyHackingSpace/Misko/backend/internal/identity/adapters/token"
	"github.com/HappyHackingSpace/Misko/backend/internal/identity/application"
	"github.com/HappyHackingSpace/Misko/backend/internal/identity/domain"
	"github.com/HappyHackingSpace/Misko/backend/internal/platform/pgtest"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

var ctx = context.Background()

func newService(t *testing.T, pool *pgxpool.Pool) *application.Service {
	t.Helper()
	passwords, err := password.NewBcrypt(bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	tokens, err := token.NewJWT([]byte(strings.Repeat("k", 32)), "misko", "misko-api", time.Hour, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	return application.New(postgres.NewStore(pool), tokens, passwords)
}

// The SUPERADMIN account is permanent: bootstrap creates it once and nothing
// else can ever demote, delete or reassign it. Because that guard reads the
// target's role rather than racing a shared count, every concurrent attempt
// against it must lose, not just one of them — this proves that holds under
// real database concurrency, not only in a single-threaded call.
func TestConcurrentSuperAdminIsProtectedUnderLoad(t *testing.T) {
	pool := pgtest.Database(t)
	s := newService(t, pool)
	researcher := "RESEARCHER"
	for iteration := range 5 {
		if _, err := pool.Exec(ctx, "DELETE FROM misko.users"); err != nil {
			t.Fatal(err)
		}
		_, adminPassword, err := s.BootstrapAdministrator(ctx, "admin@lab.io")
		if err != nil {
			t.Fatal(err)
		}
		admin, err := s.Login(ctx, "admin@lab.io", adminPassword)
		if err != nil {
			t.Fatal(err)
		}
		adminActor := access.Actor{UserID: admin.User.ID, Role: access.SuperAdmin}
		manager, err := s.CreateUser(ctx, adminActor, application.CreateUser{Email: "manager@lab.io", Name: "Manager", Role: "LAB_MANAGER"})
		if err != nil {
			t.Fatal(err)
		}
		managerActor := access.Actor{UserID: manager.User.ID, Role: access.LabManager}
		resetPassword := "Correct-Horse-9"
		operations := []func() error{
			func() error { return s.DeleteUser(ctx, managerActor, admin.User.ID) },
			func() error {
				_, err := s.UpdateUser(ctx, managerActor, admin.User.ID, application.UpdateUser{Role: &researcher})
				return err
			},
			func() error {
				_, err := s.ResetPassword(ctx, managerActor, admin.User.ID, &resetPassword)
				return err
			},
		}
		errs := make([]error, len(operations))
		var wg sync.WaitGroup
		start := make(chan struct{})
		for i, op := range operations {
			wg.Go(func() {
				<-start
				errs[i] = op()
			})
		}
		close(start)
		wg.Wait()
		for i, err := range errs {
			if !errors.Is(err, domain.ErrSuperAdminProtected) {
				t.Fatalf("iteration %d op %d: err=%v want %v", iteration, i, err, domain.ErrSuperAdminProtected)
			}
		}
		var role string
		if err := pool.QueryRow(ctx, "SELECT role FROM misko.users WHERE id = $1", admin.User.ID).Scan(&role); err != nil {
			t.Fatal(err)
		}
		if role != string(access.SuperAdmin) {
			t.Fatalf("iteration %d: admin role changed to %s", iteration, role)
		}
	}
}

func TestConcurrentBootstrapCreatesOneAdministrator(t *testing.T) {
	pool := pgtest.Database(t)
	s := newService(t, pool)
	var mu sync.Mutex
	var wg sync.WaitGroup
	created := 0
	start := make(chan struct{})
	for range 8 {
		wg.Go(func() {
			<-start
			ok, _, err := s.BootstrapAdministrator(ctx, "admin@lab.io")
			if err != nil {
				t.Error(err)
			}
			mu.Lock()
			defer mu.Unlock()
			if ok {
				created++
			}
		})
	}
	close(start)
	wg.Wait()
	var users int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM misko.users").Scan(&users); err != nil {
		t.Fatal(err)
	}
	if created != 1 || users != 1 {
		t.Fatalf("created=%d users=%d", created, users)
	}
}

func TestStoreTranslatesMissingRowsAndConstraints(t *testing.T) {
	pool := pgtest.Database(t)
	store := postgres.NewStore(pool)
	user, err := store.CreateUser(ctx, application.NewUser{Email: "a@lab.io", Name: "Alpha", Role: access.Researcher, PasswordHash: "h1"})
	if err != nil || user.ID == "" || user.CreatedAt.IsZero() || user.Role != access.Researcher {
		t.Fatalf("create: %+v %v", user, err)
	}
	if _, err := store.CreateUser(ctx, application.NewUser{Email: "a@lab.io", Name: "Other", Role: access.Viewer, PasswordHash: "h"}); !errors.Is(err, application.ErrEmailTaken) {
		t.Fatalf("duplicate email: %v", err)
	}
	if _, err := store.CreateUser(ctx, application.NewUser{Email: "b@lab.io", Name: "B", Role: "ADMIN", PasswordHash: "h"}); err == nil || errors.Is(err, application.ErrEmailTaken) {
		t.Fatalf("unknown role stored: %v", err)
	}
	for _, id := range []string{"not-a-uuid", "00000000-0000-0000-0000-000000000000", ""} {
		if _, err := store.User(ctx, id); !errors.Is(err, application.ErrNotFound) {
			t.Errorf("User(%q): %v", id, err)
		}
		if _, err := store.AccountByID(ctx, id); !errors.Is(err, application.ErrNotFound) {
			t.Errorf("AccountByID(%q): %v", id, err)
		}
		name := "x"
		if _, err := store.UpdateUser(ctx, id, application.UserChange{Name: &name}); !errors.Is(err, application.ErrNotFound) {
			t.Errorf("UpdateUser(%q): %v", id, err)
		}
		if _, err := store.SetPasswordHash(ctx, id, "h"); !errors.Is(err, application.ErrNotFound) {
			t.Errorf("SetPasswordHash(%q): %v", id, err)
		}
		if err := store.DeleteUser(ctx, id); !errors.Is(err, application.ErrNotFound) {
			t.Errorf("DeleteUser(%q): %v", id, err)
		}
	}
	if _, err := store.AccountByEmail(ctx, "missing@lab.io"); !errors.Is(err, application.ErrNotFound) {
		t.Fatalf("missing email: %v", err)
	}
	if version, err := store.SetPasswordHash(ctx, user.ID, "h2"); err != nil || version != 2 {
		t.Fatalf("set password: %d %v", version, err)
	}
	account, err := store.AccountByEmail(ctx, "a@lab.io")
	if err != nil || account.PasswordHash != "h2" || account.SessionVersion != 2 || account.User != user {
		t.Fatalf("account: %+v %v", account, err)
	}
	name := "Renamed"
	if updated, err := store.UpdateUser(ctx, user.ID, application.UserChange{Name: &name}); err != nil || updated.Name != name || updated.Role != access.Researcher {
		t.Fatalf("partial update: %+v %v", updated, err)
	}
	if n, err := store.CountPrivileged(ctx); err != nil || n != 0 {
		t.Fatalf("privileged=%d %v", n, err)
	}
	for _, role := range []access.Role{access.SuperAdmin, access.LabManager} {
		if _, err := store.CreateUser(ctx, application.NewUser{Email: string(role) + "@lab.io", Name: "P", Role: role, PasswordHash: "h"}); err != nil {
			t.Fatal(err)
		}
	}
	if n, err := store.CountPrivileged(ctx); err != nil || n != 2 {
		t.Fatalf("privileged=%d %v", n, err)
	}
	if err := store.DeleteUser(ctx, user.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.User(ctx, user.ID); !errors.Is(err, application.ErrNotFound) {
		t.Fatalf("deleted user still readable: %v", err)
	}
}

func TestListUsersEscapesSearchAndPagesStably(t *testing.T) {
	pool := pgtest.Database(t)
	store := postgres.NewStore(pool)
	for _, u := range []struct {
		email, name string
		role        access.Role
	}{
		{"alice@lab.io", "Alice", access.Researcher},
		{"bob@lab.io", "Bob_Smith", access.Viewer},
		{"carol@lab.io", "Carol 100%", access.Researcher},
		{"dave@lab.io", "Dave", access.Researcher},
		{"same1@lab.io", "Same", access.Technician},
		{"same2@lab.io", "Same", access.Technician},
	} {
		if _, err := store.CreateUser(ctx, application.NewUser{Email: u.email, Name: u.name, Role: u.role, PasswordHash: "h"}); err != nil {
			t.Fatal(err)
		}
	}
	list := func(f application.UserFilter) ([]string, int) {
		t.Helper()
		users, total, err := store.ListUsers(ctx, f)
		if err != nil {
			t.Fatal(err)
		}
		var names []string
		for _, u := range users {
			names = append(names, u.Name)
		}
		return names, total
	}
	for _, tc := range []struct {
		filter application.UserFilter
		names  []string
		total  int
	}{
		{application.UserFilter{Search: "%", Sort: "name", Limit: 10}, []string{"Carol 100%"}, 1},
		{application.UserFilter{Search: "_", Sort: "name", Limit: 10}, []string{"Bob_Smith"}, 1},
		{application.UserFilter{Search: `\`, Sort: "name", Limit: 10}, nil, 0},
		{application.UserFilter{Search: "ALICE@LAB", Sort: "name", Limit: 10}, []string{"Alice"}, 1},
		{application.UserFilter{Role: access.Researcher, Sort: "name", Limit: 10}, []string{"Alice", "Carol 100%", "Dave"}, 3},
		{application.UserFilter{Role: access.Researcher, Sort: "name", Descending: true, Limit: 2}, []string{"Dave", "Carol 100%"}, 3},
		{application.UserFilter{Sort: "email", Limit: 2, Offset: 2}, []string{"Carol 100%", "Dave"}, 6},
		{application.UserFilter{Sort: "email", Limit: 2, Offset: 10}, nil, 6},
	} {
		names, total := list(tc.filter)
		if !slices.Equal(names, tc.names) || total != tc.total {
			t.Errorf("%+v: names=%v total=%d want %v %d", tc.filter, names, total, tc.names, tc.total)
		}
	}
	for _, descending := range []bool{false, true} {
		users, _, err := store.ListUsers(ctx, application.UserFilter{Role: access.Technician, Sort: "name", Descending: descending, Limit: 10})
		if err != nil || len(users) != 2 || users[0].ID > users[1].ID {
			t.Fatalf("equal names not ordered by id (descending=%v): %+v %v", descending, users, err)
		}
	}
}
