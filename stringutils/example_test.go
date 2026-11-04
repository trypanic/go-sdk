package stringutils_test

import (
	"fmt"

	"github.com/trypanic/go-sdk/stringutils"
)

func ExampleNormalizeText() {
	fmt.Println(stringutils.NormalizeText("Order Status: SHIPPED!"))
	// Output: order_status_shipped
}

func ExampleRemoveMarkdownCodeBlock() {
	reply := "```json\n{\"ok\": true}\n```"
	fmt.Println(stringutils.RemoveMarkdownCodeBlock(reply))
	fmt.Println(stringutils.RemoveMarkdownCodeBlock("plain text"))
	// Output:
	// {"ok": true}
	// plain text
}
