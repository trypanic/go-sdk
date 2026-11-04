package urlkit_test

import (
	"fmt"

	"github.com/trypanic/go-sdk/urlkit"
)

func ExampleBuildURL() {
	u, err := urlkit.BuildURL("https://api.example.com/v1", "users", map[string]string{
		"page":  "1",
		"limit": "10",
	})
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	fmt.Println(u)
	// Output: https://api.example.com/v1/users?limit=10&page=1
}

func ExampleJoinPath() {
	s, err := urlkit.JoinPath("https://api.example.com/", "/v1/", "/users/")
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	fmt.Println(s)

	_, err = urlkit.JoinPath("https://api.example.com/v1", "..", "admin")
	fmt.Println(err != nil)
	// Output:
	// https://api.example.com/v1/users
	// true
}
