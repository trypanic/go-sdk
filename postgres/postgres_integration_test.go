//go:build integration

package postgres_test

import (
	"context"
	"errors"
	"net"
	"net/url"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/trypanic/go-sdk/errorkit"
	"github.com/trypanic/go-sdk/internal/integrationtest"
	"github.com/trypanic/go-sdk/postgres"
)

func TestPostgresIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("requires local PostgreSQL container")
	}
	container := integrationtest.Start(t, testcontainers.ContainerRequest{
		Image: "postgres:18.6-alpine3.24@sha256:10a5c69a83035ab5239f5620d708520e8b28f5aa1f2b20143e6024532edadfb5", ExposedPorts: []string{"5432/tcp"},
		Env:        map[string]string{"POSTGRES_HOST_AUTH_METHOD": "trust", "POSTGRES_DB": "sdk"},
		WaitingFor: wait.ForLog("database system is ready to accept connections").WithOccurrence(2).WithStartupTimeout(time.Minute),
	})
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	host, err := container.Host(ctx)
	require.NoError(t, err)
	port, err := container.MappedPort(ctx, "5432/tcp")
	require.NoError(t, err)
	dsn := (&url.URL{Scheme: "postgres", User: url.User("postgres"), Host: net.JoinHostPort(host, port.Port()), Path: "sdk", RawQuery: "sslmode=disable"}).String()
	setup, finishSetup := context.WithCancel(ctx)
	defer finishSetup()
	pool, err := postgres.NewContext(setup, postgres.Config{DSN: dsn, MaxConns: 3})
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	finishSetup()
	require.NoError(t, pool.Ping(ctx))
	_, err = pool.Exec(ctx, `CREATE TABLE items (id bigint PRIMARY KEY, name text NOT NULL);
 CREATE FUNCTION put_item(doc jsonb) RETURNS SETOF items LANGUAGE sql AS $$
 INSERT INTO items SELECT (doc->>'id')::bigint, doc->>'name' RETURNING * $$;
 CREATE FUNCTION get_items() RETURNS SETOF items LANGUAGE sql AS $$ SELECT * FROM items ORDER BY id $$;`)
	require.NoError(t, err)
	type item struct {
		ID   int64  `db:"id" json:"id"`
		Name string `db:"name" json:"name"`
	}
	sp := postgres.NewStoredProcedure[item](pool)
	t.Run("stored procedure JSON and rows", func(t *testing.T) {
		want := item{ID: 1, Name: "first"}
		got, err := sp.QueryRowJSON(ctx, "SELECT * FROM put_item($1::jsonb)", want)
		require.NoError(t, err)
		require.Equal(t, want, got)
		rows, err := sp.QueryJSON(ctx, "SELECT * FROM put_item($1::jsonb)", item{ID: 2, Name: "second"})
		require.NoError(t, err)
		require.Equal(t, []item{{ID: 2, Name: "second"}}, rows)
		rows, err = sp.Query(ctx, "SELECT * FROM get_items()")
		require.NoError(t, err)
		require.Equal(t, []item{want, {ID: 2, Name: "second"}}, rows)
	})
	t.Run("missing row and unique violation preserve causes", func(t *testing.T) {
		_, err := sp.QueryRow(ctx, "SELECT * FROM items WHERE id = $1", 99)
		require.True(t, errorkit.HasCode(err, postgres.ERR_DB_POSTGRES_NOT_FOUND))
		require.ErrorIs(t, err, pgx.ErrNoRows)
		_, err = sp.QueryRowJSON(ctx, "SELECT * FROM put_item($1::jsonb)", item{ID: 3, Name: "duplicate"})
		require.NoError(t, err)
		_, err = sp.QueryRowJSON(ctx, "SELECT * FROM put_item($1::jsonb)", item{ID: 3, Name: "duplicate"})
		require.True(t, errorkit.HasCode(err, errorkit.ERR_VALIDATION_DUPLICATE))
		cause, ok := errors.AsType[*pgconn.PgError](err)
		require.True(t, ok)
		require.Equal(t, "23505", cause.Code)
	})
	t.Run("cancellation and server timeout", func(t *testing.T) {
		canceled, stop := context.WithCancel(ctx)
		stop()
		_, err := sp.Query(canceled, "SELECT * FROM get_items()")
		require.True(t, errorkit.HasCode(err, postgres.ERR_DB_POSTGRES_CANCELED))
		require.ErrorIs(t, err, context.Canceled)
		tx, err := pool.Begin(ctx)
		require.NoError(t, err)
		defer func() {
			cleanup, stop := context.WithTimeout(context.Background(), time.Second)
			defer stop()
			require.NoError(t, tx.Rollback(cleanup))
		}()
		_, err = tx.Exec(ctx, "SET LOCAL statement_timeout = '50ms'")
		require.NoError(t, err)
		scalar := postgres.NewStoredProcedure[int](tx)
		_, err = scalar.QueryRow(ctx, "SELECT 1 FROM pg_sleep(5)")
		require.True(t, errorkit.HasCode(err, postgres.ERR_DB_POSTGRES_TIMEOUT))
		cause, ok := errors.AsType[*pgconn.PgError](err)
		require.True(t, ok)
		require.Equal(t, "57014", cause.Code)
	})
}
