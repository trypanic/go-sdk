package database

import (
	"context"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/trypanic/go-sdk/errorkit"
)

type Config struct {
	DSN             string        // Example: database://user:pass@localhost:5432/mydb
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
	if cfg.DSN == "" {
		return nil, errorkit.NewError(errorkit.ERR_SYSTEM_CONFIG_INVALID).With(
			errorkit.WithReason("missing DSN"),
		)
	}

	poolCfg, err := pgxpool.ParseConfig(normalizePostgresDSN(cfg.DSN))
	if err != nil {
		return nil, errorkit.NewError(ERR_DB_POSTGRES_ERROR).With(
			errorkit.WithReason("parse config"),
			errorkit.WithPayload(err),
		)
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
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, errorkit.NewError(ERR_DB_POSTGRES_ERROR).With(
			errorkit.WithReason("connect to database"),
			errorkit.WithPayload(err),
		)
	}

	// Verify connection
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, errorkit.NewError(ERR_DB_POSTGRES_ERROR).With(
			errorkit.WithReason("ping database"),
			errorkit.WithPayload(err),
		)
	}

	return pool, nil
}

// NewPostgresClientFromEnv reads POSTGRES_DSN and forwards the rest of cfg
// (pool tuning fields) to NewPostgresClient. Returns ERR_SYSTEM_CONFIG_INVALID
// if the env var is unset or empty.
func NewPostgresClientFromEnv(cfg Config) (*pgxpool.Pool, error) {
	cfg.DSN = os.Getenv(PostgresEnvVar)
	if cfg.DSN == "" {
		return nil, errorkit.NewError(errorkit.ERR_SYSTEM_CONFIG_INVALID).With(
			errorkit.WithReason("%s env var is unset or empty", PostgresEnvVar),
		)
	}
	return NewPostgresClient(cfg)
}

func normalizePostgresDSN(dsn string) string {
	schemeEnd := strings.Index(dsn, "://")
	if schemeEnd == -1 {
		return dsn
	}

	scheme := strings.ToLower(dsn[:schemeEnd])
	if scheme != "postgres" && scheme != "postgresql" {
		return dsn
	}

	authorityStart := schemeEnd + len("://")
	authorityEnd := len(dsn)
	for _, delimiter := range []string{"/", "?", "#"} {
		if index := strings.Index(dsn[authorityStart:], delimiter); index != -1 && authorityStart+index < authorityEnd {
			authorityEnd = authorityStart + index
		}
	}

	authority := dsn[authorityStart:authorityEnd]
	userInfoEnd := strings.LastIndex(authority, "@")
	if userInfoEnd == -1 {
		return dsn
	}

	userInfo := authority[:userInfoEnd]
	hostInfo := authority[userInfoEnd+1:]
	user, password, hasPassword := strings.Cut(userInfo, ":")

	user = unescapeURLComponent(user)
	if hasPassword {
		password = unescapeURLComponent(password)
		userInfo = url.UserPassword(user, password).String()
	} else {
		userInfo = url.User(user).String()
	}

	return dsn[:authorityStart] + userInfo + "@" + hostInfo + dsn[authorityEnd:]
}

func unescapeURLComponent(value string) string {
	unescaped, err := url.PathUnescape(value)
	if err != nil {
		return value
	}

	return unescaped
}
