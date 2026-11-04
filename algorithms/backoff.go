package algorithms

import (
	"time"

	"github.com/cenkalti/backoff/v4"
)

// MaxRetriesLimit is the upper bound applied to
// ExponentialBackoffConfig.MaxRetries. Larger values (for example an int -1
// converted to uint64) are capped to it so a bad configuration cannot retry
// effectively forever.
const MaxRetriesLimit = 1000

// ExponentialBackoffConfig holds configuration for exponential backoff retry
// logic. Invalid values are replaced by NewExponentialBackoff; see its
// documentation.
type ExponentialBackoffConfig struct {
	// InitialInterval is the delay before the first retry. Values <= 0 use
	// the default (1s).
	InitialInterval time.Duration
	// MaxInterval caps the delay between retries. Values <= 0 use the
	// default (30s), raised to InitialInterval when that is larger.
	MaxInterval time.Duration
	// Multiplier is the growth factor applied after each retry. Values < 1
	// (or NaN) use the default (2.0); 1 gives a constant delay.
	Multiplier float64
	// MaxRetries is the total number of attempts (initial call + retries).
	// 0 is treated as 1; values above MaxRetriesLimit are capped.
	MaxRetries uint64
	// MaxElapsedTime stops the schedule once the time since the first
	// NextBackOff plus the next delay exceeds it. Zero (the default) means no
	// elapsed-time limit: MaxRetries alone bounds the schedule.
	MaxElapsedTime time.Duration
}

// DefaultExponentialBackoffConfig returns a default configuration for exponential backoff
func DefaultExponentialBackoffConfig() ExponentialBackoffConfig {
	return ExponentialBackoffConfig{
		InitialInterval: 1 * time.Second,
		MaxInterval:     30 * time.Second,
		Multiplier:      2.0,
		MaxRetries:      5,
	}
}

// NewExponentialBackoff creates a new exponential backoff instance with the
// given configuration. It never fails: a non-positive InitialInterval or
// MaxInterval and a Multiplier below 1 are replaced by the values of
// DefaultExponentialBackoffConfig (MaxInterval is never set below
// InitialInterval), MaxRetries is clamped to [1, MaxRetriesLimit], and a
// negative MaxElapsedTime is treated as zero (no elapsed-time limit).
func NewExponentialBackoff(config ExponentialBackoffConfig) backoff.BackOff {
	config = sanitize(config)

	expBackoff := backoff.NewExponentialBackOff()
	expBackoff.InitialInterval = config.InitialInterval
	expBackoff.MaxInterval = config.MaxInterval
	expBackoff.Multiplier = config.Multiplier
	expBackoff.MaxElapsedTime = config.MaxElapsedTime
	// NewExponentialBackOff already called Reset with the library defaults;
	// reset again so the first NextBackOff uses the configured InitialInterval.
	expBackoff.Reset()

	// WithMaxRetries expects the number of retries (not attempts)
	// So if MaxRetries is 5, it means 1 initial attempt + 5 retries = 6 total attempts
	// But to match the original behavior of maxRetryAttempts=5, we need MaxRetries-1
	return backoff.WithMaxRetries(expBackoff, config.MaxRetries-1)
}

// sanitize replaces invalid configuration values as documented on
// NewExponentialBackoff.
func sanitize(config ExponentialBackoffConfig) ExponentialBackoffConfig {
	def := DefaultExponentialBackoffConfig()
	if config.InitialInterval <= 0 {
		config.InitialInterval = def.InitialInterval
	}
	if config.MaxInterval <= 0 {
		config.MaxInterval = max(def.MaxInterval, config.InitialInterval)
	}
	// The negated comparison also catches NaN.
	if !(config.Multiplier >= 1) {
		config.Multiplier = def.Multiplier
	}
	config.MaxRetries = min(max(config.MaxRetries, 1), MaxRetriesLimit)
	config.MaxElapsedTime = max(config.MaxElapsedTime, 0)
	return config
}
