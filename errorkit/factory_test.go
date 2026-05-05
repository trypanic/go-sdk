package errorkit

import (
	"strings"
	"testing"
)

func TestFactoryUsesConfigWithoutMutatingGlobals(t *testing.T) {
	oldMaxStackDepth := MaxStackDepth
	t.Cleanup(func() {
		MaxStackDepth = oldMaxStackDepth
	})

	factory := NewFactory(Config{
		Registry:      NewDefaultRegistry(),
		MaxStackDepth: 2,
	})

	err := factory.NewError(ERR_INTERNAL)
	if len(err.Trace) > 2 {
		t.Fatalf("factory trace length = %d, want <= 2", len(err.Trace))
	}
	if MaxStackDepth != oldMaxStackDepth {
		t.Fatalf("factory mutated global MaxStackDepth: got %d, want %d", MaxStackDepth, oldMaxStackDepth)
	}
}

func TestStackFormatterConfigDoesNotMutateGlobalColors(t *testing.T) {
	oldColorsEnabled := ColorsEnabled
	t.Cleanup(func() {
		ColorsEnabled = oldColorsEnabled
	})

	ColorsEnabled = true
	err := NewError(ERR_INTERNAL)
	formatter := NewStackFormatterWithConfig(FormatterConfig{ColorsEnabled: false})

	output := formatter.Format(err)
	if strings.Contains(output, "\033[") {
		t.Fatalf("configured plain formatter emitted ANSI output: %q", output)
	}
	if !ColorsEnabled {
		t.Fatal("configured formatter mutated global ColorsEnabled")
	}
}
