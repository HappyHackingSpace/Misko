// Package httpjson holds JSON request and response helpers shared by HTTP
// adapters. It handles protocol details only; adapters supply error mappings.
package httpjson

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
)

const MaxBodyBytes = 1 << 20

var (
	ErrInvalidJSON  = errors.New("request body must be one JSON object with known fields")
	ErrBodyTooLarge = errors.New("request body is too large")
)

// Decode reads exactly one JSON value into dst and rejects unknown fields.
func Decode(w http.ResponseWriter, r *http.Request, dst any) error {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, MaxBodyBytes))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		return bodyError(err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return bodyError(err)
	}
	return nil
}

func bodyError(err error) error {
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		return ErrBodyTooLarge
	}
	if err == nil {
		return ErrInvalidJSON
	}
	return fmt.Errorf("%w: %v", ErrInvalidJSON, err)
}

func Write(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

type problem struct {
	Error string `json:"error"`
	Code  string `json:"code"`
}

// Error writes a stable machine-readable code with an English fallback message.
func Error(w http.ResponseWriter, status int, code, message string) {
	Write(w, status, problem{Error: message, Code: code})
}

// Failure maps a sentinel error to a status and stable code. The sentinel's own
// text is the client message, so wrapped details never reach the client.
type Failure struct {
	Err    error
	Status int
	Code   string
}

var bodyFailures = []Failure{
	{ErrInvalidJSON, http.StatusBadRequest, "common.invalidJSON"},
	{ErrBodyTooLarge, http.StatusRequestEntityTooLarge, "common.bodyTooLarge"},
}

// Fail writes the first matching failure. Unmatched errors are logged with the
// route pattern only, never headers or bodies, and answered with a generic 500.
func Fail(w http.ResponseWriter, r *http.Request, logger *slog.Logger, err error, failures []Failure) {
	for _, f := range append(bodyFailures, failures...) {
		if errors.Is(err, f.Err) {
			if f.Status == http.StatusUnauthorized {
				w.Header().Set("WWW-Authenticate", "Bearer")
			}
			Error(w, f.Status, f.Code, f.Err.Error())
			return
		}
	}
	logger.ErrorContext(r.Context(), "request failed", "route", r.Pattern, "error", err)
	Error(w, http.StatusInternalServerError, "common.serverError", "internal server error")
}

// QueryInt returns 0 for an absent value and -1 for a malformed or negative one,
// which use cases reject after checking authorization.
func QueryInt(raw string) int {
	if raw == "" {
		return 0
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 0 {
		return -1
	}
	return n
}
