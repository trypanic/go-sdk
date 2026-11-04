package postgres_test

import (
	"context"
	"fmt"

	"github.com/trypanic/go-sdk/v2/errorkit"
	"github.com/trypanic/go-sdk/v2/postgres"
)

func ExampleNewContext() {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	// Cancellation is checked before opening a connection.
	_, err := postgres.NewContext(ctx, postgres.Config{})
	fmt.Println(errorkit.HasCode(err, postgres.ERR_DB_POSTGRES_CANCELED))
	// Output: true
}
