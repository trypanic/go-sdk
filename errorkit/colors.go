package errorkit

import "sync/atomic"

// ANSI color codes for terminal out
const (
	reset  = "\033[0m"
	red    = "\033[31m"
	yellow = "\033[33m"
	blue   = "\033[34m"
	purple = "\033[35m"
	cyan   = "\033[36m"
	bold   = "\033[1m"
)

// colorsDisabled reports whether DisableColors is in effect. The zero value
// (false) means colors are enabled, which is the default.
var colorsDisabled atomic.Bool

// colorize wraps text with ANSI color codes if colors are enabled
func colorize(code, text string) string {
	if colorsDisabled.Load() {
		return text
	}
	return code + text + reset
}

// DisableColors turns off color output globally. Safe for concurrent use.
func DisableColors() {
	colorsDisabled.Store(true)
}

// EnableColors turns on color output globally. Safe for concurrent use.
func EnableColors() {
	colorsDisabled.Store(false)
}
