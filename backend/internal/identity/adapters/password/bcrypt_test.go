package password

import (
	"github.com/HappyHackingSpace/Misko/backend/internal/identity/domain"
	"golang.org/x/crypto/bcrypt"
	"strings"
	"testing"
)

func TestConstructorRejectsUnsupportedCost(t *testing.T) {
	for _, cost := range []int{bcrypt.MinCost - 1, bcrypt.MaxCost + 1} {
		if _, err := NewBcrypt(cost); err == nil {
			t.Errorf("cost %d accepted", cost)
		}
	}
}

func TestHashAndMatchBoundaries(t *testing.T) {
	b, err := NewBcrypt(bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	for _, plain := range []string{"abcdefgh", strings.Repeat("ş", 36), strings.Repeat("a", 72)} {
		first, err := b.Hash(plain)
		if err != nil {
			t.Fatalf("%q: %v", plain, err)
		}
		second, _ := b.Hash(plain)
		if first == second || strings.Contains(first, plain) {
			t.Fatal("hash is unsalted or contains the password")
		}
		if !b.Matches(first, plain) || b.Matches(first, plain[:len(plain)-1]) {
			t.Fatalf("%q: comparison incorrect", plain)
		}
	}
	// bcrypt only reads 72 bytes; a longer password sharing that prefix must not match.
	prefix := strings.Repeat("a", 72)
	hash, _ := b.Hash(prefix)
	if b.Matches(hash, prefix+"suffix") {
		t.Fatal("password longer than 72 bytes matched its truncated prefix")
	}
	if _, err := b.Hash(prefix + "a"); err == nil {
		t.Fatal("73-byte password hashed with silent truncation")
	}
	for _, bad := range []string{"", "not-a-bcrypt-hash"} {
		if b.Matches(bad, "abcdefgh") {
			t.Errorf("hash %q matched", bad)
		}
	}
}

func TestGeneratedPasswordsSatisfyPolicyAndDiffer(t *testing.T) {
	b, err := NewBcrypt(bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for range 100 {
		p, err := b.Generate()
		if err != nil {
			t.Fatal(err)
		}
		if err := domain.ValidatePassword(p); err != nil || len(p) < 20 || seen[p] {
			t.Fatalf("generated %q: %v", p, err)
		}
		seen[p] = true
	}
}
