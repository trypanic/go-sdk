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

type Config struct {
	URI                    string
	Database               string
	ConnectTimeout         time.Duration
	ServerSelectionTimeout time.Duration
	MaxPoolSize            uint64
	MinPoolSize            uint64
	MaxConnIdleTime        time.Duration
	// RetryWrites controls the MongoDB driver's retryWrites option. nil means
	// "use the SDK default" (true). Pass a pointer to false to explicitly
	// disable retry writes; pass a pointer to true to be explicit. The previous
	// bool default was ambiguous because false could mean "I want it off" or
	// "I never set it".
	RetryWrites *bool
}

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
	applyDefaults(&cfg)

	clientOpts := options.Client().
		ApplyURI(normalizeURI(cfg.URI)).
		SetConnectTimeout(cfg.ConnectTimeout).
		SetServerSelectionTimeout(cfg.ServerSelectionTimeout).
		SetMaxPoolSize(cfg.MaxPoolSize).
		SetMinPoolSize(cfg.MinPoolSize).
		SetMaxConnIdleTime(cfg.MaxConnIdleTime).
		SetRetryWrites(*cfg.RetryWrites)

	mongoClient, err := mongo.Connect(clientOpts)
	if err != nil {
		return nil, wrapUnavailable(err, "connect")
	}

	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if err := mongoClient.Ping(pingCtx, readpref.Primary()); err != nil {
		_ = mongoClient.Disconnect(context.Background())
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

func applyDefaults(cfg *Config) {
	if cfg.ConnectTimeout == 0 {
		cfg.ConnectTimeout = 10 * time.Second
	}
	if cfg.ServerSelectionTimeout == 0 {
		cfg.ServerSelectionTimeout = 5 * time.Second
	}
	if cfg.MaxPoolSize == 0 {
		cfg.MaxPoolSize = 100
	}
	if cfg.MinPoolSize == 0 {
		cfg.MinPoolSize = 10
	}
	if cfg.MaxConnIdleTime == 0 {
		cfg.MaxConnIdleTime = 30 * time.Second
	}

	// Safe production default: enable retry writes unless caller explicitly
	// chose otherwise. nil means "use SDK default".
	if cfg.RetryWrites == nil {
		t := true
		cfg.RetryWrites = &t
	}
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
