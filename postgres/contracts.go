package postgres

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// Querier runs queries. *pgxpool.Pool, *pgx.Conn and pgx.Tx implement it, so
// a StoredProcedure can run inside a transaction or against a test fake.
type Querier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}

// Execer runs statements that return no rows.
type Execer interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

// ClientPort is the subset of *pgxpool.Pool that repositories need: queries,
// statements, transactions and Close.
type ClientPort interface {
	Querier
	Execer
	Begin(ctx context.Context) (pgx.Tx, error)
	Close()
}
