package errorkit

import (
	"errors"
	"strings"
	"testing"
)

const (
	ERR_REGISTER_TEST_A      ErrorCode = "ERR_REGISTER_TEST_A"
	ERR_REGISTER_TEST_B      ErrorCode = "ERR_REGISTER_TEST_B"
	ERR_REGISTER_TEST_INVAL  ErrorCode = "ERR_REGISTER_TEST_INVAL"
	ERR_REGISTER_TEST_CONFL  ErrorCode = "ERR_REGISTER_TEST_CONFL"
	ERR_REGISTER_TEST_STRICT ErrorCode = "ERR_REGISTER_TEST_STRICT"
)

func metaA() Metadata {
	return Metadata{
		Code:        ERR_REGISTER_TEST_A,
		Type:        ErrorTypeInternal,
		Group:       GroupUnknown,
		Category:    "test",
		Description: "register test A",
		HTTPStatus:  500,
		Retriable:   false,
	}
}

func metaB() Metadata {
	return Metadata{
		Code:        ERR_REGISTER_TEST_B,
		Type:        ErrorTypeExternal,
		Group:       GroupUnknown,
		Category:    "test",
		Description: "register test B",
		HTTPStatus:  503,
		Retriable:   true,
	}
}

func TestRegisterMany_AcceptsValidMetadata(t *testing.T) {
	if err := RegisterMany(metaA(), metaB()); err != nil {
		t.Fatalf("RegisterMany returned %v", err)
	}
	if _, ok := GetMetadata(ERR_REGISTER_TEST_A); !ok {
		t.Fatal("metaA not registered")
	}
	if _, ok := GetMetadata(ERR_REGISTER_TEST_B); !ok {
		t.Fatal("metaB not registered")
	}
}

func TestRegisterMany_IdenticalDuplicateIsNoop(t *testing.T) {
	if err := RegisterMany(metaA()); err != nil {
		t.Fatalf("first call: %v", err)
	}
	if err := RegisterMany(metaA()); err != nil {
		t.Fatalf("second call (identical) returned %v, want nil", err)
	}
}

func TestRegisterMany_RejectsInvalidMetadata(t *testing.T) {
	cases := []struct {
		name string
		mut  func(*Metadata)
	}{
		{"empty code", func(m *Metadata) { m.Code = "" }},
		{"empty type", func(m *Metadata) { m.Type = "" }},
		{"empty group", func(m *Metadata) { m.Group = "" }},
		{"empty category", func(m *Metadata) { m.Category = "" }},
		{"empty description", func(m *Metadata) { m.Description = "" }},
		{"bad status low", func(m *Metadata) { m.HTTPStatus = 50 }},
		{"bad status high", func(m *Metadata) { m.HTTPStatus = 700 }},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			meta := Metadata{
				Code:        ERR_REGISTER_TEST_INVAL,
				Type:        ErrorTypeInternal,
				Group:       GroupUnknown,
				Category:    "test",
				Description: "invalid",
				HTTPStatus:  500,
			}
			tc.mut(&meta)
			err := RegisterMany(meta)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !errors.Is(err, ErrInvalidMetadata) {
				t.Fatalf("error not ErrInvalidMetadata: %v", err)
			}
		})
	}
}

func TestRegisterMany_RejectsConflict(t *testing.T) {
	meta := Metadata{
		Code:        ERR_REGISTER_TEST_CONFL,
		Type:        ErrorTypeInternal,
		Group:       GroupUnknown,
		Category:    "test",
		Description: "conflict v1",
		HTTPStatus:  500,
	}
	if err := RegisterMany(meta); err != nil {
		t.Fatalf("first call: %v", err)
	}

	conflict := meta
	conflict.Description = "conflict v2"
	err := RegisterMany(conflict)
	if err == nil {
		t.Fatal("expected conflict error, got nil")
	}
	if !errors.Is(err, ErrMetadataConflict) {
		t.Fatalf("error not ErrMetadataConflict: %v", err)
	}
}

func TestMustRegister_PanicsOnFailure(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic, got none")
		}
	}()
	MustRegister(Metadata{Code: ""}) // invalid
}

