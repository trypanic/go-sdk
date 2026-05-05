package marshal

import (
	"errors"
	"testing"

	"github.com/trypanic/go-sdk/errorkit"
)

func TestNoEscape_UnsupportedType_ReturnsAppError(t *testing.T) {
	_, err := NoEscape(make(chan int))
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	var appErr *errorkit.AppError
	if !errors.As(err, &appErr) {
		t.Fatalf("expected *errorkit.AppError, got %T", err)
	}
	if appErr.Code() != errorkit.ERR_INTERNAL {
		t.Fatalf("expected code %s, got %s", errorkit.ERR_INTERNAL, appErr.Code())
	}
}

func TestNoEscapeAddsTrailingNewline(t *testing.T) {
	t.Parallel()

	got, err := NoEscape(map[string]int{"a": 1})
	if err != nil {
		t.Fatalf("NoEscape: %v", err)
	}
	if got == "" || got[len(got)-1] != '\n' {
		t.Fatalf("expected trailing newline, got %q", got)
	}
}

func TestNoEscapeNoNewlineStripsTrailingNewline(t *testing.T) {
	t.Parallel()

	got, err := NoEscapeNoNewline(map[string]int{"a": 1})
	if err != nil {
		t.Fatalf("NoEscapeNoNewline: %v", err)
	}
	if got == "" || got[len(got)-1] == '\n' {
		t.Fatalf("expected no trailing newline, got %q", got)
	}
}

func TestNoEscapeBytesNoNewlineStripsTrailingNewline(t *testing.T) {
	t.Parallel()

	got, err := NoEscapeBytesNoNewline(map[string]int{"a": 1})
	if err != nil {
		t.Fatalf("NoEscapeBytesNoNewline: %v", err)
	}
	if len(got) == 0 || got[len(got)-1] == '\n' {
		t.Fatalf("expected no trailing newline, got %q", got)
	}
}
