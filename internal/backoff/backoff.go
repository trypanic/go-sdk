// Package backoff implements the shared full-jitter transport retry schedule.
package backoff

import (
	"math/rand/v2"
	"time"
)

// Stop marks an exhausted schedule.
const Stop time.Duration = -1

// Config bounds retries after the first operation attempt.
type Config struct {
	Initial        time.Duration
	Max            time.Duration
	Multiplier     float64
	Attempts       int
	MaxElapsedTime time.Duration
}

// Schedule owns mutable state for one retry loop; do not share it concurrently.
type Schedule struct {
	config    Config
	cap       time.Duration
	remaining int
	started   time.Time
}

// New creates a schedule with safe defaults and at most 1000 total attempts.
func New(config Config) *Schedule {
	if config.Initial <= 0 {
		config.Initial = time.Second
	}
	if config.Max <= 0 {
		config.Max = max(30*time.Second, config.Initial)
	}
	if !(config.Multiplier >= 1) {
		config.Multiplier = 2
	}
	config.Attempts = min(max(config.Attempts, 1), 1000)
	s := &Schedule{config: config}
	s.Reset()
	return s
}

// Reset starts a new operation budget.
func (s *Schedule) Reset() {
	s.cap = min(s.config.Initial, s.config.Max)
	s.remaining = s.config.Attempts - 1
	s.started = time.Now()
}

// NextBackOff returns a full-jitter delay, or Stop when an attempt/time budget ends.
func (s *Schedule) NextBackOff() time.Duration {
	if s.remaining <= 0 {
		return Stop
	}
	delay := Jitter(s.cap)
	if budget := s.config.MaxElapsedTime; budget > 0 && (time.Since(s.started) >= budget || delay > budget-time.Since(s.started)) {
		return Stop
	}
	s.remaining--
	s.cap = Grow(s.cap, s.config.Max, s.config.Multiplier)
	return delay
}

// Jitter samples [0, cap) without overflowing the duration range.
// The randomness spreads load and is not used for any security decision.
func Jitter(cap time.Duration) time.Duration {
	if cap <= 0 {
		return 0
	}
	return time.Duration(rand.Int64N(int64(cap)))
}

// Grow multiplies a cap, saturating before duration conversion can overflow.
func Grow(current, limit time.Duration, multiplier float64) time.Duration {
	if current <= 0 || limit <= 0 {
		return 0
	}
	if !(multiplier >= 1) {
		multiplier = 2
	}
	if current >= limit || float64(current) >= float64(limit)/multiplier {
		return limit
	}
	return min(time.Duration(float64(current)*multiplier), limit)
}
