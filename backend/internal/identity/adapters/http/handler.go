// Package identityhttp exposes authentication and user management over HTTP.
package identityhttp

import (
	access "github.com/HappyHackingSpace/Misko/backend/internal/access/domain"
	"github.com/HappyHackingSpace/Misko/backend/internal/identity/application"
	"github.com/HappyHackingSpace/Misko/backend/internal/identity/domain"
	"github.com/HappyHackingSpace/Misko/backend/internal/platform/httpjson"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

var failures = []httpjson.Failure{
	{Err: access.ErrUnauthenticated, Status: http.StatusUnauthorized, Code: "auth.unauthenticated"},
	{Err: access.ErrForbidden, Status: http.StatusForbidden, Code: "auth.forbidden"},
	{Err: application.ErrInvalidCredentials, Status: http.StatusUnauthorized, Code: "auth.invalidCredentials"},
	{Err: application.ErrCurrentPasswordMismatch, Status: http.StatusBadRequest, Code: "auth.currentPasswordMismatch"},
	{Err: application.ErrNotFound, Status: http.StatusNotFound, Code: "user.notFound"},
	{Err: application.ErrEmailTaken, Status: http.StatusConflict, Code: "user.emailExists"},
	{Err: application.ErrNoChanges, Status: http.StatusBadRequest, Code: "common.noChanges"},
	{Err: application.ErrInvalidQuery, Status: http.StatusBadRequest, Code: "common.invalidQuery"},
	{Err: access.ErrUnknownRole, Status: http.StatusBadRequest, Code: "user.invalidRole"},
	{Err: domain.ErrInvalidEmail, Status: http.StatusBadRequest, Code: "user.invalidEmail"},
	{Err: domain.ErrInvalidName, Status: http.StatusBadRequest, Code: "user.invalidName"},
	{Err: domain.ErrPasswordTooShort, Status: http.StatusBadRequest, Code: "password.tooShort"},
	{Err: domain.ErrPasswordTooLong, Status: http.StatusBadRequest, Code: "password.tooLong"},
	{Err: domain.ErrPasswordEncoding, Status: http.StatusBadRequest, Code: "password.invalidEncoding"},
	{Err: domain.ErrSelfDeletion, Status: http.StatusBadRequest, Code: "user.cannotDeleteSelf"},
	{Err: domain.ErrLastPrivileged, Status: http.StatusConflict, Code: "user.lastPrivileged"},
	{Err: domain.ErrSuperAdminProtected, Status: http.StatusForbidden, Code: "user.superAdminProtected"},
}

type handler struct {
	service *application.Service
	logger  *slog.Logger
}

// Register mounts authentication and user routes. There is no signup route:
// accounts are created by the setup command or by users holding user:manage.
func Register(mux *http.ServeMux, service *application.Service, logger *slog.Logger) {
	h := handler{service: service, logger: logger}
	mux.HandleFunc("POST /api/auth/login", h.login)
	mux.HandleFunc("GET /api/auth/me", h.authenticated(h.me))
	mux.HandleFunc("POST /api/auth/password", h.authenticated(h.changePassword))
	mux.HandleFunc("GET /api/users", h.authenticated(h.listUsers))
	mux.HandleFunc("POST /api/users", h.authenticated(h.createUser))
	mux.HandleFunc("GET /api/users/{id}", h.authenticated(h.getUser))
	mux.HandleFunc("PATCH /api/users/{id}", h.authenticated(h.updateUser))
	mux.HandleFunc("POST /api/users/{id}/reset-password", h.authenticated(h.resetPassword))
	mux.HandleFunc("DELETE /api/users/{id}", h.authenticated(h.deleteUser))
}

// Authenticator resolves the current actor for other domains' HTTP adapters,
// so they depend on the identity use case rather than on this adapter.
func Authenticator(service *application.Service) func(*http.Request) (access.Actor, error) {
	return func(r *http.Request) (access.Actor, error) {
		principal, err := authenticate(service, r)
		return principal.Actor, err
	}
}

func authenticate(service *application.Service, r *http.Request) (application.Principal, error) {
	scheme, token, found := strings.Cut(r.Header.Get("Authorization"), " ")
	if !found || !strings.EqualFold(scheme, "Bearer") || token == "" {
		return application.Principal{}, access.ErrUnauthenticated
	}
	return service.Authenticate(r.Context(), token)
}

func (h handler) authenticated(next func(http.ResponseWriter, *http.Request, application.Principal)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		principal, err := authenticate(h.service, r)
		if err != nil {
			h.fail(w, r, err)
			return
		}
		next(w, r, principal)
	}
}

func (h handler) login(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := httpjson.Decode(w, r, &body); err != nil {
		h.fail(w, r, err)
		return
	}
	session, err := h.service.Login(r.Context(), body.Email, body.Password)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpjson.Write(w, http.StatusOK, toSession(session))
}

