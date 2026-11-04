// Package postgres provides a pgx/v5 connection-pool factory and a generic
// stored-procedure runner for PostgreSQL.
//
// Import it as github.com/trypanic/go-sdk/postgres; the package identifier
// is postgres.
//
//   - New builds a *pgxpool.Pool from an explicit Config and
//     pings it; NewFromEnv reads the DSN from POSTGRES_DSN.
//   - StoredProcedure runs a query and scans the rows into T (structs by
//     column name, scalars directly). NewStoredProcedure returns one wrapped
//     with a span per call; WrapWithInstrumenter chooses the tracer, and a
//     nil instrumenter disables tracing.
//
// Every error is an *errorkit.AppError that wraps the driver error, with a
// code that follows the cause (see the README "Error Handling" table).
//
// A pool and a StoredProcedure are safe for concurrent use. Create one pool
// per process, share it, and close it at shutdown.
package postgres
