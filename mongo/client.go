package mongodb

import (
	"context"
	"strings"
	"time"

	"github.com/trypanic/go-sdk/errorkit"
	"github.com/trypanic/go-sdk/telemetry"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.mongodb.org/mongo-driver/v2/mongo/readpref"
)

// Config configures New, NewWithoutTracing and NewWithInstrumenter. A zero
// field means "not set": the option then comes from the URI, else from the
// SDK default (see clientOptions and the README "Defaults").
type Config struct {
	// URI is the connection string; a missing mongodb:// scheme, a leading
	// '=' and surrounding quotes are corrected.
	URI string
	// Database names the database that Collection uses.
	Database               string
	ConnectTimeout         time.Duration
	ServerSelectionTimeout time.Duration
	MaxPoolSize            uint64
	MinPoolSize            uint64
	MaxConnIdleTime        time.Duration
	// RetryWrites controls the MongoDB driver's retryWrites option. nil means
	// "not set": retryWrites from the URI, else the SDK default (true). Pass a
	// pointer to false to disable retry writes, or to true to force them on.
	RetryWrites *bool
}

// Client is the untraced ClientPort returned by NewWithoutTracing (and
// wrapped by New). It is safe for concurrent use.
type Client struct {
	client   *mongo.Client
	database *mongo.Database
}

// New creates a production-ready MongoDB client with OTEL instrumentation
// using the global tracer. Equivalent to NewWithInstrumenter(ctx, cfg, telemetry.NewInstrumenter(...)).
func New(ctx context.Context, cfg Config) (ClientPort, error) {
	return NewWithInstrumenter(ctx, cfg, telemetry.NewInstrumenter(telemetry.InstrumenterConfig{}))
}

// NewWithoutTracing creates a MongoDB client with no tracing wrapper. SDK
// consumers can wrap it later via WrapWithInstrumenter or use NewWithInstrumenter.
func NewWithoutTracing(ctx context.Context, cfg Config) (ClientPort, error) {
	return NewWithInstrumenter(ctx, cfg, nil)
}

// NewWithInstrumenter creates a MongoDB client wrapped with the supplied
// instrumenter. Pass nil to disable tracing entirely.
func NewWithInstrumenter(ctx context.Context, cfg Config, instrumenter *telemetry.Instrumenter) (ClientPort, error) {
	clientOpts := clientOptions(cfg)

	// mongo.Connect performs no I/O: it fails only when the URI or the
	// options are invalid, which retrying cannot fix.
	mongoClient, err := mongo.Connect(clientOpts)
	if err != nil {
		return nil, errorkit.NewError(errorkit.ERR_SYSTEM_CONFIG_INVALID).With(
			errorkit.WithReason("mongo client options are invalid"),
			errorkit.WithWrapped(err),
		)
	}

	pingCtx, cancel := context.WithTimeout(ctx, startupPingTimeout(clientOpts))
	defer cancel()

	if err := mongoClient.Ping(pingCtx, readpref.Primary()); err != nil {
		// Bounded clean-up that still runs when ctx is already cancelled.
		disconnectCtx, cancelDisconnect := context.WithTimeout(context.WithoutCancel(ctx), cleanupTimeout)
		defer cancelDisconnect()
		_ = mongoClient.Disconnect(disconnectCtx)
		return nil, wrapUnavailable(err, "ping")
	}

	plain := &Client{
		client:   mongoClient,
		database: mongoClient.Database(cfg.Database),
	}
	return WrapWithInstrumenter(plain, instrumenter), nil
}

// Database returns the underlying mongo.Database.
func (c *Client) Database() *mongo.Database {
	return c.database
}

// Collection returns an instrumented, error-normalizing collection adapter.
func (c *Client) Collection(name string) Collection {
	return collectionAdapter{collection: c.database.Collection(name)}
}

// Ping allows readiness/liveness checks.
func (c *Client) Ping(ctx context.Context) error {
	if err := c.client.Ping(ctx, readpref.Primary()); err != nil {
		return WrapOperationError(err, "ping")
	}
	return nil
}

// Close gracefully disconnects the Mongo client.
func (c *Client) Close(ctx context.Context) error {
	if err := c.client.Disconnect(ctx); err != nil {
		return WrapOperationError(err, "disconnect")
	}
	return nil
}

