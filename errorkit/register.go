package errorkit

import (
	"errors"
	"fmt"
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
// the same code returns ErrMetadataConflict. Invalid metadata returns
// ErrInvalidMetadata. Stops at the first error.
func RegisterMany(metadata ...Metadata) error {
	errorRegistryMu.Lock()
	defer errorRegistryMu.Unlock()

	for _, meta := range metadata {
		if err := ValidateMetadata(meta); err != nil {
			return err
		}
		if existing, ok := ErrorRegistry[meta.Code]; ok {
			if existing == meta {
				continue
			}
			return fmt.Errorf("%w: %s", ErrMetadataConflict, meta.Code)
		}
		ErrorRegistry[meta.Code] = meta
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

// RegisterMany validates and registers metadata into this registry. Identical
// duplicate metadata is a no-op. Conflicting metadata for the same code returns
// ErrMetadataConflict. Invalid metadata returns ErrInvalidMetadata. Stops at
// the first error.
func (r *Registry) RegisterMany(metadata ...Metadata) error {
	if r == nil {
		return errors.New("errorkit: nil registry")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	for _, meta := range metadata {
		if err := ValidateMetadata(meta); err != nil {
			return err
		}
		if existing, ok := r.metadata[meta.Code]; ok {
			if existing == meta {
				continue
			}
			return fmt.Errorf("%w: %s", ErrMetadataConflict, meta.Code)
		}
		r.metadata[meta.Code] = meta
	}
	return nil
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

// Merge copies metadata from other into r using RegisterMany semantics.
// Returns the first conflict or validation error encountered.
func (r *Registry) Merge(other *Registry) error {
	if r == nil {
		return errors.New("errorkit: nil registry")
	}
	if other == nil {
		return nil
	}

	other.mu.RLock()
	snapshot := make([]Metadata, 0, len(other.metadata))
	for _, meta := range other.metadata {
		snapshot = append(snapshot, meta)
	}
	other.mu.RUnlock()

	return r.RegisterMany(snapshot...)
}

// ==============================
// Override
// ==============================

// OverrideMetadata replaces metadata for an existing code in the package-global
// registry. Returns false when the code is not yet registered.
//
// Production code should register canonical metadata via MustRegister. Use
// OverrideMetadata only for tests or runtime overrides where conflict
// detection must be bypassed deliberately.
func OverrideMetadata(code ErrorCode, updater func(*Metadata)) bool {
	errorRegistryMu.Lock()
	defer errorRegistryMu.Unlock()

	meta, exists := ErrorRegistry[code]
	if !exists {
		return false
	}

	updater(&meta)
	ErrorRegistry[code] = meta
	return true
}

// OverrideMetadata replaces metadata for an existing code in this registry.
// Returns false when the code is not yet registered.
func (r *Registry) OverrideMetadata(code ErrorCode, updater func(*Metadata)) bool {
	if r == nil {
		return false
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	meta, exists := r.metadata[code]
	if !exists {
		return false
	}

	updater(&meta)
	r.metadata[code] = meta
	return true
}
