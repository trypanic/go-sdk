package httprequest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"

	"github.com/trypanic/go-sdk/errorkit"
	"github.com/trypanic/go-sdk/httpclient"
	"github.com/trypanic/go-sdk/logger"
)

// redactBody returns the redactor output as a string. It centralizes the
// nil-safety logic so callers can always call it.
func (h *HTTPRequest) redactBody(b []byte) string {
	r := h.redactor
	if r == nil {
		r = DefaultBodyRedactor
	}
	return string(r(b))
}

// buildNetworkError creates a structured network error. Request body is only
// included when WithBodyCapture is enabled, and even then it is redacted.
func (h *HTTPRequest) buildNetworkError(err error, requestURL string, requestBody []byte) error {
	payload := map[string]any{
		"url":            requestURL,
		"original_error": err.Error(),
	}
	if h.captureBodies {
		payload["request_body"] = h.redactBody(requestBody)
	}

	// Check for context errors
	if errors.Is(err, context.DeadlineExceeded) {
		return errorkit.NewError(ERR_NETWORK_TIMEOUT).
			With(
				errorkit.WithReason("Request deadline exceeded"),
				errorkit.WithWrapped(err),
				errorkit.WithPayload(payload),
			)
	}

	if errors.Is(err, context.Canceled) {
		return errorkit.NewError(errorkit.ERR_SYSTEM_UNEXPECTED).
			With(
				errorkit.WithReason("Request was canceled"),
				errorkit.WithWrapped(err),
				errorkit.WithPayload(payload),
			)
	}

	// Check for net.Error timeout
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return errorkit.NewError(ERR_NETWORK_TIMEOUT).
			With(
				errorkit.WithReason("Network operation timed out"),
				errorkit.WithWrapped(err),
				errorkit.WithPayload(payload),
			)
	}

	// Generic network error
	return errorkit.NewError(ERR_NETWORK_ERROR).
		With(
			errorkit.WithReason("Network communication error"),
			errorkit.WithWrapped(err),
			errorkit.WithPayload(payload),
		)
}

// buildHTTPError maps HTTP status codes to structured errors.
func (h *HTTPRequest) buildHTTPError(res *http.Response, bodyBytes []byte, requestBody []byte) error {
	statusCode := res.StatusCode
	requestURL := res.Request.URL.String()
	providerErr := parseProviderError(bodyBytes)

	payload := map[string]any{
		"url":         requestURL,
		"status_code": statusCode,
	}
	if h.captureBodies {
		payload["response_body"] = h.redactBody(bodyBytes)
		payload["request_body"] = h.redactBody(requestBody)
	}
	if providerErr.Code != "" {
		payload["provider_error_code"] = providerErr.Code
	}
	if providerErr.Type != "" {
		payload["provider_error_type"] = providerErr.Type
	}
	if providerErr.Message != "" {
		payload["provider_error_message"] = providerErr.Message
	}

	var errCode errorkit.ErrorCode
	var reason string
	forceNonRetriable := false

	// Map status codes to error codes
	switch {
	// 5xx Server Errors
	case statusCode == http.StatusServiceUnavailable: // 503
		errCode = httpclient.ERR_EXTERNAL_SERVICE_UNAVAILABLE
		reason = fmt.Sprintf("External service unavailable (%d)", statusCode)
	case statusCode == http.StatusGatewayTimeout: // 504
		errCode = httpclient.ERR_EXTERNAL_SERVICE_TIMEOUT
		reason = fmt.Sprintf("External service timeout (%d)", statusCode)
	case statusCode >= 500 && statusCode <= 599:
		errCode = httpclient.ERR_EXTERNAL_SERVICE_ERROR
		reason = fmt.Sprintf("External service error (%d)", statusCode)

	// 4xx Client Errors
	case statusCode == http.StatusTooManyRequests: // 429
		errCode = errorkit.ERR_CLIENT_RATE_LIMIT
		reason = fmt.Sprintf("Rate limit exceeded (%d)", statusCode)
		if strings.EqualFold(providerErr.Code, "insufficient_quota") {
			reason = fmt.Sprintf("Insufficient quota at provider (%s) (%d)", providerErr.Code, statusCode)
			forceNonRetriable = true
		}
	case statusCode == http.StatusNotFound: // 404
		errCode = errorkit.ERR_CLIENT_NOT_FOUND
		reason = fmt.Sprintf("Resource not found (%d)", statusCode)
	case statusCode == http.StatusBadRequest: // 400
		errCode = errorkit.ERR_CLIENT_BAD_REQUEST
		reason = fmt.Sprintf("Bad request (%d)", statusCode)
	case statusCode == http.StatusUnauthorized: // 401
		errCode = ERR_AUTH_UNAUTHENTICATED
		reason = fmt.Sprintf("Authentication required (%d)", statusCode)
	case statusCode == http.StatusForbidden: // 403
		errCode = ERR_AUTH_UNAUTHORIZED
		reason = fmt.Sprintf("Access forbidden (%d)", statusCode)
	case statusCode >= 400 && statusCode <= 499:
		errCode = httpclient.ERR_EXTERNAL_INVALID_RESPONSE
		reason = fmt.Sprintf("Invalid request or response (%d)", statusCode)

	// Unexpected status codes
	default:
		errCode = errorkit.ERR_SYSTEM_UNEXPECTED
		reason = fmt.Sprintf("Unexpected status code (%d)", statusCode)
	}

	appErr := errorkit.NewError(errCode).
		With(
			errorkit.WithReason("%s", reason),
			errorkit.WithPayload(payload),
		)
	if forceNonRetriable {
		appErr.Metadata.Retriable = false
	}
	logger.Error(appErr)
	return appErr
}

type providerError struct {
	Code    string
	Type    string
	Message string
}

func parseProviderError(bodyBytes []byte) providerError {
	var raw struct {
		Error struct {
			Code    string `json:"code"`
			Type    string `json:"type"`
			Message string `json:"message"`
		} `json:"error"`
	}

	if err := json.Unmarshal(bodyBytes, &raw); err != nil {
		return providerError{}
	}

	return providerError{
		Code:    raw.Error.Code,
		Type:    raw.Error.Type,
		Message: raw.Error.Message,
	}
}
