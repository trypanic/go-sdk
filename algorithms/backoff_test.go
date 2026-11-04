package algorithms

import (
	"math"
	"testing"
	"time"
)

func TestDefaultExponentialBackoffConfig(t *testing.T) {
	c := DefaultExponentialBackoffConfig()
	if c.InitialInterval != time.Second {
		t.Errorf("InitialInterval = %v, want 1s", c.InitialInterval)
	}
	if c.MaxInterval != 30*time.Second {
		t.Errorf("MaxInterval = %v, want 30s", c.MaxInterval)
	}
	if c.Multiplier != 2.0 {
		t.Errorf("Multiplier = %v, want 2.0", c.Multiplier)
	}
	if c.MaxRetries != 5 {
		t.Errorf("MaxRetries = %v, want 5", c.MaxRetries)
	}
}

// MaxRetries is uint64; without the floor, MaxRetries=0 would compute
// MaxRetries-1 and underflow to ~1.8e19 retries. The floor to 1 (=> 0
// effective retries) must keep the backoff bounded, stopping immediately.
func TestNewExponentialBackoffGuardsUnderflow(t *testing.T) {
	const backoffStop = time.Duration(-1)
	for _, retries := range []uint64{0, 1} {
		cfg := DefaultExponentialBackoffConfig()
		cfg.MaxRetries = retries
		b := NewExponentialBackoff(cfg)
		if d := b.NextBackOff(); d != backoffStop {
			t.Fatalf("MaxRetries=%d: first NextBackOff = %v, want Stop (0 retries)", retries, d)
		}
	}
}

// MaxRetries counts total attempts; effective retries = MaxRetries-1.
func TestNewExponentialBackoffExhausts(t *testing.T) {
	const backoffStop = time.Duration(-1)
	cfg := ExponentialBackoffConfig{
		InitialInterval: time.Millisecond,
		MaxInterval:     time.Millisecond,
		Multiplier:      1.0,
		MaxRetries:      3, // WithMaxRetries gets MaxRetries-1 = 2 retries
	}
	b := NewExponentialBackoff(cfg)

	got := 0
	for b.NextBackOff() != backoffStop {
		got++
		if got > 10 {
			t.Fatal("backoff never stopped")
		}
	}
	if got != 2 {
		t.Fatalf("retries before Stop = %d, want 2 (MaxRetries-1)", got)
	}
}

// The factory overrides InitialInterval after cenkalti's constructor has
// already reset the current interval to its own 500ms default; without a
// Reset the first NextBackOff ignores the configured InitialInterval, which
// matters for custom loops that call NextBackOff directly.
func TestNewExponentialBackoffFirstDelayUsesInitialInterval(t *testing.T) {
	const (
		initial = 10 * time.Second
		// cenkalti/backoff default RandomizationFactor.
		randomization = 0.5
	)
	lo := time.Duration(float64(initial) * (1 - randomization))
	hi := time.Duration(float64(initial) * (1 + randomization))

	for range 20 {
		b := NewExponentialBackoff(ExponentialBackoffConfig{
			InitialInterval: initial,
			MaxInterval:     time.Minute,
			Multiplier:      2,
			MaxRetries:      3,
		})
		d := b.NextBackOff()
		if d < lo || d > hi {
			t.Fatalf("first NextBackOff = %v, want within [%v, %v]", d, lo, hi)
		}
	}
}

// collectDelays drains b and returns every delay before Stop, failing the
// test if it does not stop within limit calls.
func collectDelays(t *testing.T, b interface{ NextBackOff() time.Duration }, limit int) []time.Duration {
	t.Helper()
	const backoffStop = time.Duration(-1)
	var delays []time.Duration
	for {
		d := b.NextBackOff()
		if d == backoffStop {
			return delays
		}
		delays = append(delays, d)
		if len(delays) > limit {
			t.Fatalf("backoff did not stop within %d delays", limit)
		}
	}
}

