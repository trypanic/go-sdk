package errorkit

import (
	"path"
	"runtime"
	"strings"
)

// maxCapturedPCs bounds the program counters captured per error: the
// largest allowed stack depth (100) plus room for errorkit's own frames and
// the runtime frames that are filtered out.
const maxCapturedPCs = 128

// errorkitFuncPrefix is the fully qualified function-name prefix of this
// package (for example "github.com/trypanic/go-sdk/errorkit."). Frames with
// this prefix at the top of a captured stack are errorkit internals.
var errorkitFuncPrefix = func() string {
	pc, _, _, _ := runtime.Caller(0)
	name := runtime.FuncForPC(pc).Name()
	pkgPath, _ := splitFuncName(name)
	return pkgPath + "."
}()

// captureStackWithDepth captures up to maxStackDepth frames starting at the
// first frame outside errorkit (the code that created the error). runtime
// frames are dropped.
func captureStackWithDepth(maxStackDepth int) []TraceContext {
	maxStackDepth = clampStackDepth(maxStackDepth)

	var pcs [maxCapturedPCs]uintptr
	// Skip runtime.Callers and captureStackWithDepth itself.
	n := runtime.Callers(2, pcs[:])
	frames := runtime.CallersFrames(pcs[:n])

	trace := make([]TraceContext, 0, min(maxStackDepth, n))
	inErrorkit := true
	for len(trace) < maxStackDepth {
		frame, more := frames.Next()
		if inErrorkit && isErrorkitInternal(frame) {
			if !more {
				break
			}
			continue
		}
		inErrorkit = false

		if frame.Function != "" && !strings.HasPrefix(frame.Function, "runtime.") {
			trace = append(trace, newTraceContext(frame))
		}
		if !more {
			break
		}
	}
	return trace
}

// isErrorkitInternal reports whether frame belongs to errorkit's own
// (non-test) code.
func isErrorkitInternal(frame runtime.Frame) bool {
	return strings.HasPrefix(frame.Function, errorkitFuncPrefix) &&
		!strings.HasSuffix(frame.File, "_test.go")
}

func newTraceContext(frame runtime.Frame) TraceContext {
	pkgPath, function := splitFuncName(frame.Function)
	return TraceContext{
		File:     moduleRelativeFile(pkgPath, frame.File),
		Line:     frame.Line,
		Package:  path.Base(pkgPath),
		Function: function,
	}
}

// splitFuncName splits a runtime function name into its package import path
// and the function part.
// Example: "github.com/user/project/service.(*MyType).Method" returns
// "github.com/user/project/service" and "(*MyType).Method".
func splitFuncName(fullName string) (pkgPath, function string) {
	lastSlash := strings.LastIndexByte(fullName, '/')
	dir, segment := "", fullName
	if lastSlash >= 0 {
		dir, segment = fullName[:lastSlash+1], fullName[lastSlash+1:]
	}
	pkg, function, found := strings.Cut(segment, ".")
	if !found {
		return fullName, segment
	}
	return dir + pkg, function
}

// moduleRelativeFile rewrites an absolute build path into the file's import
// path ("github.com/acme/svc/handler/users.go"), so traces do not expose
// the build host's directory layout. A Go package lives in one directory,
// so the package import path plus the file name identifies the file. The
// "_test" suffix of external test packages is dropped.
func moduleRelativeFile(pkgPath, file string) string {
	if pkgPath == "" {
		return path.Base(file)
	}
	return strings.TrimSuffix(pkgPath, "_test") + "/" + path.Base(file)
}
