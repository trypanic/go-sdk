package errorkit

import (
	"maps"
	"slices"
	"sync"
)

var (
	errorRegistryMu sync.RWMutex
	// errorRegistry is the package-global registry of error metadata. It is
	// guarded by errorRegistryMu; use GetMetadata, RegisterMany,
	// OverrideMetadata or an isolated Registry to access it.
	errorRegistry = map[ErrorCode]Metadata{
		// ==============================
		// General Errors
		// ==============================
		ERR_UNKNOWN: {
			Code:        ERR_UNKNOWN,
			Type:        ErrorTypeInternal,
			Group:       GroupUnknown,
			Category:    "general",
			Description: "Unknown error occurred",
			HTTPStatus:  500,
			Retriable:   false,
		},
		ERR_INTERNAL: {
			Code:        ERR_INTERNAL,
			Type:        ErrorTypeInternal,
			Group:       GroupSystem,
			Category:    "system",
			Description: "Internal server error",
			HTTPStatus:  500,
			Retriable:   false,
		},

		// ==============================
		// Validation & Data Errors
		// ==============================
		ERR_VALIDATION: {
			Code:        ERR_VALIDATION,
			Type:        ErrorTypeInternal,
			Group:       GroupData,
			Category:    "validation",
			Description: "Validation failed",
			HTTPStatus:  400,
			Retriable:   false,
		},
		ERR_VALIDATION_INVALID_FORMAT: {
			Code:        ERR_VALIDATION_INVALID_FORMAT,
			Type:        ErrorTypeInternal,
			Group:       GroupData,
			Category:    "validation",
			Description: "Field has invalid format",
			HTTPStatus:  400,
			Retriable:   false,
		},
		ERR_VALIDATION_MISSING_FIELD: {
			Code:        ERR_VALIDATION_MISSING_FIELD,
			Type:        ErrorTypeInternal,
			Group:       GroupData,
			Category:    "validation",
			Description: "Required field is missing",
			HTTPStatus:  400,
			Retriable:   false,
		},
		ERR_VALIDATION_INCONSISTENT: {
			Code:        ERR_VALIDATION_INCONSISTENT,
			Type:        ErrorTypeInternal,
			Group:       GroupData,
			Category:    "validation",
			Description: "Fields are inconsistent",
			HTTPStatus:  400,
			Retriable:   false,
		},
		ERR_VALIDATION_BUSINESS_RULE: {
			Code:        ERR_VALIDATION_BUSINESS_RULE,
			Type:        ErrorTypeInternal,
			Group:       GroupData,
			Category:    "validation",
			Description: "Business rule violation",
			HTTPStatus:  422,
			Retriable:   false,
		},
		ERR_VALIDATION_DUPLICATE: {
			Code:        ERR_VALIDATION_DUPLICATE,
			Type:        ErrorTypeInternal,
			Group:       GroupData,
			Category:    "validation",
			Description: "Duplicate resource",
			HTTPStatus:  409,
			Retriable:   false,
		},

		// ==============================
		// Client Errors
		// ==============================
		ERR_CLIENT_BAD_REQUEST: {
			Code:        ERR_CLIENT_BAD_REQUEST,
			Type:        ErrorTypeInternal,
			Group:       GroupClient,
			Category:    "client",
			Description: "Invalid client request",
			HTTPStatus:  400,
			Retriable:   false,
		},
		ERR_CLIENT_NOT_FOUND: {
			Code:        ERR_CLIENT_NOT_FOUND,
			Type:        ErrorTypeInternal,
			Group:       GroupClient,
			Category:    "client",
			Description: "Resource not found",
			HTTPStatus:  404,
			Retriable:   false,
		},
		ERR_CLIENT_RATE_LIMIT: {
			Code:        ERR_CLIENT_RATE_LIMIT,
			Type:        ErrorTypeInternal,
			Group:       GroupClient,
			Category:    "client",
			Description: "Rate limit exceeded",
			HTTPStatus:  429,
			Retriable:   true,
		},

		// ==============================
		// System Internal Errors
		// ==============================
		ERR_SYSTEM_UNEXPECTED: {
			Code:        ERR_SYSTEM_UNEXPECTED,
			Type:        ErrorTypeInternal,
			Group:       GroupSystem,
			Category:    "system",
			Description: "Unexpected error occurred",
			HTTPStatus:  500,
			Retriable:   false,
		},
		ERR_SYSTEM_CONFIG_INVALID: {
			Code:        ERR_SYSTEM_CONFIG_INVALID,
			Type:        ErrorTypeInternal,
			Group:       GroupSystem,
			Category:    "system",
			Description: "Invalid system configuration",
			HTTPStatus:  500,
			Retriable:   false,
		},
		ERR_SYSTEM_TIMEOUT_INTERNAL: {
			Code:        ERR_SYSTEM_TIMEOUT_INTERNAL,
			Type:        ErrorTypeInternal,
			Group:       GroupSystem,
			Category:    "system",
			Description: "Internal operation timeout",
			HTTPStatus:  500,
			Retriable:   true,
		},
		ERR_SYSTEM_CONCURRENCY: {
			Code:        ERR_SYSTEM_CONCURRENCY,
			Type:        ErrorTypeInternal,
			Group:       GroupSystem,
			Category:    "system",
			Description: "Concurrency conflict detected",
			HTTPStatus:  409,
			Retriable:   true,
		},
	}
)

