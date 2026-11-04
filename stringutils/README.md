# stringutils

String transformation utilities for normalizing identifiers and stripping Markdown code-block wrappers from LLM output.

## Overview

| Symbol | Kind | Purpose |
|---|---|---|
| `NormalizeText` | Function | Convert a human-readable string into a lowercase, underscore-separated, alphanumeric-only identifier |
| `RemoveMarkdownCodeBlock` | Function | Strip a wrapping ` ``` ` / ` ```lang ` / `~~~` code fence from a string when it is exactly one block covering the entire content |

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

Returns the inner content of `input` if — and only if — the entire string (after trimming leading/trailing whitespace) is exactly one Markdown fenced code block. Otherwise returns `input` unchanged.

**Grammar (parsed line by line):**
- **Opening fence** (first line): three or more backticks (` ``` `, ` ```` `) or tildes (`~~~`), followed by an optional info string. The info string may contain any characters (`json`, `python3`, `c++`, `objective-c`, `js title="a.js"`, trailing spaces), except that a backtick fence's info string must not contain a backtick.
- **Closing fence** (last line): the same fence character repeated at least as many times as in the opening fence, optionally surrounded by spaces or tabs.
- **No inner fence line:** if any line between them starts (after spaces or tabs) with at least as many fence characters as the opening fence, the input is not a single block and is returned as-is. A longer outer fence (` ```` `) may therefore wrap shorter inner fences (` ``` `).

**Line ending support:** Both `\n` and `\r\n` are handled.

**The outer fence must span the whole string.** If there is any content before the opening fence or after the closing fence, the string is returned as-is.

**Edge cases:**
- Leading and trailing whitespace around the fence is trimmed before matching, and the extracted content is also trimmed before being returned.
- Several blocks (for example two ` ```json ` blocks with prose between them) are returned unchanged, not merged.
- An empty code block (` ```\n``` ` or ` ```\n\n``` `) returns `""`.
- Two backticks (` `` `) are not a fence; mixed fence characters (` ``` ` … `~~~`) do not match.

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

Both functions are pure and stateless and are safe for concurrent use without additional synchronisation. `RemoveMarkdownCodeBlock` parses the input line by line without regular expressions.

---

## Dependencies

| Package | Role |
|---|---|
| `regexp` | Character filter in `NormalizeText` |
| `strings` | Lowercasing, space replacement, and trimming |