func TestRegistryRegisterMany_IsolatedFromGlobal(t *testing.T) {
	const code ErrorCode = "ERR_REGISTRY_REGISTER_MANY_ISOLATED"
	registry := NewRegistry()
	if err := registry.RegisterMany(Metadata{
		Code:        code,
		Type:        ErrorTypeInternal,
		Group:       GroupUnknown,
		Category:    "test",
		Description: "isolated",
		HTTPStatus:  500,
	}); err != nil {
		t.Fatalf("RegisterMany: %v", err)
	}
	if _, ok := GetMetadata(code); ok {
		t.Fatal("isolated registration leaked to global registry")
	}
	if _, ok := registry.GetMetadata(code); !ok {
		t.Fatal("isolated registry missing code")
	}
}

func TestRegistryMerge_CopiesMetadata(t *testing.T) {
	const code ErrorCode = "ERR_REGISTRY_MERGE_SRC"
	src := NewRegistry()
	if err := src.RegisterMany(Metadata{
		Code:        code,
		Type:        ErrorTypeInternal,
		Group:       GroupUnknown,
		Category:    "test",
		Description: "merge src",
		HTTPStatus:  500,
	}); err != nil {
		t.Fatalf("src register: %v", err)
	}

	dst := NewRegistry()
	if err := dst.Merge(src); err != nil {
		t.Fatalf("Merge: %v", err)
	}
	if _, ok := dst.GetMetadata(code); !ok {
		t.Fatal("merged code missing in dst")
	}
}

func TestRegistryMerge_SurfacesConflict(t *testing.T) {
	const code ErrorCode = "ERR_REGISTRY_MERGE_CONFLICT"
	base := Metadata{
		Code:        code,
		Type:        ErrorTypeInternal,
		Group:       GroupUnknown,
		Category:    "test",
		Description: "v1",
		HTTPStatus:  500,
	}

	dst := NewRegistry()
	if err := dst.RegisterMany(base); err != nil {
		t.Fatalf("dst register: %v", err)
	}

	src := NewRegistry()
	conflict := base
	conflict.Description = "v2"
	if err := src.RegisterMany(conflict); err != nil {
		t.Fatalf("src register: %v", err)
	}

	err := dst.Merge(src)
	if err == nil {
		t.Fatal("expected conflict error from Merge")
	}
	if !errors.Is(err, ErrMetadataConflict) {
		t.Fatalf("expected ErrMetadataConflict, got %v", err)
	}
}

func TestOverrideMetadata_ReplacesExisting(t *testing.T) {
	const code ErrorCode = "ERR_OVERRIDE_TARGET"
	if err := RegisterMany(Metadata{
		Code:        code,
		Type:        ErrorTypeInternal,
		Group:       GroupUnknown,
		Category:    "test",
		Description: "before",
		HTTPStatus:  500,
	}); err != nil {
		t.Fatalf("register: %v", err)
	}

	ok := OverrideMetadata(code, func(m *Metadata) {
		m.Description = "after"
	})
	if !ok {
		t.Fatal("OverrideMetadata returned false on registered code")
	}

	meta, _ := GetMetadata(code)
	if meta.Description != "after" {
		t.Fatalf("Description = %q, want after", meta.Description)
	}
}

func TestOverrideMetadata_ReturnsFalseForUnknown(t *testing.T) {
	const code ErrorCode = "ERR_OVERRIDE_UNKNOWN_NEVER_REGISTERED"
	called := false
	ok := OverrideMetadata(code, func(m *Metadata) { called = true })
	if ok {
		t.Fatal("OverrideMetadata should return false for unknown code")
	}
	if called {
		t.Fatal("updater should not be called for unknown code")
	}
}

func TestFactory_StrictMetadataMarksUnknown(t *testing.T) {
	factory := NewFactory(Config{StrictMetadata: true})
	const code ErrorCode = "ERR_STRICT_NEVER_REGISTERED"
	err := factory.NewError(code)
	if err.Reason != StrictMetadataReason {
		t.Fatalf("Reason = %q, want %q", err.Reason, StrictMetadataReason)
	}
	if !strings.Contains(err.Reason, "metadata not registered") {
		t.Fatalf("Reason missing marker: %q", err.Reason)
	}
}

func TestFactory_StrictMetadataDoesNotMarkKnown(t *testing.T) {
	factory := NewFactory(Config{StrictMetadata: true})
	err := factory.NewError(ERR_INTERNAL)
	if err.Reason == StrictMetadataReason {
		t.Fatalf("known code marked as unregistered: %q", err.Reason)
	}
}
