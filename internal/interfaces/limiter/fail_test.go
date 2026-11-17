package limiter

import (
	"testing"
	"time"
)

func TestFailLimiter(t *testing.T) {
	l := NewFailLimiter(3, time.Minute)
	if l.Blocked("1.2.3.4") {
		t.Fatal("Blocked = true at start")
	}
	if l.Fail("1.2.3.4") {
		t.Fatal("Fail #1 reported lock-out")
	}
	if l.Fail("1.2.3.4") {
		t.Fatal("Fail #2 reported lock-out")
	}
	if !l.Fail("1.2.3.4") {
		t.Fatal("Fail #3 did not report lock-out")
	}
	if l.Fail("1.2.3.4") {
		t.Fatal("Fail #4 reported lock-out again")
	}
	if !l.Blocked("1.2.3.4") {
		t.Fatal("Blocked = false after reaching the limit")
	}
	l.Reset("1.2.3.4")
	if l.Blocked("1.2.3.4") {
		t.Fatal("Blocked = true after Reset")
	}

	zero := NewFailLimiter(0, time.Minute)
	if zero.Blocked("5.6.7.8") {
		t.Fatal("limit 0: Blocked = true")
	}
	if zero.Fail("5.6.7.8") {
		t.Fatal("limit 0: Fail reported lock-out")
	}
	if zero.Blocked("5.6.7.8") {
		t.Fatal("limit 0: Blocked = true after Fail")
	}
}
