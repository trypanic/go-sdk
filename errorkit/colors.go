package errorkit

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

// ColorsEnabled controls whether colors are used in out
// Can be disabled for environments that don't support ANSI codes
var ColorsEnabled = true

// colorize wraps text with ANSI color codes if colors are enabled
func colorize(code, text string) string {
	if !ColorsEnabled {
		return text
	}
	return code + text + Reset
}

// DisableColors turns off color out globally
func DisableColors() {
	ColorsEnabled = false
}

// EnableColors turns on color out globally
func EnableColors() {
	ColorsEnabled = true
}
