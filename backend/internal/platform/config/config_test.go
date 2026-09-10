package config

import (
	"strings"
	"testing"
	"time"
)

func TestLoadRejectsInvalidConfigurationWithoutLeakingSecrets(t *testing.T) {
	for _, tc := range []struct{ name, key, value string }{
		{"missing database", "DATABASE_URL", ""},
		{"wrong scheme", "DATABASE_URL", "https://user:private-password@localhost/db"},
		{"invalid url", "DATABASE_URL", "postgres://user:private-password@%zz/db"},
		{"missing database name", "DATABASE_URL", "postgres://localhost/"},
		{"invalid address", "HTTP_ADDR", "not-an-address"},
		{"invalid port", "HTTP_ADDR", ":70000"},
		{"zero timeout", "SHUTDOWN_TIMEOUT", "0s"},
		{"negative timeout", "PROBE_TIMEOUT", "-1s"},
		{"bad timeout", "PROBE_TIMEOUT", "tomorrow"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			values := map[string]string{"DATABASE_URL": "postgres://localhost/test"}
			values[tc.key] = tc.value
			_, err := Load(func(key string) string { return values[key] })
			if err == nil {
				t.Fatal("accepted invalid configuration")
			}
			if strings.Contains(err.Error(), "private-password") {
				t.Fatal("error leaked database credentials")
			}
		})
	}
}

func TestLoadAuthRejectsWeakSettingsWithoutLeakingSecret(t *testing.T) {
	const secret = "private-signing-secret-0123456789abcdef"
	for _, tc := range []struct{ name, key, value string }{
		{"missing secret", "JWT_SECRET", ""},
		{"short secret", "JWT_SECRET", "private-short-secret"},
		{"ttl too short", "TOKEN_TTL", "1m"},
		{"ttl too long", "TOKEN_TTL", "169h"},
		{"bad ttl", "TOKEN_TTL", "forever"},
		{"cost too low", "BCRYPT_COST", "9"},
		{"cost too high", "BCRYPT_COST", "15"},
		{"bad cost", "BCRYPT_COST", "twelve"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			values := map[string]string{"JWT_SECRET": secret, tc.key: tc.value}
			_, err := LoadAuth(func(key string) string { return values[key] })
			if err == nil {
				t.Fatal("accepted invalid configuration")
			}
			if strings.Contains(err.Error(), "private") {
				t.Fatal("error leaked the signing secret")
			}
		})
	}
	auth, err := LoadAuth(func(key string) string { return map[string]string{"JWT_SECRET": secret}[key] })
	if err != nil {
		t.Fatal(err)
	}
	if string(auth.JWTSecret) != secret || auth.JWTIssuer != "misko" || auth.JWTAudience != "misko-api" || auth.TokenTTL != 12*time.Hour || auth.BcryptCost != 12 {
		t.Fatalf("invalid defaults: %+v", auth)
	}
}

func TestLoadSetup(t *testing.T) {
	for _, missing := range []string{"ADMIN_EMAIL", "LAB_NAME"} {
		values := map[string]string{"ADMIN_EMAIL": "admin@lab.io", "LAB_NAME": "Lab", missing: ""}
		if _, err := LoadSetup(func(key string) string { return values[key] }); err == nil {
			t.Errorf("missing %s accepted", missing)
		}
	}
	values := map[string]string{"ADMIN_EMAIL": "admin@lab.io", "LAB_NAME": "Lab", "BCRYPT_COST": "99"}
	if _, err := LoadSetup(func(key string) string { return values[key] }); err == nil {
		t.Error("invalid bcrypt cost accepted")
	}
	delete(values, "BCRYPT_COST")
	setup, err := LoadSetup(func(key string) string { return values[key] })
	if err != nil || setup.LabTimezone != "UTC" || setup.BcryptCost != 12 || setup.AdminEmail != "admin@lab.io" || setup.LabName != "Lab" {
		t.Fatalf("setup=%+v err=%v", setup, err)
	}
}

func TestLoadValidConfiguration(t *testing.T) {
	cfg, err := Load(func(key string) string {
		if key == "DATABASE_URL" {
			return "postgres://localhost/test?sslmode=disable"
		}
		return ""
	})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.HTTPAddr != ":4000" || cfg.ProbeTimeout <= 0 || cfg.ShutdownTimeout <= 0 {
		t.Fatalf("invalid defaults: %#v", cfg)
	}
}
