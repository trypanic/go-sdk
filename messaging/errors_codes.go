package messaging

import "github.com/trypanic/go-sdk/errorkit"

// ==============================
// Message Queue Error Codes (RabbitMQ)
// ==============================

const (
	// ERR_MQ_UNAVAILABLE indicates message queue service is not available
	ERR_MQ_UNAVAILABLE errorkit.ErrorCode = "ERR_MQ_UNAVAILABLE"

	// ERR_MQ_TIMEOUT indicates message queue operation timed out
	ERR_MQ_TIMEOUT errorkit.ErrorCode = "ERR_MQ_TIMEOUT"

	// ERR_MQ_AUTH_FAILED indicates message queue authentication failed
	ERR_MQ_AUTH_FAILED errorkit.ErrorCode = "ERR_MQ_AUTH_FAILED"

	// ERR_MQ_CONNECTION_FAILED indicates connection to message queue failed
	ERR_MQ_CONNECTION_FAILED errorkit.ErrorCode = "ERR_MQ_CONNECTION_FAILED"

	// ERR_MQ_CHANNEL_ERROR indicates channel operation failed
	ERR_MQ_CHANNEL_ERROR errorkit.ErrorCode = "ERR_MQ_CHANNEL_ERROR"

	// ERR_MQ_PUBLISH_FAILED indicates message publishing failed
	ERR_MQ_PUBLISH_FAILED errorkit.ErrorCode = "ERR_MQ_PUBLISH_FAILED"

	// ERR_MQ_CONSUME_FAILED indicates message consumption failed
	ERR_MQ_CONSUME_FAILED errorkit.ErrorCode = "ERR_MQ_CONSUME_FAILED"

	// ERR_MQ_QUEUE_ERROR indicates queue operation error
	ERR_MQ_QUEUE_ERROR errorkit.ErrorCode = "ERR_MQ_QUEUE_ERROR"

	// ERR_MQ_EXCHANGE_ERROR indicates exchange operation error
	ERR_MQ_EXCHANGE_ERROR errorkit.ErrorCode = "ERR_MQ_EXCHANGE_ERROR"

	// ERR_MQ_ERROR indicates a general message queue error
	ERR_MQ_ERROR errorkit.ErrorCode = "ERR_MQ_ERROR"
)

// ErrorMetadata is the canonical metadata for all messaging-owned error codes.
var ErrorMetadata = []errorkit.Metadata{
	{
		Code:        ERR_MQ_UNAVAILABLE,
		Type:        errorkit.ErrorTypeExternal,
		Group:       errorkit.GroupMessageQueue,
		Category:    "message_queue",
		Description: "Message queue service unavailable",
		HTTPStatus:  503,
		Retriable:   true,
	},
	{
		Code:        ERR_MQ_TIMEOUT,
		Type:        errorkit.ErrorTypeExternal,
		Group:       errorkit.GroupMessageQueue,
		Category:    "message_queue",
		Description: "Message queue operation timeout",
		HTTPStatus:  504,
		Retriable:   true,
	},
	{
		Code:        ERR_MQ_AUTH_FAILED,
		Type:        errorkit.ErrorTypeExternal,
		Group:       errorkit.GroupMessageQueue,
		Category:    "message_queue",
		Description: "Message queue authentication failed",
		HTTPStatus:  502,
		Retriable:   false,
	},
	{
		Code:        ERR_MQ_CONNECTION_FAILED,
		Type:        errorkit.ErrorTypeExternal,
		Group:       errorkit.GroupMessageQueue,
		Category:    "message_queue",
		Description: "Message queue connection failed",
		HTTPStatus:  503,
		Retriable:   true,
	},
	{
		Code:        ERR_MQ_CHANNEL_ERROR,
		Type:        errorkit.ErrorTypeExternal,
		Group:       errorkit.GroupMessageQueue,
		Category:    "message_queue",
		Description: "Message queue channel error",
		HTTPStatus:  503,
		Retriable:   true,
	},
	{
		Code:        ERR_MQ_PUBLISH_FAILED,
		Type:        errorkit.ErrorTypeExternal,
		Group:       errorkit.GroupMessageQueue,
		Category:    "message_queue",
		Description: "Message publishing failed",
		HTTPStatus:  503,
		Retriable:   true,
	},
	{
		Code:        ERR_MQ_CONSUME_FAILED,
		Type:        errorkit.ErrorTypeExternal,
		Group:       errorkit.GroupMessageQueue,
		Category:    "message_queue",
		Description: "Message consumption failed",
		HTTPStatus:  503,
		Retriable:   true,
	},
	{
		Code:        ERR_MQ_QUEUE_ERROR,
		Type:        errorkit.ErrorTypeExternal,
		Group:       errorkit.GroupMessageQueue,
		Category:    "message_queue",
		Description: "Message queue error",
		HTTPStatus:  503,
		Retriable:   true,
	},
	{
		Code:        ERR_MQ_EXCHANGE_ERROR,
		Type:        errorkit.ErrorTypeExternal,
		Group:       errorkit.GroupMessageQueue,
		Category:    "message_queue",
		Description: "Message queue exchange error",
		HTTPStatus:  503,
		Retriable:   true,
	},
	{
		Code:        ERR_MQ_ERROR,
		Type:        errorkit.ErrorTypeExternal,
		Group:       errorkit.GroupMessageQueue,
		Category:    "message_queue",
		Description: "Message queue error",
		HTTPStatus:  503,
		Retriable:   true,
	},
}

// RegisterErrors registers messaging-owned error metadata. When registry is
// nil it registers into the package-global registry. Calling more than once
// with identical metadata is a no-op.
func RegisterErrors(registry *errorkit.Registry) error {
	if registry == nil {
		return errorkit.RegisterMany(ErrorMetadata...)
	}
	return registry.RegisterMany(ErrorMetadata...)
}

func init() {
	errorkit.MustRegister(ErrorMetadata...)
}
