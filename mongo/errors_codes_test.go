package mongodb

import (
	"testing"

	"github.com/trypanic/go-sdk/errorkit"
)

func TestErrorMetadataRegisteredInGlobalRegistry(t *testing.T) {
	for _, want := range ErrorMetadata {
		got, ok := errorkit.GetMetadata(want.Code)
		if !ok {
			t.Fatalf("code %s not registered in global registry", want.Code)
		}
		if got != want {
			t.Fatalf("metadata mismatch for %s:\n  got  %+v\n  want %+v", want.Code, got, want)
		}
	}
}

func TestRegisterErrorsIdempotent(t *testing.T) {
	if err := RegisterErrors(nil); err != nil {
		t.Fatalf("re-registering identical metadata into global registry: %v", err)
	}
}

func TestRegisterErrorsIntoIsolatedRegistry(t *testing.T) {
	registry := errorkit.NewRegistry()
	if err := RegisterErrors(registry); err != nil {
		t.Fatalf("RegisterErrors(isolated): %v", err)
	}
	for _, want := range ErrorMetadata {
		got, ok := registry.GetMetadata(want.Code)
		if !ok {
			t.Fatalf("code %s missing from isolated registry", want.Code)
		}
		if got != want {
			t.Fatalf("isolated metadata mismatch for %s:\n  got  %+v\n  want %+v", want.Code, got, want)
		}
	}
}