// Registry is an isolated error metadata registry for SDK consumers that must
// avoid package-global mutation.
type Registry struct {
	mu       sync.RWMutex
	metadata map[ErrorCode]Metadata
}

// NewRegistry creates an isolated registry containing only the provided
// metadata. Entries are stored as given, without ValidateMetadata or conflict
// checks (a later entry for the same code replaces an earlier one); use
// NewRegistry() followed by RegisterMany to validate them.
func NewRegistry(metadata ...Metadata) *Registry {
	registry := &Registry{
		metadata: make(map[ErrorCode]Metadata, len(metadata)),
	}
	for _, meta := range metadata {
		registry.metadata[meta.Code] = meta
	}
	return registry
}

// NewDefaultRegistry creates an isolated copy of the package default registry.
func NewDefaultRegistry() *Registry {
	errorRegistryMu.RLock()
	defer errorRegistryMu.RUnlock()

	return &Registry{metadata: maps.Clone(errorRegistry)}
}

// GetMetadata retrieves metadata for a given error code
func GetMetadata(code ErrorCode) (Metadata, bool) {
	errorRegistryMu.RLock()
	defer errorRegistryMu.RUnlock()

	meta, exists := errorRegistry[code]
	return meta, exists
}

// GetMetadata retrieves metadata for a given error code from this registry.
func (r *Registry) GetMetadata(code ErrorCode) (Metadata, bool) {
	if r == nil {
		return Metadata{}, false
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	meta, exists := r.metadata[code]
	return meta, exists
}

// GetAllCodes returns all registered error codes, sorted.
func GetAllCodes() []ErrorCode {
	errorRegistryMu.RLock()
	defer errorRegistryMu.RUnlock()

	return sortedCodes(errorRegistry, nil)
}

// GetAllCodes returns all registered error codes in this registry, sorted.
func (r *Registry) GetAllCodes() []ErrorCode {
	if r == nil {
		return nil
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	return sortedCodes(r.metadata, nil)
}

// GetCodesByGroup returns all error codes in a specific group, sorted.
func GetCodesByGroup(group ErrorGroup) []ErrorCode {
	errorRegistryMu.RLock()
	defer errorRegistryMu.RUnlock()

	return sortedCodes(errorRegistry, func(meta Metadata) bool { return meta.Group == group })
}

// GetCodesByGroup returns all error codes in a group from this registry,
// sorted.
func (r *Registry) GetCodesByGroup(group ErrorGroup) []ErrorCode {
	if r == nil {
		return nil
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	return sortedCodes(r.metadata, func(meta Metadata) bool { return meta.Group == group })
}

// GetCodesByType returns all error codes of a specific type, sorted.
func GetCodesByType(errType ErrorType) []ErrorCode {
	errorRegistryMu.RLock()
	defer errorRegistryMu.RUnlock()

	return sortedCodes(errorRegistry, func(meta Metadata) bool { return meta.Type == errType })
}

// GetCodesByType returns all error codes of a specific type from this
// registry, sorted.
func (r *Registry) GetCodesByType(errType ErrorType) []ErrorCode {
	if r == nil {
		return nil
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	return sortedCodes(r.metadata, func(meta Metadata) bool { return meta.Type == errType })
}

// sortedCodes returns the codes of m whose metadata satisfies keep (every
// code when keep is nil), in ascending order. The caller holds m's lock.
func sortedCodes(m map[ErrorCode]Metadata, keep func(Metadata) bool) []ErrorCode {
	codes := make([]ErrorCode, 0, len(m))
	for code, meta := range m {
		if keep == nil || keep(meta) {
			codes = append(codes, code)
		}
	}
	slices.Sort(codes)
	return codes
}

// NewError creates an AppError using this registry's metadata.
func (r *Registry) NewError(code ErrorCode) *AppError {
	meta, exists := r.GetMetadata(code)
	if !exists {
		meta = unknownMetadata(code)
	}
	return newAppError(code, meta)
}

func unknownMetadata(code ErrorCode) Metadata {
	return Metadata{
		Code:        code,
		Type:        ErrorTypeInternal,
		Group:       GroupUnknown,
		Category:    "unknown",
		Description: "Unknown error",
		HTTPStatus:  500,
		Retriable:   false,
	}
}
