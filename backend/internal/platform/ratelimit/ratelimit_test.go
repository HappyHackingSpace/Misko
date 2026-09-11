package ratelimit

import (
	"sync"
	"testing"
	"time"
)

func TestSlidingWindowPerKey(t *testing.T) {
	now := time.Date(2026, 9, 11, 12, 0, 0, 0, time.UTC)
	l := New(3, time.Minute, func() time.Time { return now })
	for i := range 3 {
		if !l.Allow("alice") {
			t.Fatalf("request %d denied", i+1)
		}
		now = now.Add(10 * time.Second)
	}
	if l.Allow("alice") {
		t.Fatal("fourth request within the window allowed")
	}
	if !l.Allow("bob") {
		t.Fatal("keys must be limited independently")
	}
	// The first request left the window 60 s after it was made.
	now = now.Add(29 * time.Second)
	if l.Allow("alice") {
		t.Fatal("allowed before the oldest request left the window")
	}
	now = now.Add(time.Second)
	if !l.Allow("alice") {
		t.Fatal("denied after the oldest request left the window")
	}
	// A denied request does not extend the window.
	if l.Allow("alice") {
		t.Fatal("window full again")
	}
}

func TestIdleKeysAreForgotten(t *testing.T) {
	now := time.Date(2026, 9, 11, 12, 0, 0, 0, time.UTC)
	l := New(1, time.Minute, func() time.Time { return now })
	for _, key := range []string{"a", "b", "c"} {
		l.Allow(key)
	}
	now = now.Add(2 * time.Minute)
	l.Allow("d")
	l.mu.Lock()
	defer l.mu.Unlock()
	if len(l.hits) != 1 {
		t.Fatalf("idle keys kept: %d", len(l.hits))
	}
}

func TestConcurrentRequestsNeverExceedTheLimit(t *testing.T) {
	l := New(20, time.Minute, time.Now)
	var wg sync.WaitGroup
	var mu sync.Mutex
	allowed := 0
	for range 100 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if l.Allow("same-user") {
				mu.Lock()
				allowed++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if allowed != 20 {
		t.Fatalf("allowed %d of 100 concurrent requests, want 20", allowed)
	}
}
