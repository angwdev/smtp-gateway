package limiter

import (
	"context"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

const idleTTL = 10 * time.Minute

// PerIP is a token bucket per client address. Idle buckets are swept so the
// map cannot grow without bound.
type PerIP struct {
	mu      sync.Mutex
	limit   rate.Limit
	burst   int
	entries map[string]*bucket
}

type bucket struct {
	limiter *rate.Limiter
	seen    time.Time
}

// NewPerIP allows limit events per second with the given burst, per address.
func NewPerIP(limit rate.Limit, burst int) *PerIP {
	return &PerIP{limit: limit, burst: burst, entries: map[string]*bucket{}}
}

// Allow reports whether ip may proceed now.
func (p *PerIP) Allow(ip string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	b := p.entries[ip]
	if b == nil {
		b = &bucket{limiter: rate.NewLimiter(p.limit, p.burst)}
		p.entries[ip] = b
	}
	b.seen = time.Now()
	return b.limiter.Allow()
}

// Sweep drops buckets idle since before now - idleTTL.
func (p *PerIP) Sweep(now time.Time) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for ip, b := range p.entries {
		if now.Sub(b.seen) > idleTTL {
			delete(p.entries, ip)
		}
	}
}

// Len is the number of tracked addresses.
func (p *PerIP) Len() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.entries)
}

// Run sweeps once a minute until ctx is cancelled.
func (p *PerIP) Run(ctx context.Context) {
	t := time.NewTicker(time.Minute)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-t.C:
			p.Sweep(now)
		}
	}
}
