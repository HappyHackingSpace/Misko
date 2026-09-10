package healthhttp

import (
	"context"
	"errors"
	"github.com/HappyHackingSpace/Misko/backend/internal/health/application"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type dependencyFunc func(context.Context) error

func (f dependencyFunc) Check(ctx context.Context) error { return f(ctx) }

func TestProbeEndpoints(t *testing.T) {
	s := application.New(dependencyFunc(func(context.Context) error { return errors.New("sensitive database detail") }), time.Second)
	h := New(s)
	for _, tc := range []struct {
		method, path string
		code         int
	}{
		{"GET", "/api/health", 200}, {"GET", "/api/ready", 503}, {"POST", "/api/health", 405}, {"GET", "/unknown", 404},
	} {
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, httptest.NewRequest(tc.method, tc.path, nil))
		if rr.Code != tc.code {
			t.Errorf("%s %s: got %d want %d", tc.method, tc.path, rr.Code, tc.code)
		}
		if strings.Contains(rr.Body.String(), "sensitive") {
			t.Fatal("dependency error leaked")
		}
		if tc.code == 200 || tc.code == 503 {
			if rr.Header().Get("Cache-Control") != "no-store" {
				t.Fatal("probe can be cached")
			}
		}
	}
}
