package github_test

import (
	"testing"
	"time"

	ghub "github.com/shivamstaq/github-symphony/internal/github"
)

func TestRateLimiter_AllowsWithinRate(t *testing.T) {
	rl := ghub.NewRateLimiter(100) // 100 QPS

	start := time.Now()
	for i := 0; i < 10; i++ {
		rl.Wait()
	}
	elapsed := time.Since(start)

	// 10 requests at 100 QPS should take < 200ms
	if elapsed > 200*time.Millisecond {
		t.Errorf("expected fast execution at 100 QPS, took %v", elapsed)
	}
}

func TestRateLimiter_ThrottlesOverRate(t *testing.T) {
	rl := ghub.NewRateLimiter(10) // 10 QPS

	start := time.Now()
	for i := 0; i < 15; i++ {
		rl.Wait()
	}
	elapsed := time.Since(start)

	// 15 requests at 10 QPS should take at least ~400ms (burst + throttle)
	if elapsed < 300*time.Millisecond {
		t.Errorf("expected throttling at 10 QPS with 15 requests, but only took %v", elapsed)
	}
}
