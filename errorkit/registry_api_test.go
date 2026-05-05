package errorkit

import "testing"

const ERR_REGISTRY_ISOLATED ErrorCode = "ERR_REGISTRY_ISOLATED"

func TestRegistryIsIsolatedFromGlobalRegistry(t *testing.T) {
	registry := NewRegistry(Metadata{
		Code:        ERR_REGISTRY_ISOLATED,
		Type:        ErrorTypeExternal,
		Group:       GroupUnknown,
		Category:    "test",
		Description: "isolated registry error",
		HTTPStatus:  418,
		Retriable:   true,
	})

	if _, ok := GetMetadata(ERR_REGISTRY_ISOLATED); ok {
		t.Fatal("isolated registry code leaked into global registry")
	}

	meta, ok := registry.GetMetadata(ERR_REGISTRY_ISOLATED)
	if !ok {
		t.Fatal("isolated registry did not return registered metadata")
	}
	if meta.Description != "isolated registry error" {
		t.Fatalf("unexpected metadata description: %q", meta.Description)
	}

	err := registry.NewError(ERR_REGISTRY_ISOLATED)
	if err.Metadata.Description != "isolated registry error" {
		t.Fatalf("Registry.NewError() metadata description = %q", err.Metadata.Description)
	}
	if err.Metadata.HTTPStatus != 418 {
		t.Fatalf("Registry.NewError() HTTPStatus = %d, want 418", err.Metadata.HTTPStatus)
	}
}

func TestDefaultRegistryCopyDoesNotMutateGlobalRegistry(t *testing.T) {
	registry := NewDefaultRegistry()
	registry.MustRegister(Metadata{
		Code:        ERR_REGISTRY_ISOLATED,
		Type:        ErrorTypeInternal,
		Group:       GroupUnknown,
		Category:    "test",
		Description: "default copy local extension",
		HTTPStatus:  499,
		Retriable:   false,
	})

	if _, ok := GetMetadata(ERR_REGISTRY_ISOLATED); ok {
		t.Fatal("default registry copy mutation leaked into global registry")
	}

	err := registry.NewError(ERR_INTERNAL)
	if err.Metadata.Description != "Internal server error" {
		t.Fatalf("default registry copy lost built-in metadata: %q", err.Metadata.Description)
	}
}

func TestRegistryUnknownCodeUsesDefaultMetadata(t *testing.T) {
	const code ErrorCode = "ERR_NOT_REGISTERED_IN_INSTANCE"

	err := NewRegistry().NewError(code)
	if err.ErrCode != code {
		t.Fatalf("ErrCode = %s, want %s", err.ErrCode, code)
	}
	if err.Metadata.Description != "Unknown error" {
		t.Fatalf("Description = %q, want Unknown error", err.Metadata.Description)
	}
}
