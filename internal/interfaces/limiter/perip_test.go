package limiter

import (
	"context"
	"testing"
	"time"

	"golang.org/x/time/rate"
)

func TestPerIP(t *testing.T) {
	p := NewPerIP(rate.Limit(1), 2)

	if !p.Allow("a") {
		t.Fatal("Allow #1 for a = false")
	}
	if !p.Allow("a") {
		t.Fatal("Allow #2 for a (within burst) = false")
	}
	if p.Allow("a") {
		t.Fatal("Allow #3 for a (over burst) = true")
	}
	if !p.Allow("b") {
		t.Fatal("Allow for a different address = false")
	}
}

func TestPerIPSweep(t *testing.T) {
	p := NewPerIP(rate.Limit(1), 2)
	p.Allow("a")
	p.Allow("b")
	if got := p.Len(); got != 2 {
		t.Fatalf("Len() = %d, want 2", got)
	}

	p.Sweep(time.Now().Add(11 * time.Minute))
	if got := p.Len(); got != 0 {
		t.Fatalf("Len() after Sweep = %d, want 0", got)
	}
}

func TestPerIPRun(t *testing.T) {
	p := NewPerIP(rate.Limit(1), 2)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		p.Run(ctx)
		close(done)
	}()
	cancel()
	select {
	case <-done:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Run did not return within 100ms of cancellation")
	}
}
