package errorkit

import (
	"fmt"
	"os"
	"time"
)

// ==============================
// Configuration Variables
// ==============================

// MaxStackDepth controls the maximum number of stack frames to capture
// Can be modified at runtime: errorkit.MaxStackDepth = 50
var MaxStackDepth = 32

// ==============================
// Configuration Functions
// ==============================

// SetMaxStackDepth configures the maximum stack depth to capture
// Default is 32 frames
func SetMaxStackDepth(depth int) {
	MaxStackDepth = clampStackDepth(depth)
}

func clampStackDepth(depth int) int {
	if depth < 1 {
		depth = 1
	}
	if depth > 100 {
		depth = 100
	}
	return depth
}

// ==============================
// Environment Detection
// ==============================

// InitFromEnv initializes configuration from environment variables
// Supported variables:
// - ERROR_KIT_MAX_STACK: integer for max stack depth
func InitFromEnv() {
	// Check max stack depth
	if maxStackEnv := os.Getenv("ERROR_KIT_MAX_STACK"); maxStackEnv != "" {
		var depth int
		if _, err := fmt.Sscanf(maxStackEnv, "%d", &depth); err == nil {
			SetMaxStackDepth(depth)
		}
	}
}

// nowISO8601 returns current time in ISO8601 format
func nowISO8601() string {
	return time.Now().UTC().Format(time.RFC3339)
}
