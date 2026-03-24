package github

import (
	"sync"
	"time"
)

// RateLimiter implements a simple token-bucket rate limiter for GitHub API calls.
type RateLimiter struct {
	mu       sync.Mutex
	rate     float64 // tokens per second
	tokens   float64
	maxBurst float64
	lastTime time.Time
}

// NewRateLimiter creates a rate limiter with the given queries-per-second limit.
func NewRateLimiter(qps int) *RateLimiter {
	burst := float64(qps)
	if burst < 1 {
		burst = 1
	}
	return &RateLimiter{
		rate:     float64(qps),
		tokens:   burst,
		maxBurst: burst,
		lastTime: time.Now(),
	}
}

// Wait blocks until a token is available.
func (rl *RateLimiter) Wait() {
	for {
		rl.mu.Lock()
		now := time.Now()
		elapsed := now.Sub(rl.lastTime).Seconds()
		rl.tokens += elapsed * rl.rate
		if rl.tokens > rl.maxBurst {
			rl.tokens = rl.maxBurst
		}
		rl.lastTime = now

		if rl.tokens >= 1 {
			rl.tokens--
			rl.mu.Unlock()
			return
		}

		// Calculate wait time for next token
		waitFor := time.Duration((1 - rl.tokens) / rl.rate * float64(time.Second))
		rl.mu.Unlock()
		time.Sleep(waitFor)
	}
}
