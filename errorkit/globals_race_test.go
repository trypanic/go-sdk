package errorkit

import (
	"sync"
	"testing"
)

// The tests in this file must run under -race: every package-level setter
// must be safe to call while other goroutines create and format errors.

func runConcurrently(writer, reader func()) {
	var wg sync.WaitGroup
	wg.Go(func() {
		for range 100 {
			writer()
		}
	})
	wg.Go(func() {
		for range 100 {
			reader()
		}
	})
	wg.Wait()
}

func TestSetMaxStackDepthConcurrentWithNewError(t *testing.T) {
	t.Cleanup(func() { SetMaxStackDepth(defaultMaxStackDepth) })
	runConcurrently(
		func() { SetMaxStackDepth(8) },
		func() { _ = NewError(ERR_INTERNAL) },
	)
}

func TestSetIDGeneratorConcurrentWithNewError(t *testing.T) {
	t.Cleanup(func() { SetIDGenerator(nil) })
	runConcurrently(
		func() { SetIDGenerator(func() string { return "fixed-id" }) },
		func() { _ = NewError(ERR_INTERNAL) },
	)
}

func TestColorTogglesConcurrentWithFormatting(t *testing.T) {
	t.Cleanup(EnableColors)
	err := NewError(ERR_INTERNAL)
	formatter := NewStackFormatter()
	runConcurrently(
		func() { DisableColors(); EnableColors() },
		func() { _ = formatter.FormatColored(err) },
	)
}

func TestRegistryWritesConcurrentWithNewError(t *testing.T) {
	const code ErrorCode = "ERR_GLOBALS_RACE"
	MustRegister(Metadata{
		Code: code, Type: ErrorTypeInternal, Group: GroupUnknown,
		Category: "test", Description: "race", HTTPStatus: 500,
	})
	runConcurrently(
		func() { OverrideMetadata(code, func(m *Metadata) { m.Retriable = !m.Retriable }) },
		func() { _ = NewError(code) },
	)
}

func TestSetIDGeneratorIsUsedAndResettable(t *testing.T) {
	t.Cleanup(func() { SetIDGenerator(nil) })

	SetIDGenerator(func() string { return "custom-id" })
	if got := NewError(ERR_INTERNAL).ID; got != "custom-id" {
		t.Fatalf("ID = %q, want custom-id", got)
	}

	SetIDGenerator(nil)
	if got := NewError(ERR_INTERNAL).ID; !ValidateULID(got) {
		t.Fatalf("ID after reset = %q, want a ULID", got)
	}
}
