package postgres_test

import (
	"context"
	"fmt"

	"github.com/trypanic/go-sdk/errorkit"
	"github.com/trypanic/go-sdk/postgres"
)

func ExampleNewContext() {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	// Cancellation is checked before opening a connection.
	_, err := postgres.NewContext(ctx, postgres.Config{})
	fmt.Println(errorkit.HasCode(err, postgres.ERR_DB_POSTGRES_CANCELED))
	// Output: true
}
