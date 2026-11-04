package grpc_test

import (
	"fmt"

	"github.com/trypanic/go-sdk/errorkit"
	sdkgrpc "github.com/trypanic/go-sdk/grpc"
	"google.golang.org/grpc/status"
)

func ExampleToStatus() {
	err := errorkit.NewError(errorkit.ERR_CLIENT_NOT_FOUND)
	fmt.Println(status.Code(sdkgrpc.ToStatus(err)))
	fmt.Println(errorkit.HasCode(sdkgrpc.FromStatus(sdkgrpc.ToStatus(err)), errorkit.ERR_CLIENT_NOT_FOUND))
	// Output:
	// NotFound
	// true
}
