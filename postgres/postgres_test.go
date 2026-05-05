package database

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/trypanic/go-sdk/errorkit"
)

func TestNewPostgresClient_MissingDSN_ReturnsConfigError(t *testing.T) {
	_, err := NewPostgresClient(Config{})
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	var appErr *errorkit.AppError
	if !errors.As(err, &appErr) {
		t.Fatalf("expected *errorkit.AppError, got %T", err)
	}

	if appErr.Code() != errorkit.ERR_SYSTEM_CONFIG_INVALID {
		t.Fatalf("expected code %s, got %s", errorkit.ERR_SYSTEM_CONFIG_INVALID, appErr.Code())
	}
}

func TestNewPostgresClient_DoesNotReadEnvVar(t *testing.T) {
	// Setting POSTGRES_DSN must not influence NewPostgresClient — the env
	// fallback is now opt-in via NewPostgresClientFromEnv.
	t.Setenv("POSTGRES_DSN", "postgres://from-env:secret@host/db")

	_, err := NewPostgresClient(Config{})
	if err == nil {
		t.Fatal("expected error when DSN missing despite env var being set")
	}

	var appErr *errorkit.AppError
	if !errors.As(err, &appErr) || appErr.Code() != errorkit.ERR_SYSTEM_CONFIG_INVALID {
		t.Fatalf("expected ERR_SYSTEM_CONFIG_INVALID, got %v", err)
	}
}

func TestNewPostgresClientFromEnv_MissingEnvReturnsConfigError(t *testing.T) {
	t.Setenv("POSTGRES_DSN", "")

	_, err := NewPostgresClientFromEnv(Config{})
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	var appErr *errorkit.AppError
	if !errors.As(err, &appErr) || appErr.Code() != errorkit.ERR_SYSTEM_CONFIG_INVALID {
		t.Fatalf("expected ERR_SYSTEM_CONFIG_INVALID, got %v", err)
	}
}

func TestNormalizePostgresDSN_EscapesAtSignsInPassword(t *testing.T) {
	rawDSN := "postgres://pguser:pw@example@1706@172.18.11.42:5432/acgamzscr?sslmode=disable"

	normalizedDSN := normalizePostgresDSN(rawDSN)

	if normalizedDSN != "postgres://pguser:pw%40example%401706@172.18.11.42:5432/acgamzscr?sslmode=disable" {
		t.Fatalf("unexpected normalized DSN: %s", normalizedDSN)
	}

	cfg, err := pgxpool.ParseConfig(normalizedDSN)
	if err != nil {
		t.Fatalf("expected normalized DSN to parse: %v", err)
	}

	if cfg.ConnConfig.User != "pguser" {
		t.Fatalf("expected user pguser, got %s", cfg.ConnConfig.User)
	}

	if cfg.ConnConfig.Password != "pw@example@1706" {
		t.Fatalf("expected password with @ signs preserved, got %s", cfg.ConnConfig.Password)
	}

	if cfg.ConnConfig.Host != "172.18.11.42" {
		t.Fatalf("expected host 172.18.11.42, got %s", cfg.ConnConfig.Host)
	}
}

func TestNormalizePostgresDSN_DoesNotDoubleEscapePassword(t *testing.T) {
	rawDSN := "postgres://pguser:pw%40example%401706@172.18.11.42:5432/acgamzscr?sslmode=disable"

	normalizedDSN := normalizePostgresDSN(rawDSN)

	if normalizedDSN != rawDSN {
		t.Fatalf("expected normalized DSN to remain unchanged, got %s", normalizedDSN)
	}
}
