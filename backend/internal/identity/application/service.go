// Package application contains identity use cases: authentication, password
// management and user administration, each authorized by the RBAC kernel.
package application

import (
	"context"
	"errors"
	access "github.com/HappyHackingSpace/Misko/backend/internal/access/domain"
	"github.com/HappyHackingSpace/Misko/backend/internal/identity/domain"
	"slices"
	"strings"
	"time"
	"unicode/utf8"
)

var (
	ErrNotFound                = errors.New("user not found")
	ErrEmailTaken              = errors.New("email is already registered")
	ErrInvalidCredentials      = errors.New("invalid email or password")
	ErrCurrentPasswordMismatch = errors.New("current password is incorrect")
	ErrNoChanges               = errors.New("no fields to update")
	ErrInvalidQuery            = errors.New("invalid list query")
)

type Account struct {
	User           domain.User
	PasswordHash   string
	SessionVersion int
}

type NewUser struct {
	Email, Name  string
	Role         access.Role
	PasswordHash string
}

// UserChange holds validated fields; nil leaves a field unchanged.
type UserChange struct {
	Name *string
	Role *access.Role
}

// UserFilter is a validated list query whose sort key is allowlisted.
type UserFilter struct {
	Search        string
	Role          access.Role
	Sort          string
	Descending    bool
	Limit, Offset int
}

// Store persists users. Implementations return ErrNotFound for missing users and
// ErrEmailTaken for duplicate emails.
type Store interface {
	// Serializable runs fn in a serializable transaction and may re-run it after
	// a serialization failure, so fn must not cause external side effects.
	Serializable(ctx context.Context, fn func(Store) error) error
	CreateUser(ctx context.Context, user NewUser) (domain.User, error)
	User(ctx context.Context, id string) (domain.User, error)
	AccountByEmail(ctx context.Context, email string) (Account, error)
	AccountByID(ctx context.Context, id string) (Account, error)
	ListUsers(ctx context.Context, filter UserFilter) ([]domain.User, int, error)
	UpdateUser(ctx context.Context, id string, change UserChange) (domain.User, error)
	// SetPasswordHash stores a new hash and increments the session version so
	// previously issued tokens stop authenticating. It returns the new version.
	SetPasswordHash(ctx context.Context, id, hash string) (int, error)
	DeleteUser(ctx context.Context, id string) error
	CountPrivileged(ctx context.Context) (int, error)
}

// Claims deliberately carry no role: the current role is loaded on every request.
type Claims struct {
	UserID         string
	SessionVersion int
}

type Token struct {
	Value     string
	ExpiresAt time.Time
}

type Tokens interface {
	Issue(Claims) (Token, error)
	Verify(raw string) (Claims, error)
}

type Passwords interface {
	Hash(password string) (string, error)
	// Matches must spend comparable time for an empty hash and return false, so
	// unknown accounts cannot be detected by response timing.
	Matches(hash, password string) bool
	Generate() (string, error)
}

type Service struct {
	store     Store
	tokens    Tokens
	passwords Passwords
}

func New(store Store, tokens Tokens, passwords Passwords) *Service {
	return &Service{store: store, tokens: tokens, passwords: passwords}
}

type Session struct {
	Token Token
	User  domain.User
}

type Principal struct {
	Actor access.Actor
	User  domain.User
}

func (s *Service) Login(ctx context.Context, email, password string) (Session, error) {
	var account Account
	normalized, err := domain.NormalizeEmail(email)
	if err == nil {
		account, err = s.store.AccountByEmail(ctx, normalized)
	}
	if err != nil && !errors.Is(err, domain.ErrInvalidEmail) && !errors.Is(err, ErrNotFound) {
		return Session{}, err
	}
	// Compare even for unknown accounts (empty hash) so timing does not reveal them.
	if !s.passwords.Matches(account.PasswordHash, password) || err != nil {
		return Session{}, ErrInvalidCredentials
	}
	return s.session(account.User, account.SessionVersion)
}

// Authenticate verifies a bearer token and loads the user's current role.
// Deleted users and tokens issued before a password change are rejected.
func (s *Service) Authenticate(ctx context.Context, raw string) (Principal, error) {
	claims, err := s.tokens.Verify(raw)
	if err != nil {
		return Principal{}, access.ErrUnauthenticated
	}
	account, err := s.store.AccountByID(ctx, claims.UserID)
	if errors.Is(err, ErrNotFound) || (err == nil && account.SessionVersion != claims.SessionVersion) {
		return Principal{}, access.ErrUnauthenticated
	}
	if err != nil {
		return Principal{}, err
	}
	return Principal{Actor: access.Actor{UserID: account.User.ID, Role: account.User.Role}, User: account.User}, nil
}

