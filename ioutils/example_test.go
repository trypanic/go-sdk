package ioutils_test

import (
	"bytes"
	"fmt"

	"github.com/trypanic/go-sdk/v2/ioutils"
)

func ExampleWriteJSON() {
	var output bytes.Buffer
	if err := ioutils.WriteJSON(&output, map[string]int{"count": 2}); err != nil {
		fmt.Println(err)
		return
	}
	fmt.Print(output.String())
	// Output: {"count":2}
}
