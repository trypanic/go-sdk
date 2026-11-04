package stringutils

import "testing"

func TestNormalizeText(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"lowercase", "HELLO", "hello"},
		{"spaces to underscore", "hello world", "hello_world"},
		{"strip punctuation", "a-b.c!d", "abcd"},
		{"keep digits and underscore", "a1_b2", "a1_b2"},
		{"strip unicode", "café", "caf"},
		{"mixed", "Foo Bar! 123", "foo_bar_123"},
		{"empty", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NormalizeText(tt.in); got != tt.want {
				t.Errorf("NormalizeText(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestRemoveMarkdownCodeBlock(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"plain fence", "```\nbody\n```", "body"},
		{"json fence", "```json\n{\"a\":1}\n```", "{\"a\":1}"},
		{"crlf line endings", "```json\r\n{\"a\":1}\r\n```", "{\"a\":1}"},
		{"surrounding whitespace", "  ```\nbody\n```  ", "body"},
		{"no fence returned verbatim", "just text", "just text"},
		{"partial fence not stripped", "```\nbody", "```\nbody"},
		{"multiline body", "```\nline1\nline2\n```", "line1\nline2"},
		{
			"two blocks with prose returned verbatim",
			"```json\nA\n```\n\nSome text\n\n```json\nB\n```",
			"```json\nA\n```\n\nSome text\n\n```json\nB\n```",
		},
		{"inner fence line returned verbatim", "```\na\n```go\nb\n```", "```\na\n```go\nb\n```"},
		{"prose before fence returned verbatim", "Here:\n```\nbody\n```", "Here:\n```\nbody\n```"},
		{"prose after fence returned verbatim", "```\nbody\n```\nDone.", "```\nbody\n```\nDone."},
		{"info string with digits", "```python3\nx\n```", "x"},
		{"info string with symbols", "```c++\nx\n```", "x"},
		{"info string with dash", "```objective-c\nx\n```", "x"},
		{"info string with trailing space", "```json \nx\n```", "x"},
		{"info string with attributes", "```js title=\"a.js\"\nx\n```", "x"},
		{"backtick in info string is not a fence", "```a`b\nx\n```", "```a`b\nx\n```"},
		{"empty block single newline", "```\n```", ""},
		{"empty block blank line", "```\n\n```", ""},
		{"longer fence", "````\nx\n````", "x"},
		{"longer fence keeps shorter inner fence", "````md\n```go\nx\n```\n````", "```go\nx\n```"},
		{"closing fence longer than opening", "```\nx\n`````", "x"},
		{"closing fence shorter than opening", "````\nx\n```", "````\nx\n```"},
		{"tilde fence", "~~~\nx\n~~~", "x"},
		{"mixed fence characters", "```\nx\n~~~", "```\nx\n~~~"},
		{"two backticks are not a fence", "``\nx\n``", "``\nx\n``"},
		{"crlf multi-line body kept", "```\r\na\r\nb\r\n```", "a\r\nb"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := RemoveMarkdownCodeBlock(tt.in); got != tt.want {
				t.Errorf("RemoveMarkdownCodeBlock(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
