// Package ratelimit is an in-memory sliding-window limiter for a single API
// instance. Several instances would each keep their own counters.
package ratelimit

import (
	"sync"
	"time"
)

type Limiter struct {
	mu        sync.Mutex
	max       int
	window    time.Duration
	now       func() time.Time
	hits      map[string][]time.Time
	lastSweep time.Time
}

func New(max int, window time.Duration, now func() time.Time) *Limiter {
	return &Limiter{max: max, window: window, now: now, hits: map[string][]time.Time{}, lastSweep: now()}
}

// Allow records a request for key and reports whether it is within the limit.
// Denied requests are not recorded, so they do not extend the window.
func (l *Limiter) Allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	if now.Sub(l.lastSweep) >= l.window {
		l.sweep(now)
		l.lastSweep = now
	}
	recent := l.recent(key, now)
	if len(recent) >= l.max {
		l.store(key, recent)
		return false
	}
	l.hits[key] = append(recent, now)
	return true
}

func (l *Limiter) recent(key string, now time.Time) []time.Time {
	hits := l.hits[key]
	i := 0
	for i < len(hits) && now.Sub(hits[i]) >= l.window {
		i++
	}
	return hits[i:]
}

func (l *Limiter) store(key string, recent []time.Time) {
	if len(recent) == 0 {
		delete(l.hits, key)
		return
	}
	l.hits[key] = recent
}

// sweep forgets keys without requests in the window, bounding memory by the
// number of recently active keys.
func (l *Limiter) sweep(now time.Time) {
	for key := range l.hits {
		l.store(key, l.recent(key, now))
	}
}
