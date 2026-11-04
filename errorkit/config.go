package errorkit

import (
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

// ==============================
// Configuration Variables
// ==============================

// defaultMaxStackDepth is the stack depth used until SetMaxStackDepth is
// called.
const defaultMaxStackDepth = 32

// maxStackDepth is the maximum number of stack frames NewError captures.
// It is read on every NewError and written by SetMaxStackDepth, so it is
// atomic.
var maxStackDepth atomic.Int32

func init() {
	maxStackDepth.Store(defaultMaxStackDepth)
}

// ==============================
// Configuration Functions
// ==============================

// SetMaxStackDepth configures the maximum stack depth to capture, clamped to
// [1, 100]. Default is 32 frames. It is safe to call concurrently with
// NewError.
func SetMaxStackDepth(depth int) {
	maxStackDepth.Store(int32(clampStackDepth(depth)))
}

// currentMaxStackDepth returns the configured global stack depth.
func currentMaxStackDepth() int {
	return int(maxStackDepth.Load())
}

func clampStackDepth(depth int) int {
	return min(max(depth, 1), 100)
}

// ==============================
// Environment Detection
// ==============================

// InitFromEnv initializes configuration from environment variables.
// Supported variables:
//   - ERROR_KIT_MAX_STACK: integer for max stack depth (clamped like
//     SetMaxStackDepth). A value that is not an integer, including one with
//     trailing characters such as "12abc", is ignored.
func InitFromEnv() {
	maxStackEnv := strings.TrimSpace(os.Getenv("ERROR_KIT_MAX_STACK"))
	if maxStackEnv == "" {
		return
	}
	if depth, err := strconv.Atoi(maxStackEnv); err == nil {
		SetMaxStackDepth(depth)
	}
}

// nowISO8601 returns current time in ISO8601 format
func nowISO8601() string {
	return time.Now().UTC().Format(time.RFC3339)
}