// A zero or negative interval and a multiplier below 1 used to be passed
// through, so the schedule degenerated into 0s (or shrinking) delays: a
// retry loop without waiting.
func TestNewExponentialBackoffSubstitutesDefaultsForInvalidValues(t *testing.T) {
	cases := []struct {
		name string
		cfg  ExponentialBackoffConfig
	}{
		{"zero config", ExponentialBackoffConfig{MaxRetries: 5}},
		{"negative values", ExponentialBackoffConfig{
			InitialInterval: -time.Second,
			MaxInterval:     -time.Second,
			Multiplier:      -2,
			MaxRetries:      5,
		}},
		{"multiplier below one", ExponentialBackoffConfig{
			InitialInterval: time.Second,
			MaxInterval:     30 * time.Second,
			Multiplier:      0.5,
			MaxRetries:      5,
		}},
		{"NaN multiplier", ExponentialBackoffConfig{
			InitialInterval: time.Second,
			MaxInterval:     30 * time.Second,
			Multiplier:      math.NaN(),
			MaxRetries:      5,
		}},
	}
	def := DefaultExponentialBackoffConfig()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			delays := collectDelays(t, NewExponentialBackoff(tc.cfg), 10)
			if len(delays) != 4 {
				t.Fatalf("got %d delays, want 4 (MaxRetries-1)", len(delays))
			}
			if delays[0] < def.InitialInterval/2 || delays[0] > def.InitialInterval*3/2 {
				t.Fatalf("first delay = %v, want around the default %v", delays[0], def.InitialInterval)
			}
			// With the default multiplier (2) and randomization 0.5 the
			// 4th delay is drawn from [4s, 12s]; it can never be below the
			// 1st delay's upper bound of 1.5s.
			if delays[3] < def.InitialInterval*3/2 {
				t.Fatalf("delays do not grow: %v", delays)
			}
			for _, d := range delays {
				if d <= 0 || d > def.MaxInterval*3/2 {
					t.Fatalf("delay %v outside (0, %v]: %v", d, def.MaxInterval*3/2, delays)
				}
			}
		})
	}
}

// A MaxInterval left at zero must not cap the schedule below a larger
// configured InitialInterval.
func TestNewExponentialBackoffDefaultMaxIntervalNotBelowInitial(t *testing.T) {
	delays := collectDelays(t, NewExponentialBackoff(ExponentialBackoffConfig{
		InitialInterval: 2 * time.Minute,
		Multiplier:      1,
		MaxRetries:      3,
	}), 10)
	for _, d := range delays {
		if d < time.Minute {
			t.Fatalf("delay %v below InitialInterval/2: %v", d, delays)
		}
	}
}

// MaxRetries is a uint64 that callers fill from int config (uint64(-1)
// wraps to ~1.8e19); it is capped so a bad value cannot retry forever.
func TestNewExponentialBackoffCapsMaxRetries(t *testing.T) {
	for _, retries := range []uint64{MaxRetriesLimit + 1, 1 << 63, ^uint64(0)} {
		delays := collectDelays(t, NewExponentialBackoff(ExponentialBackoffConfig{
			InitialInterval: time.Millisecond,
			MaxInterval:     time.Millisecond,
			Multiplier:      1,
			MaxRetries:      retries,
		}), MaxRetriesLimit+10)
		if len(delays) != MaxRetriesLimit-1 {
			t.Fatalf("MaxRetries=%d: got %d retries, want %d", retries, len(delays), MaxRetriesLimit-1)
		}
	}
}

// The library default MaxElapsedTime (15 min) used to stop long schedules
// before MaxRetries attempts. MaxElapsedTime is now explicit: zero means no
// elapsed-time limit, so MaxRetries alone bounds the schedule.
func TestNewExponentialBackoffMaxElapsedTime(t *testing.T) {
	long := ExponentialBackoffConfig{
		InitialInterval: time.Hour,
		MaxInterval:     time.Hour,
		Multiplier:      1,
		MaxRetries:      3,
	}
	if got := collectDelays(t, NewExponentialBackoff(long), 10); len(got) != 2 {
		t.Fatalf("MaxElapsedTime=0: got %d retries, want 2 (MaxRetries-1): %v", len(got), got)
	}

	limited := long
	limited.MaxElapsedTime = time.Minute
	if got := collectDelays(t, NewExponentialBackoff(limited), 10); len(got) != 0 {
		t.Fatalf("MaxElapsedTime=1m with 1h delays: got %v, want immediate Stop", got)
	}
}
