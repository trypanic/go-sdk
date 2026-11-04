package validators_test

import (
	"fmt"

	"github.com/trypanic/go-sdk/v2/errorkit"
	"github.com/trypanic/go-sdk/v2/validators"
)

func ExampleIsWithinRange() {
	err := validators.IsWithinRange(1, 10)(11)
	fmt.Println(errorkit.HasCode(err, errorkit.ERR_VALIDATION_INCONSISTENT))
	// Output: true
}
