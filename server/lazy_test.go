package server

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// fakeClock lets the retry window be exercised without sleeping.
type fakeClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *fakeClock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *fakeClock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

func newFakeClock() *fakeClock {
	// A fixed instant rather than time.Now, so nothing here depends on the wall
	// clock.
	return &fakeClock{t: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
}

func TestLazyValueComputesOnce(t *testing.T) {
	var calls atomic.Int32
	l := &lazyValue[string]{}

	for i := 0; i < 5; i++ {
		got, err := l.get(context.Background(), func(context.Context) (string, error) {
			calls.Add(1)
			return "value", nil
		})
		if err != nil || got != "value" {
			t.Fatalf("get = %q, %v", got, err)
		}
	}
	if n := calls.Load(); n != 1 {
		t.Errorf("computed %d times, want once", n)
	}
}

// The whole reason this type exists. sync.Once caches a failure permanently, so a
// single unreachable moment on the first attempt meant nothing ever retried and
// the process had to be restarted to recover. This test fails against that
// implementation.
func TestLazyValueRetriesAFailure(t *testing.T) {
	clock := newFakeClock()
	l := &lazyValue[string]{retryAfter: time.Minute, now: clock.now}
	boom := errors.New("upstream unreachable")

	var calls atomic.Int32
	compute := func(context.Context) (string, error) {
		if calls.Add(1) == 1 {
			return "", boom
		}
		return "value", nil
	}

	if _, err := l.get(context.Background(), compute); !errors.Is(err, boom) {
		t.Fatalf("first get err = %v, want the upstream failure", err)
	}

	clock.advance(2 * time.Minute)

	got, err := l.get(context.Background(), compute)
	if err != nil {
		t.Fatalf("second get err = %v, want a retry that succeeds", err)
	}
	if got != "value" {
		t.Errorf("get = %q, want the retried value", got)
	}
}

// Several devices opening the app at once must not each hit a dead upstream.
func TestLazyValueDoesNotRetryInsideTheWindow(t *testing.T) {
	clock := newFakeClock()
	l := &lazyValue[string]{retryAfter: time.Minute, now: clock.now}
	boom := errors.New("upstream unreachable")

	var calls atomic.Int32
	compute := func(context.Context) (string, error) {
		calls.Add(1)
		return "", boom
	}

	for i := 0; i < 4; i++ {
		if _, err := l.get(context.Background(), compute); !errors.Is(err, boom) {
			t.Fatalf("get %d err = %v", i, err)
		}
		clock.advance(10 * time.Second)
	}

	if n := calls.Load(); n != 1 {
		t.Errorf("attempted %d times inside the window, want once", n)
	}
}

// A success is never recomputed, whatever the clock does. Callers build source
// identifiers out of these values and other state points at them, so a value that
// changed under a live session would orphan every reference to it.
func TestLazyValueNeverRecomputesASuccess(t *testing.T) {
	clock := newFakeClock()
	l := &lazyValue[string]{retryAfter: time.Millisecond, now: clock.now}

	var calls atomic.Int32
	compute := func(context.Context) (string, error) {
		calls.Add(1)
		return "value", nil
	}

	if _, err := l.get(context.Background(), compute); err != nil {
		t.Fatalf("first get: %v", err)
	}
	clock.advance(24 * time.Hour)
	if _, err := l.get(context.Background(), compute); err != nil {
		t.Fatalf("second get: %v", err)
	}

	if n := calls.Load(); n != 1 {
		t.Errorf("computed %d times, want once even long past the retry window", n)
	}
}

// A caller that gave up must not suppress attempts for the next one: a cancelled
// context says nothing about whether the upstream works.
func TestLazyValueDoesNotCacheACancellation(t *testing.T) {
	clock := newFakeClock()
	l := &lazyValue[string]{retryAfter: time.Hour, now: clock.now}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := l.get(ctx, func(ctx context.Context) (string, error) {
		return "", ctx.Err()
	}); err == nil {
		t.Fatal("cancelled get returned no error")
	}

	// No clock advance: a recorded failure would still be inside the window.
	got, err := l.get(context.Background(), func(context.Context) (string, error) {
		return "value", nil
	})
	if err != nil {
		t.Fatalf("get after a cancellation: %v", err)
	}
	if got != "value" {
		t.Errorf("get = %q, want the value", got)
	}
}

// Concurrent first callers wait for the in-flight attempt rather than starting
// their own. Run under -race.
func TestLazyValueComputesOnceUnderConcurrency(t *testing.T) {
	var calls atomic.Int32
	l := &lazyValue[string]{}

	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = l.get(context.Background(), func(context.Context) (string, error) {
				calls.Add(1)
				time.Sleep(time.Millisecond)
				return "value", nil
			})
		}()
	}
	wg.Wait()

	if n := calls.Load(); n != 1 {
		t.Errorf("computed %d times concurrently, want once", n)
	}
}
