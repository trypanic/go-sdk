package otlp_test

import (
	"fmt"

	"github.com/trypanic/go-sdk/v2/logger/otlp"
)

func ExampleNewWriter() {
	// Without an installed provider, the global OTel provider is a no-op.
	writer := otlp.NewWriter("example-service")
	line := []byte(`{"level":"info","message":"ready"}`)
	n, err := writer.Write(line)
	fmt.Println(n == len(line), err)
	// Output: true <nil>
}