// ChangePassword replaces the caller's own password and returns a new session,
// because the change revokes every previously issued token.
func (s *Service) ChangePassword(ctx context.Context, actor access.Actor, current, next string) (Session, error) {
	if err := actor.Require(access.Read); err != nil {
		return Session{}, err
	}
	account, err := s.store.AccountByID(ctx, actor.UserID)
	if errors.Is(err, ErrNotFound) {
		return Session{}, access.ErrUnauthenticated
	}
	if err != nil {
		return Session{}, err
	}
	if !s.passwords.Matches(account.PasswordHash, current) {
		return Session{}, ErrCurrentPasswordMismatch
	}
	if err := domain.ValidatePassword(next); err != nil {
		return Session{}, err
	}
	hash, err := s.passwords.Hash(next)
	if err != nil {
		return Session{}, err
	}
	version, err := s.store.SetPasswordHash(ctx, account.User.ID, hash)
	if errors.Is(err, ErrNotFound) {
		return Session{}, access.ErrUnauthenticated
	}
	if err != nil {
		return Session{}, err
	}
	return s.session(account.User, version)
}

type UserQuery struct {
	Search, Role, Sort, Order string
	Page, PageSize            int
}

type UserPage struct {
	Users                 []domain.User
	Total, Page, PageSize int
}

const (
	defaultPageSize = 10
	maxPageSize     = 100
	maxPage         = 1 << 20
	maxSearchChars  = 100
	defaultSort     = "createdAt"
)

var userSorts = []string{"name", "email", "role", defaultSort}

func (s *Service) ListUsers(ctx context.Context, actor access.Actor, query UserQuery) (UserPage, error) {
	if err := actor.Require(access.UserManage); err != nil {
		return UserPage{}, err
	}
	filter, page, err := userFilter(query)
	if err != nil {
		return UserPage{}, err
	}
	users, total, err := s.store.ListUsers(ctx, filter)
	if err != nil {
		return UserPage{}, err
	}
	return UserPage{Users: users, Total: total, Page: page, PageSize: filter.Limit}, nil
}

func userFilter(q UserQuery) (UserFilter, int, error) {
	f := UserFilter{Search: strings.TrimSpace(q.Search), Sort: q.Sort, Limit: q.PageSize}
	if q.Page < 0 || q.Page > maxPage || q.PageSize < 0 || !utf8.ValidString(f.Search) || utf8.RuneCountInString(f.Search) > maxSearchChars {
		return UserFilter{}, 0, ErrInvalidQuery
	}
	if q.Role != "" {
		role, err := access.ParseRole(q.Role)
		if err != nil {
			return UserFilter{}, 0, ErrInvalidQuery
		}
		f.Role = role
	}
	if f.Sort == "" {
		f.Sort = defaultSort
	}
	if !slices.Contains(userSorts, f.Sort) {
		return UserFilter{}, 0, ErrInvalidQuery
	}
	switch q.Order {
	case "":
		f.Descending = f.Sort == defaultSort
	case "asc":
	case "desc":
		f.Descending = true
	default:
		return UserFilter{}, 0, ErrInvalidQuery
	}
	page := max(q.Page, 1)
	if f.Limit == 0 {
		f.Limit = defaultPageSize
	}
	f.Limit = min(f.Limit, maxPageSize)
	f.Offset = (page - 1) * f.Limit
	return f, page, nil
}

func (s *Service) User(ctx context.Context, actor access.Actor, id string) (domain.User, error) {
	if err := actor.Require(access.UserManage); err != nil {
		return domain.User{}, err
	}
	return s.store.User(ctx, id)
}

type CreateUser struct {
	Email, Name, Role string
	// Password is optional; when nil a strong password is generated and returned once.
	Password *string
}

type CreatedUser struct {
	User              domain.User
	GeneratedPassword string
}

func (s *Service) CreateUser(ctx context.Context, actor access.Actor, in CreateUser) (CreatedUser, error) {
	if err := actor.Require(access.UserManage); err != nil {
		return CreatedUser{}, err
	}
	email, err := domain.NormalizeEmail(in.Email)
	if err != nil {
		return CreatedUser{}, err
	}
	name, err := domain.NormalizeName(in.Name)
	if err != nil {
		return CreatedUser{}, err
	}
	role, err := access.ParseRole(in.Role)
	if err != nil {
		return CreatedUser{}, err
	}
	if err := domain.CheckNotSuperAdminRole(role); err != nil {
		return CreatedUser{}, err
	}
	plain, generated, err := s.choosePassword(in.Password)
	if err != nil {
		return CreatedUser{}, err
	}
	hash, err := s.passwords.Hash(plain)
	if err != nil {
		return CreatedUser{}, err
	}
	user, err := s.store.CreateUser(ctx, NewUser{Email: email, Name: name, Role: role, PasswordHash: hash})
	if err != nil {
		return CreatedUser{}, err
	}
	return CreatedUser{User: user, GeneratedPassword: generated}, nil
}

