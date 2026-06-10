package retry

import (
	"context"
	"sync"
	"time"
)

// Backoff manages exponential backoff state for reconnection or retry logic.
// It supports both non-blocking scenarios (via Try/Fail) and blocking scenarios (via Wait).
type Backoff struct {
	mu         sync.Mutex
	initial    time.Duration
	max        time.Duration
	multiplier float64

	current   time.Duration
	nextRetry time.Time
}

// NewBackoff creates a new Backoff with the given initial and max durations.
// An optional multiplier can be provided; if omitted it defaults to 2.0.
// The multiplier must be greater than 1.0.
func NewBackoff(initial, max time.Duration, multiplier ...float64) *Backoff {
	m := 2.0
	if len(multiplier) > 0 && multiplier[0] > 1.0 {
		m = multiplier[0]
	}
	return &Backoff{
		initial:    initial,
		max:        max,
		multiplier: m,
		current:    initial,
	}
}

// Try reports whether a reconnection attempt is allowed right now.
// It does not advance the backoff state; call Fail after a failed attempt.
func (b *Backoff) Try() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return !time.Now().Before(b.nextRetry)
}

// Fail records a failed reconnection attempt and advances the backoff.
// The next call to Try will return false until the backoff duration has elapsed.
func (b *Backoff) Fail() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.nextRetry = time.Now().Add(b.current)
	b.advance()
}

// Wait blocks until the current backoff duration has elapsed or ctx is canceled.
// It advances the backoff for the next call.
func (b *Backoff) Wait(ctx context.Context) error {
	b.mu.Lock()
	d := b.current
	b.advance()
	b.mu.Unlock()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(d):
		return nil
	}
}

// Reset restores the backoff to its initial state.
// Call this after a successful connection.
func (b *Backoff) Reset() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.current = b.initial
	b.nextRetry = time.Time{}
}

// advance grows the current backoff by the multiplier, capped at max.
// Must be called with b.mu held.
func (b *Backoff) advance() {
	b.current = time.Duration(float64(b.current) * b.multiplier)
	if b.current > b.max {
		b.current = b.max
	}
}
