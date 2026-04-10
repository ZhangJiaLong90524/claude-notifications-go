//go:build darwin

package notifier

import (
	"testing"
	"time"
)

func TestRetryWindowFocusWithDelays_SucceedsAfterRetries(t *testing.T) {
	t.Helper()

	delays := []time.Duration{10 * time.Millisecond, 20 * time.Millisecond, 30 * time.Millisecond}
	var slept []time.Duration
	calls := 0

	result := retryWindowFocusWithDelays(func() int {
		calls++
		if calls < 3 {
			return 0
		}
		return 1
	}, delays, func(d time.Duration) {
		slept = append(slept, d)
	})

	if result != 1 {
		t.Fatalf("retryWindowFocusWithDelays() = %d, want 1", result)
	}
	if calls != 3 {
		t.Fatalf("expected 3 attempts, got %d", calls)
	}
	if len(slept) != 3 {
		t.Fatalf("expected 3 sleeps, got %d", len(slept))
	}
	for i, want := range delays {
		if slept[i] != want {
			t.Fatalf("sleep %d = %v, want %v", i, slept[i], want)
		}
	}
}

func TestRetryWindowFocusWithDelays_ExhaustsRetries(t *testing.T) {
	t.Helper()

	delays := []time.Duration{10 * time.Millisecond, 20 * time.Millisecond, 30 * time.Millisecond}
	sleeps := 0
	calls := 0

	result := retryWindowFocusWithDelays(func() int {
		calls++
		return 0
	}, delays, func(time.Duration) {
		sleeps++
	})

	if result != 0 {
		t.Fatalf("retryWindowFocusWithDelays() = %d, want 0", result)
	}
	if calls != len(delays) {
		t.Fatalf("expected %d attempts, got %d", len(delays), calls)
	}
	if sleeps != len(delays) {
		t.Fatalf("expected %d sleeps, got %d", len(delays), sleeps)
	}
}

func TestRetryWindowFocusWithDelays_ShortCircuitsOnPermissionError(t *testing.T) {
	t.Helper()

	delays := []time.Duration{10 * time.Millisecond, 20 * time.Millisecond, 30 * time.Millisecond}
	sleeps := 0
	calls := 0

	result := retryWindowFocusWithDelays(func() int {
		calls++
		return -1
	}, delays, func(time.Duration) {
		sleeps++
	})

	if result != -1 {
		t.Fatalf("retryWindowFocusWithDelays() = %d, want -1", result)
	}
	if calls != 1 {
		t.Fatalf("expected 1 attempt, got %d", calls)
	}
	if sleeps != 1 {
		t.Fatalf("expected 1 sleep, got %d", sleeps)
	}
}

func TestRetryWindowFocusWithDelays_RestoredOnLastAttemptGetsExtraRetry(t *testing.T) {
	t.Helper()

	delays := []time.Duration{10 * time.Millisecond, 20 * time.Millisecond, 30 * time.Millisecond}
	var slept []time.Duration
	calls := 0

	result := retryWindowFocusWithDelays(func() int {
		calls++
		switch calls {
		case 1, 2:
			return 0
		case 3:
			return windowFocusRetryAfterRestore
		case 4:
			return 1
		default:
			t.Fatalf("unexpected extra attempt %d", calls)
			return 0
		}
	}, delays, func(d time.Duration) {
		slept = append(slept, d)
	})

	if result != 1 {
		t.Fatalf("retryWindowFocusWithDelays() = %d, want 1", result)
	}
	if calls != 4 {
		t.Fatalf("expected 4 attempts including post-restore retry, got %d", calls)
	}
	if len(slept) != 4 {
		t.Fatalf("expected 4 sleeps including post-restore retry, got %d", len(slept))
	}
	if slept[3] != delays[len(delays)-1] {
		t.Fatalf("post-restore sleep = %v, want %v", slept[3], delays[len(delays)-1])
	}
}
