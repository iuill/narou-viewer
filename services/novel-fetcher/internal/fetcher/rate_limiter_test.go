package fetcher

import (
	"context"
	"testing"
	"time"
)

func TestHostRateLimiterKeepsHostsIndependent(t *testing.T) {
	limiter := NewHostRateLimiter()
	policy := FetchPolicy{
		Interval:  40 * time.Millisecond,
		WaitSteps: 0,
		StepsWait: 5 * time.Second,
	}

	if err := limiter.Wait(context.Background(), "https://example.com/1", policy); err != nil {
		t.Fatalf("first wait returned error: %v", err)
	}

	start := time.Now()
	if err := limiter.Wait(context.Background(), "https://other.example/1", policy); err != nil {
		t.Fatalf("other host wait returned error: %v", err)
	}
	if elapsed := time.Since(start); elapsed > 20*time.Millisecond {
		t.Fatalf("different host waited %s, want no meaningful wait", elapsed)
	}

	start = time.Now()
	if err := limiter.Wait(context.Background(), "https://example.com/2", policy); err != nil {
		t.Fatalf("same host wait returned error: %v", err)
	}
	if elapsed := time.Since(start); elapsed < 25*time.Millisecond {
		t.Fatalf("same host waited %s, want at least interval", elapsed)
	}
}

func TestHostRateLimiterKeepsSyosetuAndMiteminIndependent(t *testing.T) {
	limiter := NewHostRateLimiter()
	policy := FetchPolicy{
		Interval:  40 * time.Millisecond,
		WaitSteps: 0,
		StepsWait: 5 * time.Second,
	}

	if err := limiter.Wait(context.Background(), "https://ncode.syosetu.com/n1234ab/1/", policy); err != nil {
		t.Fatalf("syosetu wait returned error: %v", err)
	}

	start := time.Now()
	if err := limiter.Wait(context.Background(), "https://29644.mitemin.net/userpageimage/viewimage/icode/i422674/", policy); err != nil {
		t.Fatalf("mitemin wait returned error: %v", err)
	}
	if elapsed := time.Since(start); elapsed > 20*time.Millisecond {
		t.Fatalf("mitemin waited after syosetu %s, want independent host bucket", elapsed)
	}
}

func TestHostRateLimiterUsesStepWait(t *testing.T) {
	limiter := NewHostRateLimiter()
	policy := FetchPolicy{
		Interval:  5 * time.Millisecond,
		WaitSteps: 2,
		StepsWait: 30 * time.Millisecond,
	}

	if err := limiter.Wait(context.Background(), "https://example.com/1", policy); err != nil {
		t.Fatalf("first wait returned error: %v", err)
	}
	if err := limiter.Wait(context.Background(), "https://example.com/2", policy); err != nil {
		t.Fatalf("second wait returned error: %v", err)
	}

	start := time.Now()
	if err := limiter.Wait(context.Background(), "https://example.com/3", policy); err != nil {
		t.Fatalf("third wait returned error: %v", err)
	}
	if elapsed := time.Since(start); elapsed < 20*time.Millisecond {
		t.Fatalf("step wait elapsed %s, want longer pause", elapsed)
	}
}
