package fetcher

import (
	"context"
	"net/url"
	"sync"
	"time"
)

type FetchPolicy struct {
	Interval   time.Duration
	WaitSteps  int
	StepsWait  time.Duration
	RetryWait  time.Duration
	MaxRetries int
}

type HostRateLimiter struct {
	mu    sync.Mutex
	hosts map[string]*hostState
}

type hostState struct {
	counter     int
	lastRequest time.Time
	nextAllowed time.Time
}

func NewHostRateLimiter() *HostRateLimiter {
	return &HostRateLimiter{hosts: map[string]*hostState{}}
}

func (l *HostRateLimiter) Wait(ctx context.Context, rawURL string, policy FetchPolicy) error {
	wait := l.reserve(rawURL, policy)
	if wait <= 0 {
		return nil
	}

	timer := time.NewTimer(wait)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func (l *HostRateLimiter) reserve(rawURL string, policy FetchPolicy) time.Duration {
	host := hostKey(rawURL)
	now := time.Now()
	stepsWait := maxDuration(policy.StepsWait, policy.Interval)

	l.mu.Lock()
	defer l.mu.Unlock()

	state := l.hosts[host]
	if state == nil {
		state = &hostState{}
		l.hosts[host] = state
	}

	if !state.lastRequest.IsZero() && now.Sub(state.lastRequest) > stepsWait && !state.nextAllowed.After(now) {
		state.counter = 0
		state.lastRequest = time.Time{}
		state.nextAllowed = time.Time{}
	}

	allowedAt := state.nextAllowed
	if allowedAt.IsZero() || allowedAt.Before(now) {
		allowedAt = now
	}

	state.counter++
	state.lastRequest = allowedAt
	state.nextAllowed = allowedAt.Add(delayAfterRequest(state.counter, policy, stepsWait))

	if allowedAt.After(now) {
		return allowedAt.Sub(now)
	}
	return 0
}

func delayAfterRequest(counter int, policy FetchPolicy, stepsWait time.Duration) time.Duration {
	if policy.WaitSteps > 0 && counter%policy.WaitSteps == 0 && counter >= policy.WaitSteps {
		return stepsWait
	}
	if counter > 0 {
		return policy.Interval
	}
	return 0
}

func hostKey(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Host == "" {
		return "__global__"
	}
	return parsed.Host
}

func maxDuration(left time.Duration, right time.Duration) time.Duration {
	if left > right {
		return left
	}
	return right
}
