package server

import (
	"context"
	"sync"
	"time"
)

// discoveryRetryAfter bounds how often a failed lazy computation is retried.
//
// The two failure modes it sits between: a page load from several devices at once
// must not become one upstream request per device, and a media server that has
// come back must be picked up without anyone restarting the container. Thirty
// seconds is short enough that a restart is never the remedy and long enough that
// a household opening the app together produces one request.
const discoveryRetryAfter = 30 * time.Second

// lazyValue computes a value the first time it is needed, keeps a success for the
// life of the process, and retries a failure no more often than retryAfter.
//
// It exists because two things in this application need exactly that shape —
// discovering a Plex movie section, and resolving a library named rather than
// identified — and because the obvious implementation of both is wrong in the same
// way. sync.Once runs its function exactly once, so it caches a *failure*
// permanently: one unreachable moment on the first attempt and nothing ever
// retries, which presents days later as "it worked yesterday and returns nothing
// today" with the explaining log line long since scrolled past.
//
// The asymmetry between success and failure is deliberate and load-bearing:
//
//   - A success is never recomputed. A recomputed value can differ, and callers
//     build identifiers out of these results that other state already points at.
//     Recomputing a library identifier orphans the fetcher every card in a live
//     session refers to, and every poster starts returning 404 mid-swipe.
//   - A failure is retried, because the reason for it is almost always temporary
//     and the alternative is a process that has to be restarted to recover.
type lazyValue[T any] struct {
	mu       sync.Mutex
	value    T
	ok       bool
	lastErr  error
	failedAt time.Time

	// retryAfter is the floor between attempts after a failure. Zero means
	// discoveryRetryAfter.
	retryAfter time.Duration
	// now is the clock, so the retry window is testable without sleeping.
	now func() time.Time
}

// get returns the value, computing it if necessary.
//
// The lock is held across compute so concurrent callers wait for the in-flight
// attempt rather than starting their own. That is heavier than a channel-based
// singleflight and correct for this use: these calls are a network round trip
// apart at worst, never a hot path, and the simpler thing is the one that stays
// right when someone edits it.
func (l *lazyValue[T]) get(ctx context.Context, compute func(context.Context) (T, error)) (T, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.ok {
		return l.value, nil
	}

	clock := l.now
	if clock == nil {
		clock = time.Now
	}
	window := l.retryAfter
	if window == 0 {
		window = discoveryRetryAfter
	}

	// Inside the retry window, report the previous failure without another
	// attempt. Without this, every request against a dead upstream becomes its own
	// request against a dead upstream.
	if l.lastErr != nil && clock().Sub(l.failedAt) < window {
		var zero T
		return zero, l.lastErr
	}

	value, err := compute(ctx)
	if err != nil {
		// A caller that gave up must not poison the value for the next one. A
		// cancelled context says nothing about whether the upstream works, so it
		// is reported without being recorded — otherwise one abandoned request
		// would suppress attempts for the whole retry window.
		if ctx.Err() != nil {
			var zero T
			return zero, err
		}
		l.lastErr = err
		l.failedAt = clock()
		var zero T
		return zero, err
	}

	l.value = value
	l.ok = true
	l.lastErr = nil
	return value, nil
}
