package mongo_test

import (
	"context"
	"fmt"

	"github.com/trypanic/go-sdk/errorkit"
	"github.com/trypanic/go-sdk/mongo"
)

func ExampleWrapOperationError() {
	err := mongo.WrapOperationError(context.Canceled, "find")
	fmt.Println(errorkit.HasCode(err, mongo.ERR_DB_MONGO_CANCELED))
	// Output: true
}
