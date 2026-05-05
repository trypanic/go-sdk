package httpclient

import (
	"crypto/tls"
	"net"
	"net/http"
	"time"

	"github.com/trypanic/go-sdk/errorkit"
)

// Default configuration values for production use
const (
	DefaultTimeout               = 60 * time.Second
	DefaultKeepAlive             = 60 * time.Second
	DefaultMaxIdleConns          = 500
	DefaultMaxIdleConnsPerHost   = 50
	DefaultMaxConnsPerHost       = 500
	DefaultIdleConnTimeout       = 120 * time.Second
	DefaultTLSHandshakeTimeout   = 10 * time.Second
	DefaultExpectContinueTimeout = 1 * time.Second
	DefaultResponseHeaderTimeout = 10 * time.Second
	DefaultMaxRedirects          = 10
)

// ClientConfig holds HTTP client configuration options.
type ClientConfig struct {
	// Timeouts
	Timeout               time.Duration // Overall request timeout; 0 disables the client-level deadline
	DialTimeout           time.Duration // TCP dial timeout
	KeepAlive             time.Duration // Keep-alive probe interval
	TLSHandshakeTimeout   time.Duration // TLS handshake timeout
	ResponseHeaderTimeout time.Duration // Time to wait for response headers
	ExpectContinueTimeout time.Duration // Time to wait for 100-continue response

	// Connection pooling
	MaxIdleConns        int // Maximum idle connections across all hosts
	MaxIdleConnsPerHost int // Maximum idle connections per host
	MaxConnsPerHost     int // Maximum total connections per host
	IdleConnTimeout     time.Duration

	// Features
	DisableCompression bool // Disable gzip compression
	DisableKeepAlives  bool // Disable HTTP keep-alives
	MaxRedirects       int  // Maximum number of redirects to follow (0 = no redirects)

	// TLS configuration
	InsecureSkipVerify bool        // Skip TLS certificate verification (NOT recommended for production)
	TLSConfig          *tls.Config // Custom TLS configuration; takes precedence over InsecureSkipVerify when non-nil

	// TransportWrapper, when non-nil, wraps the base *http.Transport before
	// it is installed on the *http.Client. This is the opt-in seam for
	// tracing/metrics middleware (e.g. otelhttp.NewTransport). The default
	// is no wrapper, so the SDK has no telemetry import in its baseline.
	TransportWrapper func(http.RoundTripper) http.RoundTripper
}

// newDefaultConfig returns a production-ready default configuration.
func newDefaultConfig() *ClientConfig {
	return &ClientConfig{
		Timeout:               DefaultTimeout,
		DialTimeout:           DefaultTimeout,
		KeepAlive:             DefaultKeepAlive,
		TLSHandshakeTimeout:   DefaultTLSHandshakeTimeout,
		ResponseHeaderTimeout: DefaultResponseHeaderTimeout,
		ExpectContinueTimeout: DefaultExpectContinueTimeout,
		MaxIdleConns:          DefaultMaxIdleConns,
		MaxIdleConnsPerHost:   DefaultMaxIdleConnsPerHost,
		MaxConnsPerHost:       DefaultMaxConnsPerHost,
		IdleConnTimeout:       DefaultIdleConnTimeout,
		DisableCompression:    false,
		DisableKeepAlives:     false,
		MaxRedirects:          DefaultMaxRedirects,
		InsecureSkipVerify:    false,
	}
}

func DefaultConfig() *ClientConfig {
	return newDefaultConfig()
}

// SetupForLLM returns a configuration optimized for LLM API interactor.
// Suitable for OpenAI, Anthropic, Google AI, and other LLM providers.
//
// Key optimizations:
// - Long timeout for generation (120s default, can take 60-120s for complex prompts)
// - Long response header timeout (30s, some LLMs are slow to start streaming)
// - Moderate connection pool (not too aggressive to respect rate limits)
// - Keep-alive enabled for streaming responses
// - Compression enabled for large prompts/responses
func SetupForLLM() *ClientConfig {
	return &ClientConfig{
		Timeout:               0, // Streaming and long generations should be bounded by caller contexts.
		DialTimeout:           10 * time.Second,
		KeepAlive:             90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 4 * time.Minute, // Allow model warmup / queue
		ExpectContinueTimeout: 2 * time.Second,
		MaxIdleConns:          50,
		MaxIdleConnsPerHost:   10,
		MaxConnsPerHost:       5,
		IdleConnTimeout:       120 * time.Second,
		DisableCompression:    false,
		DisableKeepAlives:     false,
		MaxRedirects:          5,
		InsecureSkipVerify:    false,
	}
}

// ClientOption is a functional option for configuring the HTTP client.
type ClientOption func(*ClientConfig)

// WithTimeout sets the overall http.Client timeout. A zero timeout disables
// the client-level deadline and leaves cancellation to request contexts.
func WithTimeout(timeout time.Duration) ClientOption {
	return func(c *ClientConfig) {
		c.Timeout = timeout
	}
}

// WithDialTimeout sets the TCP dial timeout.
func WithDialTimeout(timeout time.Duration) ClientOption {
	return func(c *ClientConfig) {
		c.DialTimeout = timeout
	}
}

// WithKeepAlive sets the keep-alive probe interval.
func WithKeepAlive(keepAlive time.Duration) ClientOption {
	return func(c *ClientConfig) {
		c.KeepAlive = keepAlive
	}
}

