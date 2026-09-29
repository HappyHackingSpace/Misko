//go:build integration

package bootstrap

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/HappyHackingSpace/Misko/backend/internal/identity/adapters/token"
	identityapp "github.com/HappyHackingSpace/Misko/backend/internal/identity/application"
	"github.com/HappyHackingSpace/Misko/backend/internal/platform/config"
	"github.com/HappyHackingSpace/Misko/backend/internal/platform/pgtest"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestAPIAuthenticationAndAuthorization drives the composed API over HTTP so the
// checks cannot be bypassed by skipping UI controls.
func TestAPIAuthenticationAndAuthorization(t *testing.T) {
	ctx := context.Background()
	pool := pgtest.Database(t)
	secret := []byte("integration-signing-secret-0123456789")
	first, err := Setup(ctx, pool, config.Setup{AdminEmail: "Admin@Lab.io", LabName: "Integration Lab", LabTimezone: "Europe/Istanbul", BcryptCost: bcrypt.MinCost})
	if err != nil || !first.LaboratoryCreated || !first.AdministratorCreated || first.AdminPassword == "" {
		t.Fatalf("first setup: %+v %v", first, err)
	}
	again, err := Setup(ctx, pool, config.Setup{AdminEmail: "second@lab.io", LabName: "Replacement", LabTimezone: "UTC", BcryptCost: bcrypt.MinCost})
	if err != nil || again.LaboratoryCreated || again.AdministratorCreated || again.AdminPassword != "" {
		t.Fatalf("repeated setup: %+v %v", again, err)
	}

	logs := &lockedBuffer{}
	api, err := NewAPI(pool, config.Config{ProbeTimeout: time.Second},
		config.Auth{JWTSecret: secret, JWTIssuer: "misko", JWTAudience: "misko-api", TokenTTL: time.Hour, BcryptCost: bcrypt.MinCost},
		slog.New(slog.NewJSONHandler(logs, nil)))
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(api.Handler)
	defer server.Close()
	credentials := []string{first.AdminPassword, string(secret)}
	call := func(method, path, authorization string, body any) (int, map[string]any, http.Header) {
		t.Helper()
		var reader io.Reader
		if body != nil {
			raw, _ := json.Marshal(body)
			reader = bytes.NewReader(raw)
		}
		req, _ := http.NewRequest(method, server.URL+path, reader)
		if authorization != "" {
			req.Header.Set("Authorization", authorization)
		}
		res, err := server.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		var decoded map[string]any
		_ = json.NewDecoder(res.Body).Decode(&decoded)
		return res.StatusCode, decoded, res.Header
	}
	expect := func(label string, got, want int, body map[string]any, code string) {
		t.Helper()
		if got != want || (code != "" && body["code"] != code) {
			t.Fatalf("%s: status=%d body=%v want %d %s", label, got, body, want, code)
		}
	}
	login := func(email, password string) (string, string) {
		t.Helper()
		status, body, _ := call("POST", "/api/auth/login", "", map[string]string{"email": email, "password": password})
		expect("login "+email, status, 200, body, "")
		credentials = append(credentials, body["token"].(string), password)
		return "Bearer " + body["token"].(string), body["user"].(map[string]any)["id"].(string)
	}

	status, body, _ := call("GET", "/api/meta", "", nil)
	if status != 200 || body["labName"] != "Integration Lab" {
		t.Fatalf("meta: %d %v", status, body)
	}
	for _, path := range []string{"/api/auth/signup", "/api/auth/register"} {
		status, _, _ := call("POST", path, "", map[string]string{"email": "x@lab.io", "password": "Password-123"})
		expect("public signup "+path, status, 404, nil, "")
	}
	status, body, _ = call("POST", "/api/auth/login", "", map[string]string{"email": "admin@lab.io", "password": "Wrong-Password-1"})
	expect("wrong password", status, 401, body, "auth.invalidCredentials")
	admin, adminID := login("ADMIN@lab.io", first.AdminPassword)

	sign := func(method jwt.SigningMethod, claims jwt.MapClaims) string {
		raw, err := jwt.NewWithClaims(method, claims).SignedString(secret)
		if err != nil {
			t.Fatal(err)
		}
		return "Bearer " + raw
	}
	valid := jwt.MapClaims{"iss": "misko", "aud": "misko-api", "sub": adminID, "sv": 1, "iat": time.Now().Unix(), "exp": time.Now().Add(time.Hour).Unix()}
	expiredIssuer, _ := token.NewJWT(secret, "misko", "misko-api", time.Minute, func() time.Time { return time.Now().Add(-time.Hour) })
	expired, _ := expiredIssuer.Issue(identityapp.Claims{UserID: adminID, SessionVersion: 1})
	wrongAudience := maps(valid, "aud", "another-api")
	for label, authorization := range map[string]string{
		"missing":        "",
		"basic":          "Basic YWRtaW46cGFzcw==",
		"empty bearer":   "Bearer ",
		"garbage":        "Bearer garbage",
		"HS512":          sign(jwt.SigningMethodHS512, valid),
		"wrong audience": sign(jwt.SigningMethodHS256, wrongAudience),
		"expired":        "Bearer " + expired.Value,
	} {
		status, body, header := call("GET", "/api/users", authorization, nil)
		expect("credentials "+label, status, 401, body, "auth.unauthenticated")
		if header.Get("WWW-Authenticate") != "Bearer" {
			t.Fatalf("%s: missing WWW-Authenticate", label)
		}
	}
	if status, _, _ := call("GET", "/api/users", sign(jwt.SigningMethodHS256, valid), nil); status != 200 {
		t.Fatalf("control token rejected: %d", status)
	}

	status, body, _ = call("POST", "/api/users", admin, map[string]any{"email": "Researcher@Lab.io", "name": "Researcher", "role": "RESEARCHER"})
	expect("create researcher", status, 201, body, "")
	researcherID := body["user"].(map[string]any)["id"].(string)
	researcherPassword := body["generatedPassword"].(string)
	status, body, _ = call("POST", "/api/users", admin, map[string]any{"email": "viewer@lab.io", "name": "Viewer", "role": "VIEWER", "password": "Viewer-Password-1"})
	if status != 201 || body["generatedPassword"] != nil {
		t.Fatalf("create viewer: %d %v", status, body)
	}
	viewerID := body["user"].(map[string]any)["id"].(string)
	status, body, _ = call("POST", "/api/users", admin, map[string]any{"email": "x@lab.io", "name": "X", "role": "ADMIN"})
	expect("unknown role", status, 400, body, "user.invalidRole")
	status, body, _ = call("POST", "/api/users", admin, map[string]any{"email": "x@lab.io", "name": "X", "role": "VIEWER", "isAdmin": true})
	expect("unknown field", status, 400, body, "common.invalidJSON")
	status, body, _ = call("POST", "/api/users", admin, map[string]any{"email": "VIEWER@lab.io", "name": "X", "role": "VIEWER"})
	expect("duplicate email", status, 409, body, "user.emailExists")

	researcher, _ := login("researcher@lab.io", researcherPassword)
	status, body, _ = call("GET", "/api/auth/me", researcher, nil)
	permissions := body["permissions"].([]any)
	if status != 200 || body["user"].(map[string]any)["role"] != "RESEARCHER" || !slices.Contains(permissions, any("test:write")) || slices.Contains(permissions, any("user:manage")) {
		t.Fatalf("me: %d %v", status, body)
	}
	for _, r := range []struct {
		method, path string
		body         any
	}{
		{"GET", "/api/users", nil},
		{"GET", "/api/users/" + researcherID, nil},
		{"POST", "/api/users", map[string]any{"email": "y@lab.io", "name": "Y", "role": "SUPERADMIN"}},
		{"PATCH", "/api/users/" + researcherID, map[string]any{"role": "SUPERADMIN"}},
		{"POST", "/api/users/" + viewerID + "/reset-password", map[string]any{}},
		{"DELETE", "/api/users/" + viewerID, nil},
		{"PATCH", "/api/lab", map[string]any{"name": "Hijacked"}},
	} {
		status, body, _ := call(r.method, r.path, researcher, r.body)
		expect("researcher "+r.method+" "+r.path, status, 403, body, "auth.forbidden")
	}
	status, body, _ = call("GET", "/api/lab", researcher, nil)
	if status != 200 || body["name"] != "Integration Lab" || body["timezone"] != "Europe/Istanbul" || body["code"] != nil {
		t.Fatalf("lab read: %d %v", status, body)
	}
	status, body, _ = call("GET", "/api/users/"+researcherID, admin, nil)
	if status != 200 || body["role"] != "RESEARCHER" {
		t.Fatalf("denied escalation changed role: %d %v", status, body)
	}

	status, body, _ = call("PATCH", "/api/users/"+researcherID, admin, map[string]any{"role": "LAB_MANAGER"})
	expect("promote", status, 200, body, "")
	status, body, _ = call("GET", "/api/users?sort=email&order=asc&pageSize=2", researcher, nil)
	if status != 200 || body["total"] != float64(3) || len(body["data"].([]any)) != 2 || body["pageSize"] != float64(2) {
		t.Fatalf("role change not effective on next request: %d %v", status, body)
	}
	manager := researcher
	status, body, _ = call("GET", "/api/users?sort=password_hash", manager, nil)
	expect("unknown sort", status, 400, body, "common.invalidQuery")
	status, body, _ = call("DELETE", "/api/users/"+adminID, admin, nil)
	expect("self deletion", status, 400, body, "user.cannotDeleteSelf")

	// The SUPERADMIN account is permanent: no one, including a fellow
	// privileged LAB_MANAGER, can demote, rename, delete or reset the
	// password of it, or mint a second one, through the API.
	status, body, _ = call("PATCH", "/api/users/"+adminID, manager, map[string]any{"role": "VIEWER"})
	expect("demote admin", status, 403, body, "user.superAdminProtected")
	status, body, _ = call("PATCH", "/api/users/"+adminID, manager, map[string]any{"name": "Renamed"})
	expect("rename admin", status, 403, body, "user.superAdminProtected")
	status, body, _ = call("DELETE", "/api/users/"+adminID, manager, nil)
	expect("delete admin", status, 403, body, "user.superAdminProtected")
	status, body, _ = call("POST", "/api/users/"+adminID+"/reset-password", manager, map[string]any{})
	expect("reset admin password", status, 403, body, "user.superAdminProtected")
	status, body, _ = call("POST", "/api/users", manager, map[string]any{"email": "second-admin@lab.io", "name": "Second", "role": "SUPERADMIN"})
	expect("mint a second superadmin", status, 403, body, "user.superAdminProtected")
	status, body, _ = call("GET", "/api/users", admin, nil)
	expect("admin retains access", status, 200, body, "")

	status, body, _ = call("DELETE", "/api/users/00000000-0000-0000-0000-000000000000", manager, nil)
	expect("missing user", status, 404, body, "user.notFound")

	viewer, _ := login("viewer@lab.io", "Viewer-Password-1")
	status, body, _ = call("POST", "/api/users/"+viewerID+"/reset-password", manager, map[string]any{})
	expect("reset", status, 200, body, "")
	reset := body["generatedPassword"].(string)
	credentials = append(credentials, reset)
	status, body, _ = call("GET", "/api/auth/me", viewer, nil)
	expect("token after reset", status, 401, body, "auth.unauthenticated")
	viewer, _ = login("viewer@lab.io", reset)
	status, body, _ = call("POST", "/api/auth/password", viewer, map[string]string{"currentPassword": "Wrong-Password-1", "newPassword": "Viewer-Password-2"})
	expect("wrong current password", status, 400, body, "auth.currentPasswordMismatch")
	status, body, _ = call("POST", "/api/auth/password", viewer, map[string]string{"currentPassword": reset, "newPassword": "Viewer-Password-2"})
	expect("change password", status, 200, body, "")
	replacement := "Bearer " + body["token"].(string)
	credentials = append(credentials, body["token"].(string), "Viewer-Password-2")
	status, body, _ = call("GET", "/api/auth/me", viewer, nil)
	expect("token after own change", status, 401, body, "auth.unauthenticated")
	if status, _, _ := call("GET", "/api/auth/me", replacement, nil); status != 200 {
		t.Fatalf("replacement token: %d", status)
	}
	status, _, _ = call("DELETE", "/api/users/"+viewerID, manager, nil)
	expect("delete viewer", status, 204, nil, "")
	status, body, _ = call("GET", "/api/auth/me", replacement, nil)
	expect("deleted user", status, 401, body, "auth.unauthenticated")

	status, body, _ = call("PATCH", "/api/lab", manager, map[string]any{"code": "HHS-1", "timezone": "UTC"})
	if status != 200 || body["code"] != "HHS-1" || body["timezone"] != "UTC" || body["name"] != "Integration Lab" {
		t.Fatalf("lab update: %d %v", status, body)
	}
	status, body, _ = call("PATCH", "/api/lab", manager, map[string]any{"timezone": "Mars/Base"})
	expect("invalid timezone", status, 400, body, "lab.invalidTimezone")

	pool.Close()
	status, body, _ = call("GET", "/api/auth/me", manager, nil)
	expect("database unavailable", status, 500, body, "common.serverError")
	written := logs.String()
	if !strings.Contains(written, "request failed") {
		t.Fatalf("server error not logged: %s", written)
	}
	for _, credential := range credentials {
		if strings.Contains(written, strings.TrimPrefix(credential, "Bearer ")) {
			t.Fatal("logs contain a password, token or signing secret")
		}
	}
}

func maps(claims jwt.MapClaims, key string, value any) jwt.MapClaims {
	copied := jwt.MapClaims{}
	for k, v := range claims {
		copied[k] = v
	}
	copied[key] = value
	return copied
}

type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}
