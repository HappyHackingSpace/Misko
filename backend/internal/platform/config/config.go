package config

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	DatabaseURL     string
	HTTPAddr        string
	ProbeTimeout    time.Duration
	ShutdownTimeout time.Duration
}

// Load reads explicit process configuration; it never reads a local .env file.
func Load(getenv func(string) string) (Config, error) {
	c := Config{DatabaseURL: getenv("DATABASE_URL"), HTTPAddr: getenv("HTTP_ADDR")}
	u, err := url.Parse(c.DatabaseURL)
	if err != nil || u == nil || (u.Scheme != "postgres" && u.Scheme != "postgresql") || u.Hostname() == "" || strings.Trim(u.Path, "/") == "" {
		return Config{}, errors.New("DATABASE_URL must be a PostgreSQL URL with host and database")
	}
	if c.HTTPAddr == "" {
		c.HTTPAddr = ":4000"
	}
	_, port, err := net.SplitHostPort(c.HTTPAddr)
	p, portErr := strconv.Atoi(port)
	if err != nil || portErr != nil || p < 1 || p > 65535 {
		return Config{}, errors.New("HTTP_ADDR must contain a host and port from 1 to 65535")
	}
	c.ProbeTimeout, err = duration(getenv, "PROBE_TIMEOUT", 2*time.Second)
	if err != nil {
		return Config{}, err
	}
	c.ShutdownTimeout, err = duration(getenv, "SHUTDOWN_TIMEOUT", 10*time.Second)
	if err != nil {
		return Config{}, err
	}
	return c, nil
}

type Auth struct {
	JWTSecret   []byte
	JWTIssuer   string
	JWTAudience string
	TokenTTL    time.Duration
	BcryptCost  int
}

// LoadAuth reads API authentication settings. Errors never echo secret values.
func LoadAuth(getenv func(string) string) (Auth, error) {
	a := Auth{
		JWTSecret:   []byte(getenv("JWT_SECRET")),
		JWTIssuer:   withDefault(getenv("JWT_ISSUER"), "misko"),
		JWTAudience: withDefault(getenv("JWT_AUDIENCE"), "misko-api"),
	}
	if len(a.JWTSecret) < 32 {
		return Auth{}, errors.New("JWT_SECRET must contain at least 32 bytes")
	}
	var err error
	if a.TokenTTL, err = duration(getenv, "TOKEN_TTL", 12*time.Hour); err != nil {
		return Auth{}, err
	}
	if a.TokenTTL < 5*time.Minute || a.TokenTTL > 7*24*time.Hour {
		return Auth{}, errors.New("TOKEN_TTL must be between 5m and 168h")
	}
	if a.BcryptCost, err = bcryptCost(getenv); err != nil {
		return Auth{}, err
	}
	return a, nil
}

// Setup configures the one-time installation of the laboratory and first administrator.
type Setup struct {
	AdminEmail  string
	LabName     string
	LabTimezone string
	BcryptCost  int
}

func LoadSetup(getenv func(string) string) (Setup, error) {
	s := Setup{AdminEmail: getenv("ADMIN_EMAIL"), LabName: getenv("LAB_NAME"), LabTimezone: withDefault(getenv("LAB_TIMEZONE"), "UTC")}
	if strings.TrimSpace(s.AdminEmail) == "" || strings.TrimSpace(s.LabName) == "" {
		return Setup{}, errors.New("ADMIN_EMAIL and LAB_NAME are required for setup")
	}
	var err error
	if s.BcryptCost, err = bcryptCost(getenv); err != nil {
		return Setup{}, err
	}
	return s, nil
}

func bcryptCost(getenv func(string) string) (int, error) {
	raw := getenv("BCRYPT_COST")
	if raw == "" {
		return 12, nil
	}
	cost, err := strconv.Atoi(raw)
	if err != nil || cost < 10 || cost > 14 {
		return 0, errors.New("BCRYPT_COST must be an integer from 10 to 14")
	}
	return cost, nil
}

func withDefault(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

func duration(getenv func(string) string, key string, fallback time.Duration) (time.Duration, error) {
	raw := getenv(key)
	if raw == "" {
		return fallback, nil
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d <= 0 {
		return 0, fmt.Errorf("%s must be a positive duration", key)
	}
	return d, nil
}
