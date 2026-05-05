package errorkit

// ErrorCode represents a unique error code identifier
type ErrorCode string

// ErrorType classifies whether an error originated internally or externally
type ErrorType string

const (
	ErrorTypeInternal ErrorType = "internal" // Error originated within the application
	ErrorTypeExternal ErrorType = "external" // Error originated from external service
)

// ErrorGroup represents the functional group/domain of an error
type ErrorGroup string

const (
	GroupUnknown      ErrorGroup = "unknown"
	GroupData         ErrorGroup = "data"
	GroupSystem       ErrorGroup = "system"
	GroupClient       ErrorGroup = "client"
	GroupAuth         ErrorGroup = "auth"
	GroupStorage      ErrorGroup = "storage"
	GroupDatabase     ErrorGroup = "database"
	GroupMessageQueue ErrorGroup = "message_queue"
)
