package http

import (
	"testing"
	"time"
)

func TestRateLimiterAllowsWithinBudget(t *testing.T) {
	t.Parallel()

	rl := NewRateLimiter(3, 3, time.Minute)

	current := time.Unix(0, 0)
	rl.now = func() time.Time {
		return current
	}

	key := "1.2.3.4"

	for i := 0; i < 3; i++ {
		if !rl.Allow(key) {
			t.Fatalf("expected request %d to be allowed", i+1)
		}
	}

	if rl.Allow(key) {
		t.Fatalf("expected fourth request to be denied")
	}

	current = current.Add(time.Second)

	if !rl.Allow(key) {
		t.Fatalf("expected request after refill to be allowed")
	}
}
