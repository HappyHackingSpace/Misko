package token

import (
	"github.com/HappyHackingSpace/Misko/backend/internal/identity/application"
	"github.com/golang-jwt/jwt/v5"
	"strings"
	"testing"
	"time"
)

var (
	secret = []byte("0123456789abcdef0123456789abcdef")
	issued = time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)
)

func newJWT(t *testing.T, issuer, audience string, now time.Time) *JWT {
	t.Helper()
	j, err := NewJWT(secret, issuer, audience, time.Hour, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	return j
}

func TestConstructorRejectsWeakConfiguration(t *testing.T) {
	now := func() time.Time { return issued }
	for name, build := range map[string]func() (*JWT, error){
		"short secret":   func() (*JWT, error) { return NewJWT(secret[:31], "misko", "misko-api", time.Hour, now) },
		"empty issuer":   func() (*JWT, error) { return NewJWT(secret, "", "misko-api", time.Hour, now) },
		"empty audience": func() (*JWT, error) { return NewJWT(secret, "misko", "", time.Hour, now) },
		"zero ttl":       func() (*JWT, error) { return NewJWT(secret, "misko", "misko-api", 0, now) },
	} {
		if _, err := build(); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
}

func TestIssueAndVerifyUntilExpiry(t *testing.T) {
	token, err := newJWT(t, "misko", "misko-api", issued).Issue(application.Claims{UserID: "user-1", SessionVersion: 3})
	if err != nil {
		t.Fatal(err)
	}
	if !token.ExpiresAt.Equal(issued.Add(time.Hour)) {
		t.Fatalf("expiresAt=%v", token.ExpiresAt)
	}
	claims, err := newJWT(t, "misko", "misko-api", issued.Add(time.Hour-time.Second)).Verify(token.Value)
	if err != nil || claims != (application.Claims{UserID: "user-1", SessionVersion: 3}) {
		t.Fatalf("claims=%+v err=%v", claims, err)
	}
	if _, err := newJWT(t, "misko", "misko-api", issued.Add(time.Hour+time.Second)).Verify(token.Value); err == nil {
		t.Fatal("expired token accepted")
	}
}

func TestVerifyRejectsForgedOrMismatchedTokens(t *testing.T) {
	verifier := newJWT(t, "misko", "misko-api", issued)
	valid := func() jwt.MapClaims {
		return jwt.MapClaims{"iss": "misko", "aud": "misko-api", "sub": "user-1", "sv": 1, "iat": issued.Unix(), "exp": issued.Add(time.Hour).Unix()}
	}
	sign := func(method jwt.SigningMethod, key any, claims jwt.MapClaims) string {
		t.Helper()
		raw, err := jwt.NewWithClaims(method, claims).SignedString(key)
		if err != nil {
			t.Fatal(err)
		}
		return raw
	}
	with := func(key string, value any) jwt.MapClaims {
		c := valid()
		if value == nil {
			delete(c, key)
		} else {
			c[key] = value
		}
		return c
	}
	good := sign(jwt.SigningMethodHS256, secret, valid())
	if _, err := verifier.Verify(good); err != nil {
		t.Fatalf("control token rejected: %v", err)
	}
	parts := strings.Split(good, ".")
	escalated := strings.Split(sign(jwt.SigningMethodHS256, secret, with("sub", "admin")), ".")
	for name, raw := range map[string]string{
		"HS512 algorithm":   sign(jwt.SigningMethodHS512, secret, valid()),
		"none algorithm":    sign(jwt.SigningMethodNone, jwt.UnsafeAllowNoneSignatureType, valid()),
		"other secret":      sign(jwt.SigningMethodHS256, []byte(strings.Repeat("x", 32)), valid()),
		"wrong issuer":      sign(jwt.SigningMethodHS256, secret, with("iss", "someone-else")),
		"wrong audience":    sign(jwt.SigningMethodHS256, secret, with("aud", "other-api")),
		"missing audience":  sign(jwt.SigningMethodHS256, secret, with("aud", nil)),
		"missing expiry":    sign(jwt.SigningMethodHS256, secret, with("exp", nil)),
		"future issued-at":  sign(jwt.SigningMethodHS256, secret, with("iat", issued.Add(time.Hour).Unix())),
		"missing subject":   sign(jwt.SigningMethodHS256, secret, with("sub", nil)),
		"missing version":   sign(jwt.SigningMethodHS256, secret, with("sv", nil)),
		"zero version":      sign(jwt.SigningMethodHS256, secret, with("sv", 0)),
		"tampered payload":  parts[0] + "." + escalated[1] + "." + parts[2],
		"empty":             "",
		"oversized":         strings.Repeat("a", 4097),
		"not a jwt":         "garbage",
		"signature removed": parts[0] + "." + parts[1] + ".",
	} {
		if _, err := verifier.Verify(raw); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
}
