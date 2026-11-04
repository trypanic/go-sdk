package errorkit

import (
	"errors"
	"fmt"
	"maps"
	"slices"
	"sync"
)

// ==============================
// Metadata Validation
// ==============================

// ErrInvalidMetadata is returned when registration is rejected due to missing
// or invalid required fields.
var ErrInvalidMetadata = errors.New("errorkit: invalid metadata")

// ErrMetadataConflict is returned when a code is registered twice with
// different metadata.
var ErrMetadataConflict = errors.New("errorkit: metadata conflict")

// ValidateMetadata reports whether meta has all required fields populated.
// Returns nil when meta is acceptable for registration.
func ValidateMetadata(meta Metadata) error {
	if meta.Code == "" {
		return fmt.Errorf("%w: code is empty", ErrInvalidMetadata)
	}
	if meta.Type == "" {
		return fmt.Errorf("%w: %s missing type", ErrInvalidMetadata, meta.Code)
	}
	if meta.Group == "" {
		return fmt.Errorf("%w: %s missing group", ErrInvalidMetadata, meta.Code)
	}
	if meta.Category == "" {
		return fmt.Errorf("%w: %s missing category", ErrInvalidMetadata, meta.Code)
	}
	if meta.Description == "" {
		return fmt.Errorf("%w: %s missing description", ErrInvalidMetadata, meta.Code)
	}
	if meta.HTTPStatus < 100 || meta.HTTPStatus > 599 {
		return fmt.Errorf("%w: %s invalid HTTP status %d", ErrInvalidMetadata, meta.Code, meta.HTTPStatus)
	}
	return nil
}

// ==============================
// Bulk Registration (global)
// ==============================

// RegisterMany validates and registers metadata into the package-global
// registry. Identical duplicate metadata is a no-op. Conflicting metadata for
// the same code (already registered or earlier in the batch) returns
// ErrMetadataConflict. Invalid metadata returns ErrInvalidMetadata.
//
// The batch is all-or-nothing: the whole batch is checked first and nothing
// is registered when any entry is rejected; the first error is returned.
func RegisterMany(metadata ...Metadata) error {
	errorRegistryMu.Lock()
	defer errorRegistryMu.Unlock()

	return registerAll(errorRegistry, metadata)
}

// registerAll checks every entry of batch against m and against the earlier
// entries of batch, and stores the batch only if all of them pass. The
// caller holds the lock guarding m.
func registerAll(m map[ErrorCode]Metadata, batch []Metadata) error {
	for i, meta := range batch {
		if err := ValidateMetadata(meta); err != nil {
			return err
		}
		existing, ok := m[meta.Code]
		if !ok {
			for _, earlier := range batch[:i] {
				if earlier.Code == meta.Code {
					existing, ok = earlier, true
					break
				}
			}
		}
		if ok && existing != meta {
			return fmt.Errorf("%w: %s", ErrMetadataConflict, meta.Code)
		}
	}
	for _, meta := range batch {
		m[meta.Code] = meta
	}
	return nil
}

// MustRegister calls RegisterMany and panics on validation or conflict
// failure. Intended for package init() or top-level var blocks where a
// returned error cannot be handled by the caller.
//
// Panic is deliberate and follows the stdlib Must* convention (e.g.
// regexp.MustCompile, template.Must): the non-panicking variant
// RegisterMany is available for callers that need to handle errors
// gracefully. A failure here indicates a programmer error (conflicting
// or invalid metadata) that must surface at startup — silently
// returning would let a corrupt registry serve every later lookup.
func MustRegister(metadata ...Metadata) {
	if err := RegisterMany(metadata...); err != nil {
		// nosemgrep: no-panic-outside-init -- stdlib-style Must* helper; see doc comment above.
		panic(err)
	}
}

// ==============================
// Bulk Registration (Registry)
// ==============================

// RegisterMany validates and registers metadata into this registry, with the
// same all-or-nothing semantics as the package-level RegisterMany.
func (r *Registry) RegisterMany(metadata ...Metadata) error {
	if r == nil {
		return errors.New("errorkit: nil registry")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	return registerAll(r.metadata, metadata)
}

// MustRegister calls Registry.RegisterMany and panics on failure.
// Intended for package init() or top-level var blocks where a returned
// error cannot be handled by the caller.
//
// Panic is deliberate and follows the stdlib Must* convention (e.g.
// regexp.MustCompile, template.Must): the non-panicking variant
// Registry.RegisterMany is available for callers that need to handle
// errors gracefully. A failure here indicates a programmer error
// (conflicting or invalid metadata) that must surface at startup —
// silently returning would let a corrupt registry serve every later
// lookup.
func (r *Registry) MustRegister(metadata ...Metadata) {
	if err := r.RegisterMany(metadata...); err != nil {
		// nosemgrep: no-panic-outside-init -- stdlib-style Must* helper; see doc comment above.
		panic(err)
	}
}

// ==============================
// Merge
// ==============================

// Merge copies metadata from other into r using RegisterMany semantics: when
// any entry is invalid or conflicts with r, nothing is copied and the first
// error (in code order) is returned.
func (r *Registry) Merge(other *Registry) error {
	if r == nil {
		return errors.New("errorkit: nil registry")
	}
	if other == nil {
		return nil
	}

	other.mu.RLock()
	snapshot := make([]Metadata, 0, len(other.metadata))
	for _, code := range slices.Sorted(maps.Keys(other.metadata)) {
		snapshot = append(snapshot, other.metadata[code])
	}
	other.mu.RUnlock()

	return r.RegisterMany(snapshot...)
}

// ==============================
// Override
// ==============================

// OverrideMetadata replaces metadata for an existing code in the package-global
// registry. Returns false when the code is not yet registered, or when the
// updated metadata fails ValidateMetadata or changes Code; the stored entry
// is then left unchanged.
//
// The updater receives a copy of the current entry and runs without holding
// the registry lock, so it may call errorkit (for example GetMetadata). If
// the entry changes concurrently, the updater runs again on the new value.
//
// Production code should register canonical metadata via MustRegister. Use
// OverrideMetadata only for tests or runtime overrides where conflict
// detection must be bypassed deliberately.
func OverrideMetadata(code ErrorCode, updater func(*Metadata)) bool {
	return overrideMetadata(&errorRegistryMu, errorRegistry, code, updater)
}

// OverrideMetadata replaces metadata for an existing code in this registry.
// It has the same semantics as the package-level OverrideMetadata.
func (r *Registry) OverrideMetadata(code ErrorCode, updater func(*Metadata)) bool {
	if r == nil {
		return false
	}
	return overrideMetadata(&r.mu, r.metadata, code, updater)
}

// overrideMetadata applies updater to a copy of m[code] outside the lock,
// validates the result and stores it only if m[code] did not change in the
// meantime (optimistic concurrency); otherwise it retries.
func overrideMetadata(mu *sync.RWMutex, m map[ErrorCode]Metadata, code ErrorCode, updater func(*Metadata)) bool {
	for {
		mu.RLock()
		current, exists := m[code]
		mu.RUnlock()
		if !exists {
			return false
		}

		updated := current
		updater(&updated)
		if updated.Code != code || ValidateMetadata(updated) != nil {
			return false
		}

		mu.Lock()
		if m[code] == current {
			m[code] = updated
			mu.Unlock()
			return true
		}
		mu.Unlock()
	}
}
