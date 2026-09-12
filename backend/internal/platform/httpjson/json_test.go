package httpjson

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDecodeAcceptsOnlyOneKnownObject(t *testing.T) {
	type body struct {
		Name string `json:"name"`
	}
	for _, tc := range []struct {
		name, input string
		want        error
	}{
		{"valid", `{"name":"Ada"}`, nil},
		{"unknown field", `{"name":"Ada","role":"SUPERADMIN"}`, ErrInvalidJSON},
		{"trailing value", `{"name":"Ada"} {}`, ErrInvalidJSON},
		{"empty", ``, ErrInvalidJSON},
		{"wrong type", `{"name":7}`, ErrInvalidJSON},
		{"array", `[]`, ErrInvalidJSON},
		{"too large", `{"name":"` + strings.Repeat("a", MaxBodyBytes) + `"}`, ErrBodyTooLarge},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var dst body
			rr := httptest.NewRecorder()
			err := Decode(rr, httptest.NewRequest("POST", "/", strings.NewReader(tc.input)), &dst)
			if !errors.Is(err, tc.want) || (tc.want == nil && err != nil) {
				t.Fatalf("err=%v want %v", err, tc.want)
			}
		})
	}
}

func TestResponsesAreJSONAndNotCached(t *testing.T) {
	rr := httptest.NewRecorder()
	Error(rr, 409, "user.emailExists", "email is already registered")
	if rr.Code != 409 || rr.Header().Get("Content-Type") != "application/json" || rr.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("code=%d headers=%v", rr.Code, rr.Header())
	}
	if got := strings.TrimSpace(rr.Body.String()); got != `{"error":"email is already registered","code":"user.emailExists"}` {
		t.Fatalf("body=%s", got)
	}
}

func TestFailMapsKnownErrorsAndHidesUnknownOnes(t *testing.T) {
	unauthenticated := errors.New("authentication required")
	failures := []Failure{{unauthenticated, 401, "auth.unauthenticated"}}
	var logs strings.Builder
	logger := slog.New(slog.NewJSONHandler(&logs, nil))
	for _, tc := range []struct {
		err    error
		status int
		code   string
	}{
		{fmt.Errorf("verify: %w", unauthenticated), 401, "auth.unauthenticated"},
		{fmt.Errorf("%w: field secret-value", ErrInvalidJSON), 400, "common.invalidJSON"},
		{ErrBodyTooLarge, 413, "common.bodyTooLarge"},
		{errors.New("dial tcp: database detail"), 500, "common.serverError"},
	} {
		rr := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/api/users?token=query-secret", nil)
		req.Header.Set("Authorization", "Bearer header-secret")
		Fail(rr, req, logger, tc.err, failures)
		if rr.Code != tc.status || !strings.Contains(rr.Body.String(), `"code":"`+tc.code+`"`) {
			t.Fatalf("%v: %d %s", tc.err, rr.Code, rr.Body)
		}
		if strings.Contains(rr.Body.String(), "detail") || strings.Contains(rr.Body.String(), "secret") {
			t.Fatalf("error detail leaked to client: %s", rr.Body)
		}
		if (tc.status == 401) != (rr.Header().Get("WWW-Authenticate") == "Bearer") {
			t.Fatalf("%d: WWW-Authenticate=%q", tc.status, rr.Header().Get("WWW-Authenticate"))
		}
	}
	if !strings.Contains(logs.String(), "request failed") || strings.Count(logs.String(), "\n") != 1 {
		t.Fatalf("only unknown errors are logged: %s", logs.String())
	}
	if strings.Contains(logs.String(), "header-secret") || strings.Contains(logs.String(), "query-secret") {
		t.Fatalf("request credentials logged: %s", logs.String())
	}
}