func (h handler) me(w http.ResponseWriter, _ *http.Request, p application.Principal) {
	permissions := []access.Permission{}
	for _, permission := range access.Permissions() {
		if p.Actor.Role.Allows(permission) {
			permissions = append(permissions, permission)
		}
	}
	httpjson.Write(w, http.StatusOK, struct {
		User        userJSON            `json:"user"`
		Permissions []access.Permission `json:"permissions"`
	}{toUser(p.User), permissions})
}

func (h handler) changePassword(w http.ResponseWriter, r *http.Request, p application.Principal) {
	var body struct {
		CurrentPassword string `json:"currentPassword"`
		NewPassword     string `json:"newPassword"`
	}
	if err := httpjson.Decode(w, r, &body); err != nil {
		h.fail(w, r, err)
		return
	}
	session, err := h.service.ChangePassword(r.Context(), p.Actor, body.CurrentPassword, body.NewPassword)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpjson.Write(w, http.StatusOK, toSession(session))
}

func (h handler) listUsers(w http.ResponseWriter, r *http.Request, p application.Principal) {
	q := r.URL.Query()
	page, err := h.service.ListUsers(r.Context(), p.Actor, application.UserQuery{
		Search: q.Get("search"), Role: q.Get("role"), Sort: q.Get("sort"), Order: q.Get("order"),
		Page: httpjson.QueryInt(q.Get("page")), PageSize: httpjson.QueryInt(q.Get("pageSize")),
	})
	if err != nil {
		h.fail(w, r, err)
		return
	}
	users := make([]userJSON, 0, len(page.Users))
	for _, u := range page.Users {
		users = append(users, toUser(u))
	}
	httpjson.Write(w, http.StatusOK, struct {
		Data     []userJSON `json:"data"`
		Total    int        `json:"total"`
		Page     int        `json:"page"`
		PageSize int        `json:"pageSize"`
	}{users, page.Total, page.Page, page.PageSize})
}

func (h handler) createUser(w http.ResponseWriter, r *http.Request, p application.Principal) {
	var body struct {
		Email    string  `json:"email"`
		Name     string  `json:"name"`
		Role     string  `json:"role"`
		Password *string `json:"password"`
	}
	if err := httpjson.Decode(w, r, &body); err != nil {
		h.fail(w, r, err)
		return
	}
	created, err := h.service.CreateUser(r.Context(), p.Actor, application.CreateUser{Email: body.Email, Name: body.Name, Role: body.Role, Password: body.Password})
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpjson.Write(w, http.StatusCreated, struct {
		User              userJSON `json:"user"`
		GeneratedPassword string   `json:"generatedPassword,omitempty"`
	}{toUser(created.User), created.GeneratedPassword})
}

func (h handler) getUser(w http.ResponseWriter, r *http.Request, p application.Principal) {
	user, err := h.service.User(r.Context(), p.Actor, r.PathValue("id"))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpjson.Write(w, http.StatusOK, toUser(user))
}

func (h handler) updateUser(w http.ResponseWriter, r *http.Request, p application.Principal) {
	var body struct {
		Name *string `json:"name"`
		Role *string `json:"role"`
	}
	if err := httpjson.Decode(w, r, &body); err != nil {
		h.fail(w, r, err)
		return
	}
	user, err := h.service.UpdateUser(r.Context(), p.Actor, r.PathValue("id"), application.UpdateUser{Name: body.Name, Role: body.Role})
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpjson.Write(w, http.StatusOK, toUser(user))
}

func (h handler) resetPassword(w http.ResponseWriter, r *http.Request, p application.Principal) {
	var body struct {
		Password *string `json:"password"`
	}
	if err := httpjson.Decode(w, r, &body); err != nil {
		h.fail(w, r, err)
		return
	}
	generated, err := h.service.ResetPassword(r.Context(), p.Actor, r.PathValue("id"), body.Password)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpjson.Write(w, http.StatusOK, struct {
		GeneratedPassword string `json:"generatedPassword,omitempty"`
	}{generated})
}

func (h handler) deleteUser(w http.ResponseWriter, r *http.Request, p application.Principal) {
	if err := h.service.DeleteUser(r.Context(), p.Actor, r.PathValue("id")); err != nil {
		h.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h handler) fail(w http.ResponseWriter, r *http.Request, err error) {
	httpjson.Fail(w, r, h.logger, err, failures)
}

type userJSON struct {
	ID        string    `json:"id"`
	Email     string    `json:"email"`
	Name      string    `json:"name"`
	Role      string    `json:"role"`
	CreatedAt time.Time `json:"createdAt"`
}

func toUser(u domain.User) userJSON {
	return userJSON{ID: u.ID, Email: u.Email, Name: u.Name, Role: string(u.Role), CreatedAt: u.CreatedAt}
}

type sessionJSON struct {
	Token     string    `json:"token"`
	TokenType string    `json:"tokenType"`
	ExpiresAt time.Time `json:"expiresAt"`
	User      userJSON  `json:"user"`
}

func toSession(s application.Session) sessionJSON {
	return sessionJSON{Token: s.Token.Value, TokenType: "Bearer", ExpiresAt: s.Token.ExpiresAt, User: toUser(s.User)}
}
