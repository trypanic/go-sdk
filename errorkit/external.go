package errorkit

var externalMetadata = []Metadata{
	{
		Code:        ERR_EXTERNAL_SERVICE_UNAVAILABLE,
		Type:        ErrorTypeExternal,
		Group:       GroupSystem,
		Category:    "external",
		Description: "External service unavailable",
		HTTPStatus:  503,
		Retriable:   true,
	},
	{
		Code:        ERR_EXTERNAL_SERVICE_TIMEOUT,
		Type:        ErrorTypeExternal,
		Group:       GroupSystem,
		Category:    "external",
		Description: "External service timeout",
		HTTPStatus:  504,
		Retriable:   true,
	},
	{
		Code:        ERR_EXTERNAL_SERVICE_ERROR,
		Type:        ErrorTypeExternal,
		Group:       GroupSystem,
		Category:    "external",
		Description: "External service error",
		HTTPStatus:  503,
		Retriable:   true,
	},
	{
		Code:        ERR_EXTERNAL_INVALID_RESPONSE,
		Type:        ErrorTypeExternal,
		Group:       GroupSystem,
		Category:    "external",
		Description: "Invalid response from external service",
		HTTPStatus:  502,
		Retriable:   false,
	},
}

func init() { MustRegister(externalMetadata...) }
