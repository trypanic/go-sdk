package algorithms_test

import (
	"fmt"

	"github.com/trypanic/go-sdk/v2/algorithms"
)

func ExampleNewExponentialBackoff() {
	schedule := algorithms.NewExponentialBackoff(algorithms.ExponentialBackoffConfig{MaxRetries: 1})
	fmt.Println(schedule.NextBackOff() == algorithms.Stop)
	// Output: true
}
