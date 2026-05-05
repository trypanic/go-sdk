package errorkit

// StrictMetadataReason is appended to AppError.Reason when StrictMetadata is
// enabled and the requested code has no registered metadata.
const StrictMetadataReason = "metadata not registered"

// Config controls SDK-safe error creation without mutating package globals.
type Config struct {
	Registry       *Registry
	MaxStackDepth  int
	StrictMetadata bool
}

// Factory creates AppError values using explicit configuration.
type Factory struct {
	registry       *Registry
	maxStackDepth  int
	strictMetadata bool
}

// NewFactory creates an error factory. When no registry is provided, it uses an
// isolated copy of the default package registry.
func NewFactory(config Config) *Factory {
	registry := config.Registry
	if registry == nil {
		registry = NewDefaultRegistry()
	}

	return &Factory{
		registry:       registry,
		maxStackDepth:  clampStackDepth(config.MaxStackDepth),
		strictMetadata: config.StrictMetadata,
	}
}

// NewError creates an AppError using the factory registry and stack-depth config.
func (f *Factory) NewError(code ErrorCode) *AppError {
	if f == nil {
		return NewError(code)
	}

	meta, exists := f.registry.GetMetadata(code)
	if !exists {
		meta = unknownMetadata(code)
	}
	err := newAppErrorWithStackDepth(code, meta, f.maxStackDepth)
	if !exists && f.strictMetadata {
		err.Reason = StrictMetadataReason
	}
	return err
}
