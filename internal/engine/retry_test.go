package engine

import (
	"testing"
	"time"
)

func TestRetryDelay_ExponentialBackoff(t *testing.T) {
	tests := []struct {
		attempt int
		maxMs   int
		wantMs  int
	}{
		{1, 300000, 10000},   // 10s
		{2, 300000, 20000},   // 20s
		{3, 300000, 40000},   // 40s
		{4, 300000, 80000},   // 80s
		{5, 300000, 160000},  // 160s
		{6, 300000, 300000},  // capped at 300s
		{7, 300000, 300000},  // still capped
		{1, 5000, 5000},      // capped at low max
	}

	for _, tt := range tests {
		got := RetryDelay(tt.attempt, tt.maxMs)
		wantDur := time.Duration(tt.wantMs) * time.Millisecond
		if got != wantDur {
			t.Errorf("RetryDelay(%d, %d) = %v, want %v", tt.attempt, tt.maxMs, got, wantDur)
		}
	}
}

func TestRetryDelay_ZeroMaxUnlimited(t *testing.T) {
	// maxMs=0 means no cap
	got := RetryDelay(10, 0)
	if got < time.Second {
		t.Errorf("expected large delay, got %v", got)
	}
}