// SDK defaults, used only when neither Config nor the URI sets the option.
const (
	defaultConnectTimeout                = 10 * time.Second
	defaultServerSelectionTimeout        = 5 * time.Second
	defaultMaxPoolSize            uint64 = 100
	defaultMinPoolSize            uint64 = 10
	defaultMaxConnIdleTime               = 30 * time.Second
	defaultRetryWrites                   = true
)

// minStartupPingTimeout is the floor of the start-up ping budget, and
// cleanupTimeout bounds the Disconnect after a failed ping.
const (
	minStartupPingTimeout = 5 * time.Second
	cleanupTimeout        = 5 * time.Second
)

// startupPingTimeout gives the start-up ping at least the effective server
// selection timeout, so a slow cluster configured with a larger
// ServerSelectionTimeout is not cut at 5 s. A deadline on the caller's ctx
// still applies when it is earlier.
func startupPingTimeout(opts *options.ClientOptions) time.Duration {
	if opts.ServerSelectionTimeout == nil {
		return minStartupPingTimeout
	}
	return max(minStartupPingTimeout, *opts.ServerSelectionTimeout)
}

// clientOptions builds the driver options with this precedence per option:
// a non-zero Config field, else the value in the URI, else the SDK default.
// The default MinPoolSize is clamped to the effective MaxPoolSize so that a
// small pool validates.
func clientOptions(cfg Config) *options.ClientOptions {
	opts := options.Client().ApplyURI(normalizeURI(cfg.URI))

	opts.ConnectTimeout = pick(cfg.ConnectTimeout, opts.ConnectTimeout, defaultConnectTimeout)
	opts.ServerSelectionTimeout = pick(cfg.ServerSelectionTimeout, opts.ServerSelectionTimeout, defaultServerSelectionTimeout)
	opts.MaxConnIdleTime = pick(cfg.MaxConnIdleTime, opts.MaxConnIdleTime, defaultMaxConnIdleTime)
	opts.MaxPoolSize = pick(cfg.MaxPoolSize, opts.MaxPoolSize, defaultMaxPoolSize)
	opts.MinPoolSize = pick(cfg.MinPoolSize, opts.MinPoolSize, defaultMinPool(*opts.MaxPoolSize))

	switch {
	case cfg.RetryWrites != nil:
		opts.RetryWrites = new(*cfg.RetryWrites)
	case opts.RetryWrites == nil:
		opts.RetryWrites = new(defaultRetryWrites)
	}
	return opts
}

// pick returns the Config value when it is non-zero, else the value parsed
// from the URI when present, else the SDK default.
func pick[V comparable](configured V, fromURI *V, fallback V) *V {
	var zero V
	switch {
	case configured != zero:
		return &configured
	case fromURI != nil:
		return fromURI
	default:
		return &fallback
	}
}

// defaultMinPool is defaultMinPoolSize clamped to maxPool; 0 means the pool
// has no maximum.
func defaultMinPool(maxPool uint64) uint64 {
	if maxPool > 0 && maxPool < defaultMinPoolSize {
		return maxPool
	}
	return defaultMinPoolSize
}

func wrapUnavailable(err error, op string) error {
	return errorkit.
		NewError(ERR_DB_MONGO_UNAVAILABLE).
		With(
			errorkit.WithReason("mongo %s failed", op),
			errorkit.WithWrapped(err),
		)
}

func normalizeURI(uri string) string {
	normalized := strings.TrimSpace(uri)
	normalized = strings.TrimLeft(normalized, "=")
	normalized = strings.TrimSpace(normalized)
	normalized = trimMatchingQuotes(normalized)

	switch {
	case normalized == "":
		return normalized
	case strings.HasPrefix(normalized, "mongodb://"), strings.HasPrefix(normalized, "mongodb+srv://"):
		return normalized
	case strings.Contains(normalized, "://"):
		return normalized
	default:
		return "mongodb://" + normalized
	}
}

func trimMatchingQuotes(value string) string {
	if len(value) < 2 {
		return value
	}

	if value[0] == '"' && value[len(value)-1] == '"' {
		return value[1 : len(value)-1]
	}

	if value[0] == '\'' && value[len(value)-1] == '\'' {
		return value[1 : len(value)-1]
	}

	return value
}
