package stringutils

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// NormalizeText converts text to lowercase, replaces spaces with underscores,
// and strips any characters that are not ASCII letters, digits or underscores.
//
// The result is lossy and not injective: different inputs can produce the
// same output ("a-b" and "ab" both give "ab"), and input without ASCII
// letters or digits gives "". Lowercasing runs before the filter, so the few
// non-ASCII letters whose lowercase form is ASCII are kept (the Kelvin sign
// U+212A gives "k", "İ" gives "i").
func NormalizeText(input string) string {
	var b strings.Builder
	b.Grow(len(input))
	for _, r := range input {
		if r >= utf8.RuneSelf {
			r = unicode.ToLower(r)
		} else if 'A' <= r && r <= 'Z' {
			r += 'a' - 'A'
		}
		switch {
		case r == ' ':
			b.WriteByte('_')
		case r == '_', 'a' <= r && r <= 'z', '0' <= r && r <= '9':
			b.WriteByte(byte(r))
		}
	}
	return b.String()
}

// RemoveMarkdownCodeBlock returns the content of input when the whole input
// (ignoring surrounding whitespace) is exactly one fenced code block, and
// input unchanged otherwise. The returned content is trimmed of surrounding
// whitespace.
//
// Grammar:
//   - The first line is an opening fence: three or more backticks (or tildes)
//     followed by an optional info string ("json", "python3", "c++",
//     "js title=x", trailing spaces). A backtick fence's info string must not
//     contain a backtick.
//   - The last line is a closing fence: the same character, at least as many
//     times as the opening fence, optionally surrounded by spaces or tabs.
//   - No line in between is a fence line (a line that starts, after spaces or
//     tabs, with at least as many fence characters as the opening fence), so
//     content with several blocks or prose between blocks is left untouched.
//     A longer outer fence (````) may wrap shorter inner fences (```).
//   - Both \n and \r\n line endings are supported.
func RemoveMarkdownCodeBlock(input string) string {
	s := strings.TrimSpace(input)

	first, rest, ok := strings.Cut(s, "\n")
	if !ok {
		return input
	}
	char, n, ok := openingFence(strings.TrimSuffix(first, "\r"))
	if !ok {
		return input
	}

	body, last := "", rest
	if i := strings.LastIndexByte(rest, '\n'); i >= 0 {
		body, last = rest[:i], rest[i+1:]
	}
	if !isClosingFence(last, char, n) {
		return input
	}
	for line := range strings.SplitSeq(body, "\n") {
		if fenceRun(strings.TrimLeft(line, " \t"), char) >= n {
			return input
		}
	}

	return strings.TrimSpace(body)
}

// openingFence reports the fence character and length of an opening fence
// line, or ok == false when line is not one.
func openingFence(line string) (char byte, n int, ok bool) {
	if line == "" || (line[0] != '`' && line[0] != '~') {
		return 0, 0, false
	}
	char = line[0]
	n = fenceRun(line, char)
	if n < 3 {
		return 0, 0, false
	}
	if char == '`' && strings.IndexByte(line[n:], '`') >= 0 {
		return 0, 0, false
	}
	return char, n, true
}

// isClosingFence reports whether line is a run of at least n char bytes,
// optionally surrounded by spaces or tabs (and a trailing \r).
func isClosingFence(line string, char byte, n int) bool {
	t := strings.Trim(line, " \t\r")
	return len(t) >= n && fenceRun(t, char) == len(t)
}

// fenceRun returns the number of leading char bytes in s.
func fenceRun(s string, char byte) int {
	i := 0
	for i < len(s) && s[i] == char {
		i++
	}
	return i
}
