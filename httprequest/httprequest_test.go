package httprequest

import (
	"errors"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/trypanic/go-sdk/errorkit"
)

func TestIsRetryable(t *testing.T) {
	h := &HTTPRequest{}

	// Case 1: Error is AppError and Retriable is true
	appErr := errorkit.NewError(ERR_NETWORK_TIMEOUT)
	appErr.Metadata.Retriable = true

	if !h.isRetryable(appErr) {
		t.Errorf("Expected isRetryable to return true for retriable AppError")
	}

	// Case 2: Error is AppError and Retriable is false
	appErr2 := errorkit.NewError(errorkit.ERR_SYSTEM_UNEXPECTED)
	appErr2.Metadata.Retriable = false

	if h.isRetryable(appErr2) {
		t.Errorf("Expected isRetryable to return false for non-retriable AppError")
	}

	// Case 3: Error is not AppError
	if h.isRetryable(errors.New("generic error")) {
		t.Errorf("Expected isRetryable to return false for generic error")
	}
}

func TestBuildHTTPError_InsufficientQuotaIsNotRetriable(t *testing.T) {
	t.Parallel()

	h := &HTTPRequest{}
	requestURL, err := url.Parse("https://api.openai.com/v1/chat/completions")
	if err != nil {
		t.Fatalf("failed to parse URL: %v", err)
	}

	res := &http.Response{
		StatusCode: http.StatusTooManyRequests,
		Request:    &http.Request{URL: requestURL},
	}

	body := []byte(`{
		"error": {
			"message": "You exceeded your current quota",
			"type": "insufficient_quota",
			"param": null,
			"code": "insufficient_quota"
		}
	}`)

	gotErr := h.buildHTTPError(res, body, []byte(`{"model":"gpt-5"}`))

	var appErr *errorkit.AppError
	if !errors.As(gotErr, &appErr) {
		t.Fatalf("expected AppError, got %T", gotErr)
	}
	if appErr.Code() != errorkit.ERR_CLIENT_RATE_LIMIT {
		t.Fatalf("expected code %s, got %s", errorkit.ERR_CLIENT_RATE_LIMIT, appErr.Code())
	}
	if appErr.Metadata.Retriable {
		t.Fatalf("expected non-retriable for insufficient_quota, got retriable")
	}
	if !strings.Contains(appErr.Reason, "insufficient_quota") {
		t.Fatalf("expected reason to include provider code, got %q", appErr.Reason)
	}
}

func TestBuildHTTPError_TooManyRequestsStaysRetriableByDefault(t *testing.T) {
	t.Parallel()

	h := &HTTPRequest{}
	requestURL, err := url.Parse("https://api.example.com/resource")
	if err != nil {
		t.Fatalf("failed to parse URL: %v", err)
	}

	res := &http.Response{
		StatusCode: http.StatusTooManyRequests,
		Request:    &http.Request{URL: requestURL},
	}

	gotErr := h.buildHTTPError(res, []byte(`{"error":{"code":"rate_limit_exceeded"}}`), nil)

	var appErr *errorkit.AppError
	if !errors.As(gotErr, &appErr) {
		t.Fatalf("expected AppError, got %T", gotErr)
	}
	if appErr.Code() != errorkit.ERR_CLIENT_RATE_LIMIT {
		t.Fatalf("expected code %s, got %s", errorkit.ERR_CLIENT_RATE_LIMIT, appErr.Code())
	}
	if !appErr.Metadata.Retriable {
		t.Fatalf("expected retriable for regular 429, got non-retriable")
	}
}
