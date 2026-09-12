package token

import (
	"bytes"
	"strings"
	"testing"
)

func TestTokensAreRandomAndHashedConsistently(t *testing.T) {
	a, hashA, err := New().New()
	if err != nil {
		t.Fatal(err)
	}
	b, _, err := New().New()
	if err != nil {
		t.Fatal(err)
	}
	if a == b || !strings.HasPrefix(a, "mw_") || len(a) < 40 {
		t.Fatalf("tokens: %q %q", a, b)
	}
	if len(hashA) != 32 || !bytes.Equal(hashA, New().Hash(a)) || bytes.Equal(hashA, New().Hash(b)) || bytes.Contains(hashA, []byte(a)) {
		t.Fatal("hash must be a 32-byte digest of exactly this token")
	}
}