// WithMaxIdleConns sets the maximum idle connections across all hosts.
func WithMaxIdleConns(max int) ClientOption {
	return func(c *ClientConfig) {
		c.MaxIdleConns = max
	}
}

// WithMaxIdleConnsPerHost sets the maximum idle connections per host.
func WithMaxIdleConnsPerHost(max int) ClientOption {
	return func(c *ClientConfig) {
		c.MaxIdleConnsPerHost = max
	}
}

// WithMaxConnsPerHost sets the maximum total connections per host.
func WithMaxConnsPerHost(max int) ClientOption {
	return func(c *ClientConfig) {
		c.MaxConnsPerHost = max
	}
}

// WithIdleConnTimeout sets the idle connection timeout.
func WithIdleConnTimeout(timeout time.Duration) ClientOption {
	return func(c *ClientConfig) {
		c.IdleConnTimeout = timeout
	}
}

// WithTLSHandshakeTimeout sets the TLS handshake timeout.
func WithTLSHandshakeTimeout(timeout time.Duration) ClientOption {
	return func(c *ClientConfig) {
		c.TLSHandshakeTimeout = timeout
	}
}

// WithResponseHeaderTimeout sets the response header timeout.
func WithResponseHeaderTimeout(timeout time.Duration) ClientOption {
	return func(c *ClientConfig) {
		c.ResponseHeaderTimeout = timeout
	}
}

// WithMaxRedirects sets the maximum number of redirects to follow.
func WithMaxRedirects(max int) ClientOption {
	return func(c *ClientConfig) {
		c.MaxRedirects = max
	}
}

// WithDisableCompression disables gzip compression.
func WithDisableCompression(disable bool) ClientOption {
	return func(c *ClientConfig) {
		c.DisableCompression = disable
	}
}

// WithDisableKeepAlives disables HTTP keep-alives.
func WithDisableKeepAlives(disable bool) ClientOption {
	return func(c *ClientConfig) {
		c.DisableKeepAlives = disable
	}
}

// WithInsecureSkipVerify skips TLS certificate verification (NOT recommended for production).
func WithInsecureSkipVerify(skip bool) ClientOption {
	return func(c *ClientConfig) {
		c.InsecureSkipVerify = skip
	}
}

// WithTLSConfig sets a custom TLS configuration.
func WithTLSConfig(tlsConfig *tls.Config) ClientOption {
	return func(c *ClientConfig) {
		c.TLSConfig = tlsConfig
	}
}

// WithTransportWrapper installs an opt-in middleware around the base transport
// (e.g. otelhttp.NewTransport). The wrapper receives the configured
// *http.Transport and returns the http.RoundTripper used by the client.
func WithTransportWrapper(wrap func(http.RoundTripper) http.RoundTripper) ClientOption {
	return func(c *ClientConfig) {
		c.TransportWrapper = wrap
	}
}

// NewClient creates a new HTTP client with the given configuration.
func NewClient(config *ClientConfig) *http.Client {
	if config == nil {
		config = newDefaultConfig()
	}

	dialer := &net.Dialer{
		Timeout:   config.DialTimeout,
		KeepAlive: config.KeepAlive,
	}

	tlsConfig := config.TLSConfig
	if tlsConfig == nil {
		tlsConfig = &tls.Config{
			MinVersion:         tls.VersionTLS13,
			InsecureSkipVerify: config.InsecureSkipVerify,
		}
	}

	baseTransport := &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           dialer.DialContext,
		MaxIdleConns:          config.MaxIdleConns,
		MaxIdleConnsPerHost:   config.MaxIdleConnsPerHost,
		MaxConnsPerHost:       config.MaxConnsPerHost,
		IdleConnTimeout:       config.IdleConnTimeout,
		TLSHandshakeTimeout:   config.TLSHandshakeTimeout,
		ResponseHeaderTimeout: config.ResponseHeaderTimeout,
		ExpectContinueTimeout: config.ExpectContinueTimeout,
		DisableCompression:    config.DisableCompression,
		DisableKeepAlives:     config.DisableKeepAlives,
		TLSClientConfig:       tlsConfig,
		ForceAttemptHTTP2:     true,
	}

	var rt http.RoundTripper = baseTransport
	if config.TransportWrapper != nil {
		rt = config.TransportWrapper(baseTransport)
	}

	client := &http.Client{
		Transport: rt,
		Timeout:   config.Timeout,
	}

	// redirect policy unchanged...
	if config.MaxRedirects > 0 {
		maxRedirects := config.MaxRedirects
		client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
			if len(via) >= maxRedirects {
				return errorkit.NewError(ERR_EXTERNAL_SERVICE_ERROR).With(
					errorkit.WithReason("stopped after %d redirects", maxRedirects),
				)
			}
			return nil
		}
	} else if config.MaxRedirects == 0 {
		client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		}
	}

	return client
}

// NewClientWithOptions creates a new HTTP client with functional options.
// Starts with default configuration and applies the given options.
func NewClientWithOptions(opts ...ClientOption) *http.Client {
	config := newDefaultConfig()
	for _, opt := range opts {
		opt(config)
	}
	return NewClient(config)
}

func NewDefaultClient() *http.Client {
	return NewClient(DefaultConfig())
}
