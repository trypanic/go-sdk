package algorithms

import (
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
