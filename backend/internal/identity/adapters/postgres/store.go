// Package postgres implements identity persistence with sqlc-generated queries.
package postgres

import (
	"context"
	"errors"
	"fmt"
	access "github.com/HappyHackingSpace/Misko/backend/internal/access/domain"
	"github.com/HappyHackingSpace/Misko/backend/internal/identity/adapters/postgres/sqlcgen"
	"github.com/HappyHackingSpace/Misko/backend/internal/identity/application"
	"github.com/HappyHackingSpace/Misko/backend/internal/identity/domain"
	"github.com/HappyHackingSpace/Misko/backend/internal/platform/pgtx"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"regexp"
	"strings"
	"time"
)

var (
	uuidPattern = regexp.MustCompile(`^(?i)[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
	likeEscaper = strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
)

type Store struct {
	pool    *pgxpool.Pool
	queries *sqlcgen.Queries
}

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool, queries: sqlcgen.New(pool)}
}

func (s *Store) Serializable(ctx context.Context, fn func(application.Store) error) error {
	return pgtx.Serializable(ctx, s.pool, func(tx pgx.Tx) error {
		return fn(&Store{pool: s.pool, queries: s.queries.WithTx(tx)})
	})
}

func (s *Store) CreateUser(ctx context.Context, u application.NewUser) (domain.User, error) {
	row, err := s.queries.CreateUser(ctx, sqlcgen.CreateUserParams{Email: u.Email, Name: u.Name, Role: string(u.Role), PasswordHash: u.PasswordHash})
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "users_email_key" {
			return domain.User{}, application.ErrEmailTaken
		}
		return domain.User{}, fmt.Errorf("create user: %w", err)
	}
	return toUser(row.ID, row.Email, row.Name, row.Role, row.CreatedAt)
}

func (s *Store) User(ctx context.Context, id string) (domain.User, error) {
	if !uuidPattern.MatchString(id) {
		return domain.User{}, application.ErrNotFound
	}
	row, err := s.queries.GetUser(ctx, id)
	if err != nil {
		return domain.User{}, missing(err, "get user")
	}
	return toUser(row.ID, row.Email, row.Name, row.Role, row.CreatedAt)
}

func (s *Store) AccountByEmail(ctx context.Context, email string) (application.Account, error) {
	row, err := s.queries.GetAccountByEmail(ctx, email)
	if err != nil {
		return application.Account{}, missing(err, "get account")
	}
	return toAccount(row.ID, row.Email, row.Name, row.Role, row.CreatedAt, row.PasswordHash, row.SessionVersion)
}

func (s *Store) AccountByID(ctx context.Context, id string) (application.Account, error) {
	if !uuidPattern.MatchString(id) {
		return application.Account{}, application.ErrNotFound
	}
	row, err := s.queries.GetAccountByID(ctx, id)
	if err != nil {
		return application.Account{}, missing(err, "get account")
	}
	return toAccount(row.ID, row.Email, row.Name, row.Role, row.CreatedAt, row.PasswordHash, row.SessionVersion)
}

func (s *Store) ListUsers(ctx context.Context, f application.UserFilter) ([]domain.User, int, error) {
	search := likeEscaper.Replace(f.Search)
	rows, err := s.queries.ListUsers(ctx, sqlcgen.ListUsersParams{
		Search: search, Role: string(f.Role), SortKey: f.Sort, Descending: f.Descending,
		PageLimit: int32(f.Limit), PageOffset: int32(f.Offset),
	})
	if err != nil {
		return nil, 0, fmt.Errorf("list users: %w", err)
	}
	total, err := s.queries.CountUsers(ctx, sqlcgen.CountUsersParams{Search: search, Role: string(f.Role)})
	if err != nil {
		return nil, 0, fmt.Errorf("count users: %w", err)
	}
	users := make([]domain.User, 0, len(rows))
	for _, row := range rows {
		user, err := toUser(row.ID, row.Email, row.Name, row.Role, row.CreatedAt)
		if err != nil {
			return nil, 0, err
		}
		users = append(users, user)
	}
	return users, int(total), nil
}

func (s *Store) UpdateUser(ctx context.Context, id string, c application.UserChange) (domain.User, error) {
	if !uuidPattern.MatchString(id) {
		return domain.User{}, application.ErrNotFound
	}
	params := sqlcgen.UpdateUserParams{ID: id, Name: c.Name}
	if c.Role != nil {
		role := string(*c.Role)
		params.Role = &role
	}
	row, err := s.queries.UpdateUser(ctx, params)
	if err != nil {
		return domain.User{}, missing(err, "update user")
	}
	return toUser(row.ID, row.Email, row.Name, row.Role, row.CreatedAt)
}

func (s *Store) SetPasswordHash(ctx context.Context, id, hash string) (int, error) {
	if !uuidPattern.MatchString(id) {
		return 0, application.ErrNotFound
	}
	version, err := s.queries.SetPasswordHash(ctx, sqlcgen.SetPasswordHashParams{ID: id, PasswordHash: hash})
	if err != nil {
		return 0, missing(err, "set password")
	}
	return int(version), nil
}

func (s *Store) DeleteUser(ctx context.Context, id string) error {
	if !uuidPattern.MatchString(id) {
		return application.ErrNotFound
	}
	deleted, err := s.queries.DeleteUser(ctx, id)
	if err != nil {
		return fmt.Errorf("delete user: %w", err)
	}
	if deleted == 0 {
		return application.ErrNotFound
	}
	return nil
}

func (s *Store) CountPrivileged(ctx context.Context) (int, error) {
	var roles []string
	for _, role := range access.Roles() {
		if role.Privileged() {
			roles = append(roles, string(role))
		}
	}
	count, err := s.queries.CountUsersWithRoles(ctx, roles)
	if err != nil {
		return 0, fmt.Errorf("count privileged users: %w", err)
	}
	return int(count), nil
}

// missing maps an absent row to ErrNotFound and wraps other errors, keeping
// PostgreSQL error codes visible to the serializable retry loop.
func missing(err error, operation string) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return application.ErrNotFound
	}
	return fmt.Errorf("%s: %w", operation, err)
}

func toUser(id, email, name, rawRole string, createdAt time.Time) (domain.User, error) {
	role, err := access.ParseRole(rawRole)
	if err != nil {
		return domain.User{}, fmt.Errorf("user %s: %w", id, err)
	}
	return domain.User{ID: id, Email: email, Name: name, Role: role, CreatedAt: createdAt.UTC()}, nil
}

func toAccount(id, email, name, role string, createdAt time.Time, hash string, version int32) (application.Account, error) {
	user, err := toUser(id, email, name, role, createdAt)
	if err != nil {
		return application.Account{}, err
	}
	return application.Account{User: user, PasswordHash: hash, SessionVersion: int(version)}, nil
}
