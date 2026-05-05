# stringutils

String transformation utilities for normalizing identifiers and stripping Markdown code-block wrappers from LLM output.

## Overview

| Symbol | Kind | Purpose |
|---|---|---|
| `NormalizeText` | Function | Convert a human-readable string into a lowercase, underscore-separated, alphanumeric-only identifier |
| `RemoveMarkdownCodeBlock` | Function | Strip a wrapping ` ``` ` / ` ```lang ` code fence from a string when it covers the entire content |

---

## Quick Start

```go
import "github.com/trypanic/go-sdk/stringutils"

// Normalize a field name or category path
key := stringutils.NormalizeText("Men's Clothing & Accessories")
// key == "mens_clothing__accessories"

// Strip a code block returned by an LLM
raw := "```json\n[{\"id\":1}]\n```"
clean := stringutils.RemoveMarkdownCodeBlock(raw)
// clean == `[{"id":1}]`
```

---

## Configuration / Settings

This package has no configuration struct or environment variables. Both functions are stateless.

---

## API Reference

### `NormalizeText`

```go
func NormalizeText(input string) string
```

Transforms `input` into a lowercase, underscore-delimited token suitable for use as an identifier or map key.

**Steps applied in order:**
1. Convert all characters to lowercase with `strings.ToLower`.
2. Replace every space character (`U+0020`) with an underscore (`_`).
3. Strip every character that is not an ASCII letter (`a-z`), ASCII digit (`0-9`), or underscore (`_`).

**Edge cases:**
- Characters outside the ASCII range (e.g., accented letters, CJK) are removed entirely.
- Consecutive spaces produce consecutive underscores (e.g., `"a  b"` → `"a__b"`).
- An empty string or a string of only non-matching characters returns `""`.

**Error conditions:** None.

---

### `RemoveMarkdownCodeBlock`

```go
func RemoveMarkdownCodeBlock(input string) string
```

Returns the inner content of `input` if — and only if — the entire string (after trimming leading/trailing whitespace) is wrapped in a single Markdown code fence. Otherwise returns `input` unchanged.

**Fence formats recognised:**
- ` ``` `
- ` ```json `, ` ```go `, or any other language tag immediately after the opening ` ``` `

**Line ending support:** Both `\n` and `\r\n` are handled.

**The outer fence must span the whole string.** If there is any content before the opening fence or after the closing fence (ignoring trailing spaces/tabs), the string is returned as-is.

**Edge cases:**
- Leading and trailing whitespace around the fence is trimmed before matching, and the extracted content is also trimmed before being returned.
- Nested or multiple fences within the content are not altered.
- An empty code block (` ```\n\n``` `) returns `""`.

**Error conditions:** None.

---

## Real-World Usage

### LLM gateway — stripping code fences from model output

`RemoveMarkdownCodeBlock` is typically called immediately after receiving a raw LLM response. Models often wrap JSON answers in a ` ```json … ``` ` fence; this call removes the fence before `json.Unmarshal` is attempted.

```go
raw, err := l.callLLM(ctx, llmConfig, systemPrompt, chunk)
if err != nil {
    return nil, wrapExternal(err, fmt.Sprintf("LLM call failed on chunk %d", i))
}

raw = stringutils.RemoveMarkdownCodeBlock(raw)

var partial []entities.EnrichAttribute
if err := json.Unmarshal([]byte(raw), &partial); err != nil {
    return nil, wrapExternal(err, fmt.Sprintf("unmarshal failed on chunk %d", i))
}
```

---

## Lifecycle / Concurrency Notes

Both functions are pure and stateless. The compiled regular expressions (`codeBlockRegex`) are package-level variables initialised once at program start via `regexp.MustCompile`. Both functions are safe for concurrent use without additional synchronisation.

---

## Dependencies

| Package | Role |
|---|---|
| `regexp` | Compiled regular expression for fence detection |
| `strings` | Lowercasing, space replacement, and trimming |
