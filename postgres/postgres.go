package postgres

import (
	"context"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/trypanic/go-sdk/errorkit"
)

// Config configures NewPostgresClient. Zero pool-tuning fields keep the pgx
// defaults.
type Config struct {
	// DSN is a postgres:// or postgresql:// URL (for example
	// "postgres://user:pass@localhost:5432/mydb") or a key/value string.
	// Required.
	DSN string
	// ConnectTimeout bounds initial connection setup; zero uses 5s only when ctx has no deadline.
	ConnectTimeout  time.Duration
	MaxConnLifetime time.Duration // e.g. time.Hour
	MaxConnIdleTime time.Duration // e.g. time.Minute
	MaxConns        int32
	MinConns        int32
}

// PostgresEnvVar is the canonical environment variable name read by
// NewPostgresClientFromEnv. SDK consumers that want a different variable name
// should read it themselves and assign Config.DSN.
const PostgresEnvVar = "POSTGRES_DSN"

// NewPostgresClient creates a new pgx connection pool from an explicit Config.
// It does NOT read any environment variable — callers that want env-based
// fallback should use NewPostgresClientFromEnv.
func NewPostgresClient(cfg Config) (*pgxpool.Pool, error) {
	return NewPostgresClientContext(context.Background(), cfg)
}

// NewPostgresClientContext creates and verifies a pool using ctx for setup only.
// The returned pool belongs to the caller; canceling ctx later does not close it.
// Without a deadline, setup defaults to five seconds. ConnectTimeout, when
// positive, caps setup even when the caller has a later deadline.
func NewPostgresClientContext(ctx context.Context, cfg Config) (*pgxpool.Pool, error) {
	if err := ctx.Err(); err != nil {
		return nil, poolError("connect to database", err)
	}
	if cfg.ConnectTimeout < 0 {
		return nil, errorkit.NewError(errorkit.ERR_SYSTEM_CONFIG_INVALID).With(errorkit.WithReason("ConnectTimeout must not be negative"))
	}

	if cfg.DSN == "" {
		return nil, errorkit.NewError(errorkit.ERR_SYSTEM_CONFIG_INVALID).With(
			errorkit.WithReason("missing DSN"),
		)
	}

	poolCfg, err := pgxpool.ParseConfig(normalizePostgresDSN(cfg.DSN))
	if err != nil {
		return nil, poolError("parse config", err)
	}

	// Pool tuning. Only override pgx's parsed defaults when the caller
	// supplied a non-zero value; a zero MaxConns would make pgxpool reject
	// the config ("MaxConns must be greater than 0").
	if cfg.MaxConnLifetime > 0 {
		poolCfg.MaxConnLifetime = cfg.MaxConnLifetime
	}
	if cfg.MaxConnIdleTime > 0 {
		poolCfg.MaxConnIdleTime = cfg.MaxConnIdleTime
	}
	if cfg.MaxConns > 0 {
		poolCfg.MaxConns = cfg.MaxConns
	}
	if cfg.MinConns > 0 {
		poolCfg.MinConns = cfg.MinConns
	}

	// Connect with timeout for production reliability
	timeout := cfg.ConnectTimeout
	if _, hasDeadline := ctx.Deadline(); !hasDeadline && timeout == 0 {
		timeout = 5 * time.Second
	}
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, poolError("connect to database", err)
	}

	// Verify connection
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, poolError("ping database", err)
	}

	return pool, nil
}

// NewPostgresClientFromEnv reads POSTGRES_DSN and forwards the rest of cfg
// (pool tuning fields) to NewPostgresClient. Returns ERR_SYSTEM_CONFIG_INVALID
// if the env var is unset or empty.
func NewPostgresClientFromEnv(cfg Config) (*pgxpool.Pool, error) {
	return NewPostgresClientFromEnvContext(context.Background(), cfg)
}

// NewPostgresClientFromEnvContext reads POSTGRES_DSN and uses ctx during setup.
func NewPostgresClientFromEnvContext(ctx context.Context, cfg Config) (*pgxpool.Pool, error) {
	cfg.DSN = os.Getenv(PostgresEnvVar)
	if cfg.DSN == "" {
		return nil, errorkit.NewError(errorkit.ERR_SYSTEM_CONFIG_INVALID).With(
			errorkit.WithReason("%s env var is unset or empty", PostgresEnvVar),
		)
	}
	return NewPostgresClientContext(ctx, cfg)
}

// poolError builds the error returned by the pool constructors. The driver
// error is classified and attached with WithWrapped so errors.Is/errors.As
// keep working, and the payload carries only its Error() text, which pgx
// redacts. The error value itself is never used as payload:
// *pgconn.ParseConfigError exports the unredacted connection string and
// *pgconn.ConnectError cannot be marshaled.
func poolError(reason string, err error) *errorkit.AppError {
	return classifyPgError(err, reason).With(errorkit.WithPayload(err.Error()))
}

// normalizePostgresDSN percent-encodes the user and password of a
// postgres:// or postgresql:// URL so that raw '@', '/', '?' and '#'
// characters in the password parse. Other DSN forms are returned unchanged.
//
// The userinfo ends at the last '@' before the first '/', '?' or '#' that
// follows the first '@'. When no '@' appears before the first '/', '?' or
// '#' and that prefix already reads as host[:port] (for example
// "db:5432/app@x", where '@' belongs to the database name), the DSN is left
// unchanged. The function is idempotent.
func normalizePostgresDSN(dsn string) string {
	scheme, rest, found := strings.Cut(dsn, "://")
	if !found {
		return dsn
	}
	scheme = strings.ToLower(scheme)
	if scheme != "postgres" && scheme != "postgresql" {
		return dsn
	}
	authorityStart := len(dsn) - len(rest)

	firstAt := strings.IndexByte(rest, '@')
	if firstAt == -1 {
		return dsn
	}
	plainAuthority := rest
	if end := strings.IndexAny(rest, "/?#"); end != -1 {
		plainAuthority = rest[:end]
	}
	if !strings.Contains(plainAuthority, "@") && looksLikePort(plainAuthority) {
		return dsn
	}

	hostEnd := len(rest)
	if end := strings.IndexAny(rest[firstAt:], "/?#"); end != -1 {
		hostEnd = firstAt + end
	}
	userInfoEnd := strings.LastIndexByte(rest[:hostEnd], '@')

	userInfo := rest[:userInfoEnd]
	user, password, hasPassword := strings.Cut(userInfo, ":")
	if strings.ContainsAny(user, "/?#") {
		// Not a userinfo: the '@' belongs to the path or the query.
		return dsn
	}

	user = unescapeURLComponent(user)
	if hasPassword {
		password = unescapeURLComponent(password)
		userInfo = url.UserPassword(user, password).String()
	} else {
		userInfo = url.User(user).String()
	}

	return dsn[:authorityStart] + userInfo + rest[userInfoEnd:]
}

// looksLikePort reports whether authority reads as host[:port]: no ':' at
// all, an IPv6 literal ending in ']', or a numeric port after the last ':'.
func looksLikePort(authority string) bool {
	if strings.HasSuffix(authority, "]") {
		return true
	}
	colon := strings.LastIndexByte(authority, ':')
	if colon == -1 {
		return true
	}
	port := authority[colon+1:]
	if port == "" {
		return false
	}
	for i := range len(port) {
		if port[i] < '0' || port[i] > '9' {
			return false
		}
	}
	return true
}

func unescapeURLComponent(value string) string {
	unescaped, err := url.PathUnescape(value)
	if err != nil {
		return value
	}

	return unescaped
}
