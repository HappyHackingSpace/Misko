// Package domain holds the installation's single laboratory.
package domain

import (
	"errors"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

var (
	ErrInvalidName     = errors.New("laboratory name must contain 1 to 120 printable characters")
	ErrInvalidCode     = errors.New("laboratory code must contain up to 32 ASCII letters, digits, '.', '_' or '-'")
	ErrInvalidTimezone = errors.New("timezone must be an IANA time zone name")
)

// Laboratory is a singleton per installation. Code is optional; empty means unset.
type Laboratory struct {
	Name      string
	Code      string
	Timezone  string
	CreatedAt time.Time
	UpdatedAt time.Time
}

func NewLaboratory(name, code, timezone string) (Laboratory, error) {
	var lab Laboratory
	var err error
	if lab.Name, err = NormalizeName(name); err != nil {
		return Laboratory{}, err
	}
	if lab.Code, err = NormalizeCode(code); err != nil {
		return Laboratory{}, err
	}
	if err = ValidateTimezone(timezone); err != nil {
		return Laboratory{}, err
	}
	lab.Timezone = timezone
	return lab, nil
}

func NormalizeName(raw string) (string, error) {
	name := strings.TrimSpace(raw)
	if name == "" || !utf8.ValidString(name) || utf8.RuneCountInString(name) > 120 || strings.IndexFunc(name, unicode.IsControl) >= 0 {
		return "", ErrInvalidName
	}
	return name, nil
}

func NormalizeCode(raw string) (string, error) {
	code := strings.TrimSpace(raw)
	if len(code) > 32 {
		return "", ErrInvalidCode
	}
	for _, r := range code {
		if r > unicode.MaxASCII || !(unicode.IsLetter(r) || unicode.IsDigit(r) || r == '.' || r == '_' || r == '-') {
			return "", ErrInvalidCode
		}
	}
	return code, nil
}

// ValidateTimezone accepts IANA names only. The empty name and "Local" are
// rejected because they depend on the host rather than the laboratory.
func ValidateTimezone(name string) error {
	if name == "" || name == "Local" {
		return ErrInvalidTimezone
	}
	if _, err := time.LoadLocation(name); err != nil {
		return ErrInvalidTimezone
	}
	return nil
}
