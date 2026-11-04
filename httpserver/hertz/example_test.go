package hertz_test

import (
	"fmt"

	"github.com/trypanic/go-sdk/httpserver"
	"github.com/trypanic/go-sdk/httpserver/hertz"
)

func ExampleNewServer() {
	server := hertz.NewServer(httpserver.ServerConfig{Host: "127.0.0.1", Port: 8080}, hertz.WithoutTracing(), hertz.WithoutAudit())
	// Construction does not listen. The application owns Run and Shutdown.
	fmt.Println(server.Engine() != nil)
	// Output: true
}
