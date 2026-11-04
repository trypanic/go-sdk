package errorkit_test

import (
	"strings"
	"testing"

	"github.com/trypanic/go-sdk/errorkit"
)

//go:noinline
func newErrorFromHelper() *errorkit.AppError {
	return errorkit.NewError(errorkit.ERR_INTERNAL)
}

func TestStackFirstFrameIsCaller(t *testing.T) {
	cases := []struct {
		name  string
		depth int
	}{
		{name: "depth 1", depth: 1},
		{name: "depth 2", depth: 2},
		{name: "default depth", depth: 32},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			errorkit.SetMaxStackDepth(tc.depth)
			t.Cleanup(func() { errorkit.SetMaxStackDepth(32) })

			trace := newErrorFromHelper().Trace
			if len(trace) == 0 {
				t.Fatalf("depth %d captured no frame", tc.depth)
			}
			if len(trace) > tc.depth {
				t.Fatalf("captured %d frames, want <= %d", len(trace), tc.depth)
			}
			first := trace[0]
			if first.Function != "newErrorFromHelper" || first.Package != "errorkit_test" {
				t.Fatalf("first frame = %+v, want the caller newErrorFromHelper", first)
			}
		})
	}
}

func TestFactoryStackFirstFrameIsCaller(t *testing.T) {
	factory := errorkit.NewFactory(errorkit.Config{MaxStackDepth: 1})
	trace := factory.NewError(errorkit.ERR_INTERNAL).Trace
	if len(trace) != 1 {
		t.Fatalf("captured %d frames, want 1", len(trace))
	}
	if !strings.HasPrefix(trace[0].Function, "TestFactoryStackFirstFrameIsCaller") {
		t.Fatalf("first frame = %+v, want the test function", trace[0])
	}
}

func TestStackFilePathsAreModuleRelative(t *testing.T) {
	trace := newErrorFromHelper().Trace
	if len(trace) == 0 {
		t.Fatal("no frame captured")
	}
	const want = "github.com/trypanic/go-sdk/errorkit/stack_test.go"
	if got := trace[0].File; got != want {
		t.Fatalf("File = %q, want %q", got, want)
	}
	for _, frame := range trace {
		if strings.HasPrefix(frame.File, "/") {
			t.Fatalf("absolute build path leaked in frame %+v", frame)
		}
		if frame.Package == "runtime" {
			t.Fatalf("runtime frame not filtered: %+v", frame)
		}
	}
}

func BenchmarkNewError(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		_ = newErrorFromHelper()
	}
}
