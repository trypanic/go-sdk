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

// RemoveMarkdownCodeBlock strips a fenced code block wrapper from input if the
// entire content is wrapped in one. Supports:
// - ``` or ```json fences
// - \n and \r\n line endings
// - surrounding whitespace
// Only removes the wrapper when it covers the entire content.
var codeBlockRegex = regexp.MustCompile(
	"(?s)^\\s*```[a-zA-Z]*\\r?\\n(.*?)\\r?\\n```[\\t ]*$",
)

func RemoveMarkdownCodeBlock(input string) string {
	s := strings.TrimSpace(input)

	matches := codeBlockRegex.FindStringSubmatch(s)
	if len(matches) == 2 {
		return strings.TrimSpace(matches[1])
	}

	return input
}