type UpdateUser struct {
	Name, Role *string
}

func (s *Service) UpdateUser(ctx context.Context, actor access.Actor, id string, in UpdateUser) (domain.User, error) {
	if err := actor.Require(access.UserManage); err != nil {
		return domain.User{}, err
	}
	if in.Name == nil && in.Role == nil {
		return domain.User{}, ErrNoChanges
	}
	var change UserChange
	if in.Name != nil {
		name, err := domain.NormalizeName(*in.Name)
		if err != nil {
			return domain.User{}, err
		}
		change.Name = &name
	}
	if in.Role != nil {
		role, err := access.ParseRole(*in.Role)
		if err != nil {
			return domain.User{}, err
		}
		change.Role = &role
	}
	var updated domain.User
	err := s.store.Serializable(ctx, func(tx Store) error {
		target, err := tx.User(ctx, id)
		if err != nil {
			return err
		}
		// The SUPERADMIN account answers to no one, including itself through
		// this route: it cannot be renamed, reassigned or role-changed here.
		if err := domain.CheckNotSuperAdmin(target); err != nil {
			return err
		}
		if change.Role != nil {
			privileged, err := tx.CountPrivileged(ctx)
			if err != nil {
				return err
			}
			if err := domain.CheckRoleChange(target, *change.Role, privileged); err != nil {
				return err
			}
		}
		updated, err = tx.UpdateUser(ctx, id, change)
		return err
	})
	return updated, err
}

// ResetPassword sets another user's password and revokes their tokens. The
// generated password is returned only when none was supplied. The SUPERADMIN
// account is never a valid target: it changes its own password only through
// the self-service /api/auth/password route, which proves the current one.
func (s *Service) ResetPassword(ctx context.Context, actor access.Actor, id string, password *string) (string, error) {
	if err := actor.Require(access.UserManage); err != nil {
		return "", err
	}
	target, err := s.store.User(ctx, id)
	if err != nil {
		return "", err
	}
	if err := domain.CheckNotSuperAdmin(target); err != nil {
		return "", err
	}
	plain, generated, err := s.choosePassword(password)
	if err != nil {
		return "", err
	}
	hash, err := s.passwords.Hash(plain)
	if err != nil {
		return "", err
	}
	if _, err := s.store.SetPasswordHash(ctx, id, hash); err != nil {
		return "", err
	}
	return generated, nil
}

func (s *Service) DeleteUser(ctx context.Context, actor access.Actor, id string) error {
	if err := actor.Require(access.UserManage); err != nil {
		return err
	}
	return s.store.Serializable(ctx, func(tx Store) error {
		target, err := tx.User(ctx, id)
		if err != nil {
			return err
		}
		privileged, err := tx.CountPrivileged(ctx)
		if err != nil {
			return err
		}
		if err := domain.CheckDeletion(actor.UserID, target, privileged); err != nil {
			return err
		}
		return tx.DeleteUser(ctx, id)
	})
}

// BootstrapAdministrator creates the first SUPERADMIN when no privileged user
// exists. Only the installation command calls it; because the last privileged
// user cannot be removed, repeated runs never create another administrator.
func (s *Service) BootstrapAdministrator(ctx context.Context, email string) (bool, string, error) {
	normalized, err := domain.NormalizeEmail(email)
	if err != nil {
		return false, "", err
	}
	password, err := s.passwords.Generate()
	if err != nil {
		return false, "", err
	}
	hash, err := s.passwords.Hash(password)
	if err != nil {
		return false, "", err
	}
	created := false
	err = s.store.Serializable(ctx, func(tx Store) error {
		created = false
		privileged, err := tx.CountPrivileged(ctx)
		if err != nil || privileged > 0 {
			return err
		}
		_, err = tx.CreateUser(ctx, NewUser{Email: normalized, Name: "Administrator", Role: access.SuperAdmin, PasswordHash: hash})
		created = err == nil
		return err
	})
	if err != nil || !created {
		return false, "", err
	}
	return true, password, nil
}

func (s *Service) session(user domain.User, version int) (Session, error) {
	token, err := s.tokens.Issue(Claims{UserID: user.ID, SessionVersion: version})
	if err != nil {
		return Session{}, err
	}
	return Session{Token: token, User: user}, nil
}

// choosePassword validates a supplied password or generates one. generated is
// empty when the caller supplied the password.
func (s *Service) choosePassword(supplied *string) (plain, generated string, err error) {
	if supplied != nil {
		return *supplied, "", domain.ValidatePassword(*supplied)
	}
	generated, err = s.passwords.Generate()
	return generated, generated, err
}
