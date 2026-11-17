// Package limiter holds the in-memory limiters shared by the SMTP and HTTP interfaces.
package limiter

import (
	"sync"
	"time"
)

// FailLimiter blocks an IP after too many failures within a window.
type FailLimiter struct {
	mu     sync.Mutex
	limit  int
	window time.Duration
	hits   map[string]*failEntry
	last   time.Time
}

type failEntry struct {
	count int
	first time.Time
}

// NewFailLimiter returns a limiter that blocks an IP after limit failures
// within window. limit <= 0 disables blocking.
func NewFailLimiter(limit int, window time.Duration) *FailLimiter {
	return &FailLimiter{limit: limit, window: window, hits: map[string]*failEntry{}}
}

// Blocked reports whether ip has reached the failure limit within the
// current window.
func (l *FailLimiter) Blocked(ip string) bool {
	if l.limit <= 0 {
		return false
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	e := l.hits[ip]
	if e == nil {
		return false
	}
	if time.Since(e.first) > l.window {
		delete(l.hits, ip)
		return false
	}
	return e.count >= l.limit
}

// Fail counts a failure for ip and reports true exactly when this failure
// reaches the limit, so the caller can record one lock-out event per window.
func (l *FailLimiter) Fail(ip string) bool {
	if l.limit <= 0 {
		return false
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	if now.Sub(l.last) > l.window { // opportunistic cleanup keeps the map small
		for k, e := range l.hits {
			if now.Sub(e.first) > l.window {
				delete(l.hits, k)
			}
		}
		l.last = now
	}
	e := l.hits[ip]
	if e == nil || now.Sub(e.first) > l.window {
		e = &failEntry{first: now}
		l.hits[ip] = e
	}
	e.count++
	return e.count == l.limit
}

// Reset clears any recorded failures for ip (a successful login).
func (l *FailLimiter) Reset(ip string) {
	l.mu.Lock()
	delete(l.hits, ip)
	l.mu.Unlock()
}
