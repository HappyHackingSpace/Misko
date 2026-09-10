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
