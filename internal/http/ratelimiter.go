package http

import (
	"sync"
	"time"
)

type rateLimiterClient struct {
	tokens   float64
	last     time.Time
	lastSeen time.Time
}

// RateLimiter implements a simple token bucket limiter keyed by client identifier.
type RateLimiter struct {
	mu         sync.Mutex
	clients    map[string]*rateLimiterClient
	maxTokens  float64
	refillRate float64
	ttl        time.Duration
	now        func() time.Time
}

// NewRateLimiter constructs a rate limiter with the provided settings.
func NewRateLimiter(maxTokens int, refillPerSecond float64, ttl time.Duration) *RateLimiter {
	rl := &RateLimiter{
		clients:    make(map[string]*rateLimiterClient),
		maxTokens:  float64(maxTokens),
		refillRate: refillPerSecond,
		ttl:        ttl,
		now:        time.Now,
	}

	if ttl > 0 {
		ticker := time.NewTicker(ttl)
		go func() {
			for range ticker.C {
				rl.pruneStale()
			}
		}()
	}

	return rl
}

// Allow consumes a token for the provided key if possible.
func (rl *RateLimiter) Allow(key string) bool {
	if key == "" {
		key = "unknown"
	}

	now := rl.now()

	rl.mu.Lock()
	defer rl.mu.Unlock()

	client, ok := rl.clients[key]
	if !ok {
		client = &rateLimiterClient{
			tokens:   rl.maxTokens,
			last:     now,
			lastSeen: now,
		}
		rl.clients[key] = client
	}

	elapsed := now.Sub(client.last).Seconds()
	if elapsed > 0 {
		client.tokens += elapsed * rl.refillRate
		if client.tokens > rl.maxTokens {
			client.tokens = rl.maxTokens
		}
		client.last = now
	}

	if client.tokens < 1 {
		client.lastSeen = now
		return false
	}

	client.tokens -= 1
	client.lastSeen = now
	return true
}

func (rl *RateLimiter) pruneStale() {
	if rl.ttl <= 0 {
		return
	}

	now := rl.now()

	rl.mu.Lock()
	defer rl.mu.Unlock()

	for key, client := range rl.clients {
		if now.Sub(client.lastSeen) > rl.ttl {
			delete(rl.clients, key)
		}
	}
}
