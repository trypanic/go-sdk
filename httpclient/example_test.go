package httpclient_test

import (
	"fmt"

	"github.com/trypanic/go-sdk/httpclient"
)

func ExampleNewDefaultClient() {
	client := httpclient.NewDefaultClient()
	defer client.CloseIdleConnections()
	fmt.Println(client.Timeout)
	// Output: 1m0s
}
