package config

import (
	"strings"
	"testing"
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
