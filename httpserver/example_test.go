package httpserver_test

import (
	"fmt"

	"github.com/trypanic/go-sdk/errorkit"
	"github.com/trypanic/go-sdk/httpserver"
	"github.com/trypanic/go-sdk/httpserver/httpservertest"
)

func ExampleReplyError() {
	recorder := httpservertest.NewContext(nil)
	httpserver.ReplyError(recorder, errorkit.NewError(errorkit.ERR_CLIENT_NOT_FOUND))
	fmt.Println(recorder.Response.Code, recorder.Err)
	// Output: 404 <nil>
}
