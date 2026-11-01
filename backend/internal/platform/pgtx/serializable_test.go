package pgtx

import (
	"testing"
	"time"
)

func TestBackoffGrowsAndStaysCapped(t *testing.T) {
	for _, tc := range []struct {
		attempt int
		ceiling time.Duration
	}{
		{1, 5 * time.Millisecond},
		{2, 10 * time.Millisecond},
		{4, 40 * time.Millisecond},
		{6, 160 * time.Millisecond},
		{7, maxBackoff},
		{attempts, maxBackoff},
		{100, maxBackoff},
	} {
		for range 200 {
			if d := backoff(tc.attempt); d < time.Millisecond || d > time.Millisecond+tc.ceiling {
				t.Fatalf("attempt %d: backoff %v outside [1ms, %v]", tc.attempt, d, time.Millisecond+tc.ceiling)
			}
		}
	}
}
