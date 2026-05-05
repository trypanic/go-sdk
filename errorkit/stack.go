package errorkit

import (
	"runtime"
	"strings"
)

// skipNewErrorFrames is the number of frames to skip when capturing stack
// from NewError() call (NewError -> captureStack -> runtime.Caller)
const skipNewErrorFrames = 2

func captureStackWithDepth(skip int, maxStackDepth int) []TraceContext {
	rawFrames := captureRawStackWithDepth(skip, maxStackDepth)
	return filterStack(rawFrames)
}

func captureRawStackWithDepth(skip int, maxStackDepth int) []TraceContext {
	maxStackDepth = clampStackDepth(maxStackDepth)
	frames := make([]TraceContext, 0, maxStackDepth)

	for i := skip; i < skip+maxStackDepth; i++ {
		pc, file, line, ok := runtime.Caller(i)
		if !ok {
			break
		}

		fn := runtime.FuncForPC(pc)
		if fn == nil {
			continue
		}

		fnName := fn.Name()
		pkg, function := parseFuncName(fnName)

		frames = append(frames, TraceContext{
			File:     formatFilePath(file),
			Line:     line,
			Package:  pkg,
			Function: function,
		})
	}

	return frames
}

// filterStack removes noise frames from runtime and errorkit package
func filterStack(frames []TraceContext) []TraceContext {
	filtered := make([]TraceContext, 0, len(frames))

	for _, frame := range frames {
		// Build full function name for filtering
		fullName := frame.Package + "." + frame.Function

		// Filter out runtime and errorkit frames
		if strings.HasPrefix(fullName, "runtime.") ||
			strings.HasPrefix(fullName, "errorkit.") {
			continue
		}

		filtered = append(filtered, frame)
	}

	return filtered
}

// parseFuncName extracts package and function from runtime.Func.Name()
// Example in: "github.com/user/project/service.(*MyType).Method"
// Returns: pkg="service", function="(*MyType).Method"
func parseFuncName(fullName string) (pkg string, function string) {
	// Find the last slash to get the final segment
	lastSlash := -1
	for i := len(fullName) - 1; i >= 0; i-- {
		if fullName[i] == '/' {
			lastSlash = i
			break
		}
	}

	// Get the segment after last slash (or full name if no slash)
	segment := fullName
	if lastSlash != -1 {
		segment = fullName[lastSlash+1:]
	}

	// Find the first dot which separates package from function
	firstDot := -1
	for i := 0; i < len(segment); i++ {
		if segment[i] == '.' {
			firstDot = i
			break
		}
	}

	// If no dot found, return segment as both package and function
	if firstDot == -1 {
		return segment, segment
	}

	// Split at first dot
	pkg = segment[:firstDot]
	function = segment[firstDot+1:]

	return pkg, function
}

// formatFilePath strips the absolute path to a project-relative path
func formatFilePath(file string) string {
	const projectRoot = "market-ecosystem/"
	if idx := strings.LastIndex(file, projectRoot); idx != -1 {
		return "./" + file[idx+len(projectRoot):]
	}
	return file
}
