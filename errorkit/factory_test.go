package errorkit

import (
	"strings"
	"testing"
)

func TestFactoryUsesConfigWithoutMutatingGlobals(t *testing.T) {
	oldMaxStackDepth := currentMaxStackDepth()
	t.Cleanup(func() {
		SetMaxStackDepth(oldMaxStackDepth)
	})

	factory := NewFactory(Config{
		Registry:      NewDefaultRegistry(),
		MaxStackDepth: 2,
	})

	err := factory.NewError(ERR_INTERNAL)
	if len(err.Trace) > 2 {
		t.Fatalf("factory trace length = %d, want <= 2", len(err.Trace))
	}
	if got := currentMaxStackDepth(); got != oldMaxStackDepth {
		t.Fatalf("factory mutated global stack depth: got %d, want %d", got, oldMaxStackDepth)
	}
}

func TestStackFormatterConfigDoesNotMutateGlobalColors(t *testing.T) {
	oldColorsDisabled := colorsDisabled.Load()
	t.Cleanup(func() {
		colorsDisabled.Store(oldColorsDisabled)
	})

	EnableColors()
	err := NewError(ERR_INTERNAL)
	formatter := NewStackFormatterWithConfig(FormatterConfig{ColorsEnabled: false})

	output := formatter.Format(err)
	if strings.Contains(output, "\033[") {
		t.Fatalf("configured plain formatter emitted ANSI output: %q", output)
	}
	if colorsDisabled.Load() {
		t.Fatal("configured formatter mutated global color setting")
	}
}
