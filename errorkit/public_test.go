package errorkit_test

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/trypanic/go-sdk/v2/errorkit"
)

func TestPublicJSONOmitsInternalDetails(t *testing.T) {
	const (
		secretPayload = "payload-secret-value"
		wrappedText   = "pq: password authentication failed for user admin at 10.0.0.7"
		serverReason  = "query users failed on shard 3"
	)
	appErr := errorkit.NewError(errorkit.ERR_INTERNAL).With(
		errorkit.WithReason(serverReason),
		errorkit.WithPayload(map[string]string{"token": secretPayload}),
		errorkit.WithWrapped(errors.New(wrappedText)),
		errorkit.WithTraceID("trace-123"),
	)

	out := appErr.PublicJSON()

	for _, forbidden := range []string{
		"stack_trace", "payload", "wrapped", secretPayload, wrappedText,
		serverReason, ".go", "metadata",
	} {
		if strings.Contains(out, forbidden) {
			t.Errorf("PublicJSON() contains %q:\n%s", forbidden, out)
		}
	}

	var got map[string]any
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("PublicJSON() is not valid JSON: %v\n%s", err, out)
	}
	want := map[string]any{
		"code":     string(errorkit.ERR_INTERNAL),
		"message":  "Internal server error",
		"id":       appErr.ID,
		"trace_id": "trace-123",
	}
	if len(got) != len(want) {
		t.Errorf("PublicJSON() keys = %v, want exactly %v", got, want)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("PublicJSON()[%q] = %v, want %v", k, got[k], v)
		}
	}
}

func TestPublicMessageByStatusClass(t *testing.T) {
	cases := []struct {
		name   string
		code   errorkit.ErrorCode
		reason string
		want   string
	}{
		{
			name:   "client error with reason exposes the reason",
			code:   errorkit.ERR_VALIDATION_MISSING_FIELD,
			reason: "field 'email' is required",
			want:   "field 'email' is required",
		},
		{
			name: "client error without reason uses the description",
			code: errorkit.ERR_VALIDATION_MISSING_FIELD,
			want: "Required field is missing",
		},
		{
			name:   "server error hides the reason",
			code:   errorkit.ERR_INTERNAL,
			reason: "dial tcp 10.0.0.7:5432: connection refused",
			want:   "Internal server error",
		},
		{
			name:   "unknown code is a server error",
			code:   "ERR_NOT_REGISTERED_PUBLIC",
			reason: "internal detail",
			want:   "Unknown error",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			appErr := errorkit.NewError(tc.code)
			if tc.reason != "" {
				appErr = appErr.With(errorkit.WithReason("%s", tc.reason))
			}
			pub := appErr.Public()
			if pub.Message != tc.want {
				t.Errorf("Public().Message = %q, want %q", pub.Message, tc.want)
			}
			if pub.Code != tc.code || pub.ID != appErr.ID {
				t.Errorf("Public() = %+v, want code %s and id %s", pub, tc.code, appErr.ID)
			}
		})
	}
}

func TestPublicFallsBackWhenDescriptionIsEmpty(t *testing.T) {
	appErr := &errorkit.AppError{
		ErrCode:  "ERR_NO_DESCRIPTION",
		Reason:   "secret internals",
		Metadata: errorkit.Metadata{HTTPStatus: 503},
	}
	if got, want := appErr.Public().Message, "Service Unavailable"; got != want {
		t.Errorf("Public().Message = %q, want %q", got, want)
	}

	appErr.Metadata.HTTPStatus = 0
	if got, want := appErr.Public().Message, "Internal Server Error"; got != want {
		t.Errorf("Public().Message with status 0 = %q, want %q", got, want)
	}
}

func TestPublicJSONNil(t *testing.T) {
	var appErr *errorkit.AppError
	if got := appErr.PublicJSON(); got != "{}" {
		t.Errorf("nil PublicJSON() = %q, want {}", got)
	}
	if got := appErr.Public(); got != (errorkit.PublicError{}) {
		t.Errorf("nil Public() = %+v, want zero value", got)
	}
}
