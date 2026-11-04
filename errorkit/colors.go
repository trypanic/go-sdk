package errorkit

import "sync/atomic"

// ANSI color codes for terminal out
var (
	Reset  = "\033[0m"
	Red    = "\033[31m"
	Green  = "\033[32m"
	Yellow = "\033[33m"
	Blue   = "\033[34m"
	Purple = "\033[35m"
	Cyan   = "\033[36m"
	Gray   = "\033[90m"
	Bold   = "\033[1m"
)

// colorsDisabled reports whether DisableColors is in effect. The zero value
// (false) means colors are enabled, which is the default.
var colorsDisabled atomic.Bool

// colorize wraps text with ANSI color codes if colors are enabled
func colorize(code, text string) string {
	if colorsDisabled.Load() {
		return text
	}
	return code + text + Reset
}

// DisableColors turns off color output globally. Safe for concurrent use.
func DisableColors() {
	colorsDisabled.Store(true)
}

// EnableColors turns on color output globally. Safe for concurrent use.
func EnableColors() {
	colorsDisabled.Store(false)
}
