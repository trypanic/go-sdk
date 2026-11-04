package mongotest_test

import (
	"fmt"

	"github.com/trypanic/go-sdk/v2/mongo/mongotest"
)

func ExampleSingleResult() {
	result := mongotest.SingleResult(func(out any) error { *out.(*string) = "Ada"; return nil })
	var name string
	err := result.Decode(&name)
	fmt.Println(name, err)
	// Output: Ada <nil>
}
