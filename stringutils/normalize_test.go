package stringutils

import (
	"regexp"
	"strings"
	"testing"
)

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

// normalizeTextReference is the original regexp-based implementation; the
// single-pass NormalizeText must stay output-identical to it.
func normalizeTextReference(input string) string {
	text := strings.ToLower(input)
	text = strings.ReplaceAll(text, " ", "_")
	return regexp.MustCompile(`[^a-z0-9_]`).ReplaceAllString(text, "")
}

func TestNormalizeTextMatchesReference(t *testing.T) {
	for _, in := range []string{
		"Men's Clothing & Accessories",
		"K",        // Kelvin sign lowers to ASCII k
		"İstanbul", // dotted capital I lowers to ASCII i
		"日本語",
		"Café",
		"a-b",
		"a  b",
		"tab\there",
		"\xff\xfeinvalid utf-8",
		"__ALREADY_normal_123__",
		"",
	} {
		if got, want := NormalizeText(in), normalizeTextReference(in); got != want {
			t.Errorf("NormalizeText(%q) = %q, reference = %q", in, got, want)
		}
	}
}

// NormalizeText used to compile its regexp on every call (18 allocs/op);
// the single-pass version allocates only the result.
func TestNormalizeTextAllocations(t *testing.T) {
	allocs := testing.AllocsPerRun(100, func() {
		_ = NormalizeText("Men's Clothing & Accessories")
	})
	if allocs > 1 {
		t.Fatalf("NormalizeText allocs/op = %v, want <= 1", allocs)
	}
}

func BenchmarkNormalizeText(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		_ = NormalizeText("Men's Clothing & Accessories")
	}
}

// NormalizeText is lossy: the README documents collisions and the empty
// result, so pin them.
func TestNormalizeTextCollisionsAndEmptyResult(t *testing.T) {
	if NormalizeText("a-b") != NormalizeText("ab") {
		t.Errorf("expected %q and %q to collide", "a-b", "ab")
	}
	if got := NormalizeText("日本語"); got != "" {
		t.Errorf("NormalizeText(%q) = %q, want empty", "日本語", got)
	}
	if got := NormalizeText("İstanbul Kelvin"); got != "istanbul_kelvin" {
		t.Errorf("NormalizeText lowercase exception = %q, want %q", got, "istanbul_kelvin")
	}
}
