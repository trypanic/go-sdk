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
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := RemoveMarkdownCodeBlock(tt.in); got != tt.want {
				t.Errorf("RemoveMarkdownCodeBlock(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
