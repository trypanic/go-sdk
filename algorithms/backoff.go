package algorithms

import (
	"time"

	"github.com/cenkalti/backoff/v4"
)

// ExponentialBackoffConfig holds configuration for exponential backoff retry logic
type ExponentialBackoffConfig struct {
	InitialInterval time.Duration
	MaxInterval     time.Duration
	Multiplier      float64
	MaxRetries      uint64
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

// NewExponentialBackoff creates a new exponential backoff instance with the given configuration
func NewExponentialBackoff(config ExponentialBackoffConfig) backoff.BackOff {
	if config.MaxRetries < 1 {
		config.MaxRetries = 1
	}

	expBackoff := backoff.NewExponentialBackOff()
	expBackoff.InitialInterval = config.InitialInterval
	expBackoff.MaxInterval = config.MaxInterval
	expBackoff.Multiplier = config.Multiplier

	// WithMaxRetries expects the number of retries (not attempts)
	// So if MaxRetries is 5, it means 1 initial attempt + 5 retries = 6 total attempts
	// But to match the original behavior of maxRetryAttempts=5, we need MaxRetries-1
	return backoff.WithMaxRetries(expBackoff, config.MaxRetries-1)
}
