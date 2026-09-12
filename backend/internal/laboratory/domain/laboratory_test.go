package domain

import (
	"errors"
	"strings"
	"testing"
)

func TestNewLaboratoryNormalizesValidInput(t *testing.T) {
	lab, err := NewLaboratory("  Happy Hacking Lab ", " HHS-01 ", "Europe/Istanbul")
	if err != nil {
		t.Fatal(err)
	}
	if lab.Name != "Happy Hacking Lab" || lab.Code != "HHS-01" || lab.Timezone != "Europe/Istanbul" {
		t.Fatalf("got %+v", lab)
	}
	if lab, err := NewLaboratory("Lab", "", "UTC"); err != nil || lab.Code != "" {
		t.Fatalf("optional code rejected: %+v %v", lab, err)
	}
}

func TestLaboratoryRejectsInvalidFields(t *testing.T) {
	for _, tc := range []struct {
		name, labName, code, timezone string
		want                          error
	}{
		{"empty name", "  ", "", "UTC", ErrInvalidName},
		{"long name", strings.Repeat("a", 121), "", "UTC", ErrInvalidName},
		{"control in name", "Lab\nName", "", "UTC", ErrInvalidName},
		{"code with space", "Lab", "HH S", "UTC", ErrInvalidCode},
		{"long code", "Lab", strings.Repeat("A", 33), "UTC", ErrInvalidCode},
		{"non-ascii code", "Lab", "Ş01", "UTC", ErrInvalidCode},
		{"empty timezone", "Lab", "", "", ErrInvalidTimezone},
		{"host-local timezone", "Lab", "", "Local", ErrInvalidTimezone},
		{"unknown timezone", "Lab", "", "Mars/Olympus_Mons", ErrInvalidTimezone},
		{"path traversal", "Lab", "", "../../etc/passwd", ErrInvalidTimezone},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := NewLaboratory(tc.labName, tc.code, tc.timezone); !errors.Is(err, tc.want) {
				t.Fatalf("err=%v want %v", err, tc.want)
			}
		})
	}
}
