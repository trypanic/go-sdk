package stringutils

import (
	"strings"
	"testing"
)

// Invariant: NormalizeText output only contains [a-z0-9_] and never panics.
func FuzzNormalizeText(f *testing.F) {
	for _, s := range []string{"Hello World", "a-b", "José 123", "K", "İ", "\xff", ""} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		got := NormalizeText(s)
		for i := 0; i < len(got); i++ {
			c := got[i]
			if c != '_' && (c < 'a' || c > 'z') && (c < '0' || c > '9') {
				t.Fatalf("NormalizeText(%q) = %q contains byte %q", s, got, c)
			}
		}
		if got != NormalizeText(got) {
			t.Fatalf("NormalizeText is not idempotent on %q", s)
		}
	})
}

// Invariants: RemoveMarkdownCodeBlock never panics, returns either its input
// unchanged or a substring of it, and a stripped result is already trimmed.
func FuzzRemoveMarkdownCodeBlock(f *testing.F) {
	for _, s := range []string{
		"```json\n{\"a\":1}\n```",
		"~~~\nx\n~~~",
		"````\n```\ninner\n```\n````",
		"```\na\n```\ntext\n```\nb\n```",
		"```\r\nx\r\n```\r\n",
		"no fence",
		"```",
	} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		got := RemoveMarkdownCodeBlock(s)
		if got == s {
			return
		}
		if !strings.Contains(s, got) {
			t.Fatalf("RemoveMarkdownCodeBlock(%q) = %q is not a substring of the input", s, got)
		}
		if got != strings.TrimSpace(got) {
			t.Fatalf("RemoveMarkdownCodeBlock(%q) = %q is not trimmed", s, got)
		}
	})
}
