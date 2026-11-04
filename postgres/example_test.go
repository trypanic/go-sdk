package database_test

import (
	"context"
	"log"
	"time"

	database "github.com/trypanic/go-sdk/postgres"
)

type item struct {
	ID   int64  `db:"id"`
	Name string `db:"name"`
}

// Example wires a pool and a traced stored-procedure runner. It needs a
// database, so it is compiled but not run.
func Example() {
	pool, err := database.NewPostgresClient(database.Config{
		DSN:             "postgres://user:pass@localhost:5432/mydb?sslmode=disable",
		MaxConnLifetime: time.Hour,
		MaxConns:        10,
	})
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()

	// NewStoredProcedure already traces each call; do not wrap it again.
	sp := database.NewStoredProcedure[item](pool)

	ctx := context.Background()
	created, err := sp.QueryJSON(ctx, "SELECT * FROM insert_items($1, $2::jsonb)", []item{{Name: "alpha"}}, "tenant-a")
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("created %d items", len(created))
}
