package stringutils

import (
	"regexp"
	"strings"
)

// NormalizeText converts text to lowercase, replaces spaces with underscores,
// and strips any characters that are not alphanumeric or underscores.
func NormalizeText(input string) string {
	// Convert to lowercase
	text := strings.ToLower(input)

	// Replace spaces with underscores
	text = strings.ReplaceAll(text, " ", "_")

	// Keep only letters, digits and underscores
	reg := regexp.MustCompile(`[^a-z0-9_]`)
	text = reg.ReplaceAllString(text, "")

	return text
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
