package httpservertest_test

import (
	"fmt"

	"github.com/trypanic/go-sdk/httpserver/httpservertest"
)

func ExampleNewContext() {
	recorder := httpservertest.NewContext(nil)
	recorder.JSON(201, map[string]string{"id": "123"})
	fmt.Println(recorder.Response.Code, recorder.Err)
	// Output: 201 <nil>
}
