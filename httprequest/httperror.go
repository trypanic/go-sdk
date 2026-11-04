package httprequest

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"

	"github.com/trypanic/go-sdk/v2/errorkit"
	"github.com/trypanic/go-sdk/v2/internal/redact"
)

// redactBody returns the redactor output as a string. It centralizes the
// nil-safety logic so callers can always call it.
func (h *Request) redactBody(b []byte) string {
	r := h.redactor
	if r == nil {
		r = DefaultBodyRedactor
	}
	return string(r(b))
}

// buildNetworkError creates a structured network error. Request body is only
// included when WithBodyCapture is enabled, and even then it is redacted.
// The URL, in the payload and inside a wrapped *url.Error, is sanitized by
// redact.URL so credentials never reach logs or error payloads.
func (h *Request) buildNetworkError(err error, requestURL string, requestBody []byte) error {
	// An *errorkit.AppError raised inside the client (for example by a
	// CheckRedirect policy) already carries the right classification.
	if inner, ok := errorkit.As(err); ok {
		return inner
	}

	err = redact.URLError(err)
	payload := map[string]any{
		"url":            redact.URL(requestURL),
		"original_error": err.Error(),
	}
	if h.captureBodies {
		payload["request_body"] = h.redactBody(requestBody)
	}

	if isTLSVerificationError(err) {
		return errorkit.NewError(errorkit.ERR_EXTERNAL_INVALID_RESPONSE).
			With(
				errorkit.WithReason("TLS certificate verification failed"),
				errorkit.WithWrapped(err),
				errorkit.WithPayload(payload),
			)
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
		return errorkit.NewError(ERR_REQUEST_CANCELED).
			With(
				errorkit.WithReason("Request was canceled"),
				errorkit.WithWrapped(err),
				errorkit.WithPayload(payload),
			)
	}

	// Check for net.Error timeout
	if netErr, ok := errors.AsType[net.Error](err); ok && netErr.Timeout() {
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

// isTLSVerificationError reports whether err is a certificate verification
// failure, which a retry cannot fix.
func isTLSVerificationError(err error) bool {
	return isA[*tls.CertificateVerificationError](err) ||
		isA[x509.UnknownAuthorityError](err) ||
		isA[x509.HostnameError](err) ||
		isA[x509.CertificateInvalidError](err) ||
		isA[x509.ConstraintViolationError](err) ||
		isA[x509.InsecureAlgorithmError](err)
}

// isA reports whether err's chain holds an error of type E.
func isA[E error](err error) bool {
	_, ok := errors.AsType[E](err)
	return ok
}

// validateURL checks a request URL once, before any attempt: it must parse
// and name an http or https scheme and a host. The error never echoes the
// raw URL, which may hold credentials.
func validateURL(rawURL string) error {
	u, err := url.Parse(rawURL)
	reason := ""
	switch {
	case err != nil:
		reason = "request URL cannot be parsed"
	case u.Scheme != "http" && u.Scheme != "https":
		reason = "request URL scheme must be http or https"
	case u.Host == "":
		reason = "request URL has no host"
	default:
		return nil
	}
	return errorkit.NewError(errorkit.ERR_SYSTEM_CONFIG_INVALID).
		With(
			errorkit.WithReason("%s", reason),
			errorkit.WithPayload(map[string]any{"url": redact.URL(rawURL)}),
		)
}

// buildHTTPError maps HTTP status codes to structured errors.
func (h *Request) buildHTTPError(res *http.Response, bodyBytes []byte, requestBody []byte) error {
	statusCode := res.StatusCode
	requestURL := redact.URL(res.Request.URL.String())
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
	// Providers often echo input or partial credentials in the message, so
	// it follows the same opt-in and redaction policy as the bodies.
	if h.captureBodies && providerErr.Message != "" {
		payload["provider_error_message"] = h.redactBody([]byte(providerErr.Message))
	}

	var errCode errorkit.ErrorCode
	var reason string
	forceNonRetriable := false

	// Map status codes to error codes
	switch {
	// 5xx Server Errors
	case statusCode == http.StatusServiceUnavailable: // 503
		errCode = errorkit.ERR_EXTERNAL_SERVICE_UNAVAILABLE
		reason = fmt.Sprintf("External service unavailable (%d)", statusCode)
	case statusCode == http.StatusGatewayTimeout: // 504
		errCode = errorkit.ERR_EXTERNAL_SERVICE_TIMEOUT
		reason = fmt.Sprintf("External service timeout (%d)", statusCode)
	case statusCode >= 500 && statusCode <= 599:
		errCode = errorkit.ERR_EXTERNAL_SERVICE_ERROR
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
		errCode = errorkit.ERR_EXTERNAL_INVALID_RESPONSE
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
	// Not logged here: the error is returned, and the caller decides
	// whether and how to log it (handle errors once).
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
