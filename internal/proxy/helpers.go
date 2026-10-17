package proxy

import (
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/emersion/go-sasl"
)

// ---- AUTH LOGIN (go-sasl only ships the client side) -----------------------

type loginServer struct {
	step     int
	username string
	auth     func(username, password string) error
}

func newLoginServer(auth func(username, password string) error) sasl.Server {
	return &loginServer{auth: auth}
}

func (s *loginServer) Next(response []byte) (challenge []byte, done bool, err error) {
	switch s.step {
	case 0:
		s.step++
		if len(response) > 0 { // initial response carries the username
			s.username = string(response)
			s.step++
			return []byte("Password:"), false, nil
		}
		return []byte("Username:"), false, nil
	case 1:
		s.username = string(response)
		s.step++
		return []byte("Password:"), false, nil
	case 2:
		s.step++
		return nil, true, s.auth(s.username, string(response))
	}
	return nil, false, errors.New("unexpected client response")
}

// ---- sender policy ---------------------------------------------------------

// SenderAllowed reports whether addr matches one of the patterns: "*" (any
// address), "user@example.com" (exact), "@example.com" or "*@example.com"
// (any address at that domain) and "@*.example.com" (any subdomain).
//
// An empty pattern list allows everything.
func SenderAllowed(patterns []string, addr string) bool {
	if len(patterns) == 0 {
		return true
	}
	addr = strings.ToLower(strings.TrimSpace(addr))
	at := strings.LastIndexByte(addr, '@')
	if at <= 0 || at == len(addr)-1 {
		return false
	}
	domain := addr[at+1:]
	for _, p := range patterns {
		p = strings.ToLower(strings.TrimSpace(p))
		p = strings.TrimPrefix(p, "*")
		switch {
		case p == "":
			return true // pattern was "*"
		case strings.HasPrefix(p, "@*."):
			if strings.HasSuffix(domain, p[2:]) {
				return true
			}
		case strings.HasPrefix(p, "@"):
			if domain == p[1:] {
				return true
			}
		case p == addr:
			return true
		}
	}
	return false
}

// ---- brute-force protection ------------------------------------------------

type failLimiter struct {
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

func newFailLimiter(limit int, window time.Duration) *failLimiter {
	return &failLimiter{limit: limit, window: window, hits: map[string]*failEntry{}}
}

func (l *failLimiter) Blocked(ip string) bool {
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

func (l *failLimiter) Fail(ip string) {
	if l.limit <= 0 {
		return
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
}

func (l *failLimiter) Reset(ip string) {
	l.mu.Lock()
	delete(l.hits, ip)
	l.mu.Unlock()
}
