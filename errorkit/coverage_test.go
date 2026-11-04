package errorkit

import (
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestInitFromEnvSetsStackDepth(t *testing.T) {
	t.Cleanup(func() { SetMaxStackDepth(defaultMaxStackDepth) })

	tests := []struct {
		name  string
		value string
		want  int
	}{
		{name: "valid depth", value: "12", want: 12},
		{name: "clamped high", value: "1000", want: 100},
		{name: "clamped low", value: "0", want: 1},
		{name: "unset keeps the current depth", value: "", want: defaultMaxStackDepth},
		{name: "not a number keeps the current depth", value: "deep", want: defaultMaxStackDepth},
		{name: "trailing garbage keeps the current depth", value: "12abc", want: defaultMaxStackDepth},
		{name: "surrounding spaces are ignored", value: " 20 ", want: 20},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			SetMaxStackDepth(defaultMaxStackDepth)
			t.Setenv("ERROR_KIT_MAX_STACK", tc.value)

			InitFromEnv()

			if got := currentMaxStackDepth(); got != tc.want {
				t.Fatalf("depth after InitFromEnv(%q) = %d, want %d", tc.value, got, tc.want)
			}
		})
	}
}

func TestMinMaxULIDRoundTrip(t *testing.T) {
	ts := time.Date(2026, 11, 3, 10, 0, 0, 123_000_000, time.UTC)
	minID, maxID := MinULID(ts), MaxULID(ts)

	for _, id := range []string{minID, maxID} {
		if !ValidateULID(id) {
			t.Fatalf("ValidateULID(%q) = false", id)
		}
		if got := ParseULID(id); !got.Equal(ts) {
			t.Fatalf("ParseULID(%q) = %v, want %v", id, got, ts)
		}
	}
	if minID >= maxID {
		t.Fatalf("MinULID %q must sort before MaxULID %q", minID, maxID)
	}
	if !strings.HasSuffix(minID, strings.Repeat("0", 16)) || !strings.HasSuffix(maxID, strings.Repeat("Z", 16)) {
		t.Fatalf("random parts: min %q, max %q", minID, maxID)
	}

	id := generateULID()
	at := ParseULID(id)
	if id < MinULID(at) || id > MaxULID(at) {
		t.Fatalf("generated %q outside [%q, %q]", id, MinULID(at), MaxULID(at))
	}
}

func TestULIDsAreStrictlyIncreasing(t *testing.T) {
	previous := generateULID()
	for range 10_000 {
		id := generateULID()
		if id <= previous {
			t.Fatalf("generateULID() = %q after %q, want strictly increasing", id, previous)
		}
		previous = id
	}
}

func TestFormatColoredIncludesAllSections(t *testing.T) {
	EnableColors()
	appErr := NewError(ERR_INTERNAL).With(
		WithReason("reason text"),
		WithWrapped(errString("wrapped text")),
		WithPayload(map[string]int{"n": 1}),
		WithTraceID("trace-1"),
	)

	out := NewStackFormatter().FormatColored(appErr)

	for _, want := range []string{"\033[", "ERR_INTERNAL", "reason text", "wrapped text", "map[n:1]", appErr.ID, "trace-1", "Stack Trace:"} {
		if !strings.Contains(out, want) {
			t.Errorf("FormatColored output lacks %q:\n%s", want, out)
		}
	}

	plain := NewStackFormatterWithConfig(FormatterConfig{ColorsEnabled: false}).Format(appErr)
	if strings.Contains(plain, "\033[") {
		t.Errorf("formatter with colors disabled wrote ANSI codes:\n%q", plain)
	}
	colored := NewStackFormatterWithConfig(FormatterConfig{ColorsEnabled: true}).Format(appErr)
	if !strings.Contains(colored, "\033[") {
		t.Errorf("formatter with colors enabled wrote no ANSI codes")
	}
}

func TestDisableColorsOverridesFormatterConfig(t *testing.T) {
	DisableColors()
	t.Cleanup(EnableColors)

	out := NewStackFormatterWithConfig(FormatterConfig{ColorsEnabled: true}).Format(NewError(ERR_INTERNAL))
	if strings.Contains(out, "\033[") {
		t.Fatalf("DisableColors must win over FormatterConfig, got %q", out)
	}
}

type errString string

func (e errString) Error() string { return string(e) }

func BenchmarkPrettyJSON(b *testing.B) {
	appErr := NewError(ERR_INTERNAL).With(WithReason("boom"), WithPayload(map[string]int{"n": 1}))
	b.ReportAllocs()
	for b.Loop() {
		_ = appErr.PrettyJSON()
	}
}

func BenchmarkPublicJSON(b *testing.B) {
	appErr := NewError(ERR_INTERNAL).With(WithReason("boom"))
	b.ReportAllocs()
	for b.Loop() {
		_ = appErr.PublicJSON()
	}
}

// TestTraceContextForDottedPackagePath: the Go linker escapes '.' in the
// last import path element as "%2e" in runtime function names (a package
// at example.com/m/pkg.v2 reports "example.com/m/pkg%2ev2.(*T).Where").
// The trace must show the real import path, not the escaped one.
func TestTraceContextForDottedPackagePath(t *testing.T) {
	tests := []struct {
		function, file          string
		wantPkgPath, wantFunc   string
		wantPackage, wantModule string
	}{
		{
			function: "gopkg.in/yaml%2ev3.(*Decoder).Decode", file: "/home/u/go/pkg/mod/gopkg.in/yaml.v3@v3.0.1/yaml.go",
			wantPkgPath: "gopkg.in/yaml.v3", wantFunc: "(*Decoder).Decode",
			wantPackage: "yaml.v3", wantModule: "gopkg.in/yaml.v3/yaml.go",
		},
		{
			function: "example.com/m/pkg%2ev2.Frames", file: "/src/m/pkg.v2/p.go",
			wantPkgPath: "example.com/m/pkg.v2", wantFunc: "Frames",
			wantPackage: "pkg.v2", wantModule: "example.com/m/pkg.v2/p.go",
		},
		{
			function: "github.com/acme/svc/handler.(*H).Serve", file: "/build/handler/users.go",
			wantPkgPath: "github.com/acme/svc/handler", wantFunc: "(*H).Serve",
			wantPackage: "handler", wantModule: "github.com/acme/svc/handler/users.go",
		},
		{
			function: "main.main", file: "/build/main.go",
			wantPkgPath: "main", wantFunc: "main",
			wantPackage: "main", wantModule: "main/main.go",
		},
	}
	for _, tc := range tests {
		t.Run(tc.function, func(t *testing.T) {
			pkgPath, function := splitFuncName(tc.function)
			if pkgPath != tc.wantPkgPath || function != tc.wantFunc {
				t.Fatalf("splitFuncName(%q) = (%q, %q), want (%q, %q)", tc.function, pkgPath, function, tc.wantPkgPath, tc.wantFunc)
			}
			got := newTraceContext(runtime.Frame{Function: tc.function, File: tc.file, Line: 7})
			if got.Package != tc.wantPackage || got.File != tc.wantModule || got.Function != tc.wantFunc {
				t.Fatalf("newTraceContext = %+v, want package %q, file %q, function %q", got, tc.wantPackage, tc.wantModule, tc.wantFunc)
			}
		})
	}
}
