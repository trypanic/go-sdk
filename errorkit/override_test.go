package errorkit

import (
	"testing"
	"time"
)

func overrideTestMeta(code ErrorCode) Metadata {
	return Metadata{
		Code:        code,
		Type:        ErrorTypeInternal,
		Group:       GroupUnknown,
		Category:    "test",
		Description: "override test",
		HTTPStatus:  500,
	}
}

// overrideTarget abstracts the global OverrideMetadata/GetMetadata pair and
// the Registry methods so every case runs against both.
type overrideTarget struct {
	name     string
	register func(Metadata) error
	override func(ErrorCode, func(*Metadata)) bool
	get      func(ErrorCode) (Metadata, bool)
	forget   func(ErrorCode)
}

// forgetGlobalCode removes code from the package-global registry so a test
// that registers and then overrides it can run again (-count, -shuffle).
func forgetGlobalCode(code ErrorCode) {
	errorRegistryMu.Lock()
	defer errorRegistryMu.Unlock()
	delete(errorRegistry, code)
}

func overrideTargets() []overrideTarget {
	r := NewRegistry()
	return []overrideTarget{
		{name: "global", register: func(m Metadata) error { return RegisterMany(m) }, override: OverrideMetadata, get: GetMetadata, forget: forgetGlobalCode},
		{name: "registry", register: func(m Metadata) error { return r.RegisterMany(m) }, override: r.OverrideMetadata, get: r.GetMetadata, forget: func(ErrorCode) {}},
	}
}

// runWithin fails the test when fn does not return within d. A stuck fn is
// left running, so a regression of this test must be run in isolation.
func runWithin(t *testing.T, d time.Duration, fn func()) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		defer close(done)
		fn()
	}()
	select {
	case <-done:
	case <-time.After(d):
		t.Fatalf("call did not return within %v (deadlock)", d)
	}
}

func TestOverrideMetadata_ReentrantUpdaterDoesNotDeadlock(t *testing.T) {
	for _, target := range overrideTargets() {
		t.Run(target.name, func(t *testing.T) {
			code := ErrorCode("ERR_OVERRIDE_REENTRANT_" + target.name)
			if err := target.register(overrideTestMeta(code)); err != nil {
				t.Fatalf("register: %v", err)
			}
			t.Cleanup(func() { target.forget(code) })

			var ok bool
			runWithin(t, 2*time.Second, func() {
				ok = target.override(code, func(m *Metadata) {
					// Deriving an override from registry data re-enters the registry.
					other, _ := target.get(code)
					m.Description = other.Description + " (overridden)"
				})
			})
			if !ok {
				t.Fatal("OverrideMetadata returned false on a registered code")
			}
			meta, _ := target.get(code)
			if meta.Description != "override test (overridden)" {
				t.Fatalf("Description = %q", meta.Description)
			}
		})
	}
}

func TestOverrideMetadata_RejectsInvalidResult(t *testing.T) {
	cases := []struct {
		name string
		mut  func(*Metadata)
	}{
		{name: "changed code", mut: func(m *Metadata) { m.Code = "ERR_SOMETHING_ELSE" }},
		{name: "invalid HTTP status", mut: func(m *Metadata) { m.HTTPStatus = 0 }},
		{name: "empty description", mut: func(m *Metadata) { m.Description = "" }},
	}
	for _, target := range overrideTargets() {
		for _, tc := range cases {
			t.Run(target.name+"/"+tc.name, func(t *testing.T) {
				code := ErrorCode("ERR_OVERRIDE_INVALID_" + target.name)
				want := overrideTestMeta(code)
				if err := target.register(want); err != nil {
					t.Fatalf("register: %v", err)
				}

				if target.override(code, tc.mut) {
					t.Fatal("OverrideMetadata accepted invalid metadata")
				}
				if got, _ := target.get(code); got != want {
					t.Fatalf("stored metadata changed: got %+v, want %+v", got, want)
				}
			})
		}
	}
}
