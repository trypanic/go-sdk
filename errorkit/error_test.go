package errorkit

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

const (
	ERR_TEST_CODE ErrorCode = "ERR_TEST_CODE"
)

func TestMain(m *testing.M) {
	MustRegister(Metadata{
		Code:        ERR_TEST_CODE,
		Type:        ErrorTypeInternal,
		Group:       GroupUnknown,
		Category:    "test",
		Description: "This is a test error",
		HTTPStatus:  500,
		Retriable:   false,
	})
	m.Run()
}

func TestNewError(t *testing.T) {
	t.Run("known_error_code", func(t *testing.T) {
		err := NewError(ERR_TEST_CODE)
		if err == nil {
			t.Fatal("NewError() returned nil, want *AppError")
		}
		if err.ErrCode != ERR_TEST_CODE {
			t.Errorf("NewError(ERR_TEST_CODE).ErrCode = %v, want %v", err.ErrCode, ERR_TEST_CODE)
		}
		if err.Metadata.Description != "This is a test error" {
			t.Errorf("NewError(ERR_TEST_CODE).Metadata.Description = %v, want %v", err.Metadata.Description, "This is a test error")
		}
		if len(err.Trace) == 0 {
			t.Error("NewError() should capture a stack trace, but trace is empty")
		}
	})

	t.Run("unknown_error_code", func(t *testing.T) {
		const unknownCode ErrorCode = "ERR_TOTALLY_UNKNOWN"
		err := NewError(unknownCode)
		if err == nil {
			t.Fatal("NewError() returned nil, want *AppError")
		}
		if err.ErrCode != unknownCode {
			t.Errorf("NewError(%q).ErrCode = %v, want %v", unknownCode, err.ErrCode, unknownCode)
		}
		if err.Metadata.Description != "Unknown error" {
			t.Errorf("NewError(%q).Metadata.Description = %v, want %v", unknownCode, err.Metadata.Description, "Unknown error")
		}
	})
}

func TestError_Error(t *testing.T) {
	tests := []struct {
		name string
		err  *AppError
		want string
	}{
		{
			name: "error_with_description",
			err:  NewError(ERR_TEST_CODE),
			want: "[ERR_TEST_CODE] This is a test error",
		},
		{
			name: "error_with_reason",
			err:  NewError(ERR_TEST_CODE).With(WithReason("something went wrong")),
			want: "[ERR_TEST_CODE] something went wrong",
		},
		{
			name: "nil_error",
			err:  nil,
			want: "nil error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.err.Error(); got != tt.want {
				t.Errorf("(*AppError).Error() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestError_Unwrap(t *testing.T) {
	baseErr := errors.New("base error")

	tests := []struct {
		name    string
		err     *AppError
		want    error
		isEqual bool
	}{
		{
			name:    "wrapped_error",
			err:     NewError(ERR_TEST_CODE).With(WithWrapped(baseErr)),
			want:    baseErr,
			isEqual: true,
		},
		{
			name:    "no_wrapped_error",
			err:     NewError(ERR_TEST_CODE),
			want:    nil,
			isEqual: true,
		},
		{
			name:    "nil_error",
			err:     nil,
			want:    nil,
			isEqual: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.err.Unwrap()
			if tt.isEqual && got != tt.want {
				t.Errorf("(*AppError).Unwrap() = %v, want %v", got, tt.want)
			}
			if !tt.isEqual && errors.Is(got, tt.want) {
				t.Errorf("(*AppError).Unwrap() should not be %v", tt.want)
			}
		})
	}
}

func TestError_Code(t *testing.T) {
	tests := []struct {
		name string
		err  *AppError
		want ErrorCode
	}{
		{
			name: "valid_error",
			err:  NewError(ERR_TEST_CODE),
			want: ERR_TEST_CODE,
		},
		{
			name: "nil_error",
			err:  nil,
			want: ERR_UNKNOWN,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.err.Code(); got != tt.want {
				t.Errorf("(*AppError).Code() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestError_WithOptions(t *testing.T) {
	baseErr := errors.New("base error")
	payload := map[string]string{"key": "value"}

	err := NewError(ERR_TEST_CODE).With(
		WithPayload(payload),
		WithReason("custom reason"),
		WithWrapped(baseErr),
		WithTraceID("trace-123"),
	)

	if err.Payload == nil {
		t.Errorf("With(WithPayload) payload is nil, want %v", payload)
	}
	if err.Reason != "custom reason" {
		t.Errorf("With(WithReason) reason = %q, want %q", err.Reason, "custom reason")
	}
	if err.Wrapped != baseErr {
		t.Errorf("With(WithWrapped) wrapped = %v, want %v", err.Wrapped, baseErr)
	}
	if err.TraceID != "trace-123" {
		t.Errorf("With(WithTraceID) traceID = %q, want %q", err.TraceID, "trace-123")
	}

	t.Run("nil_receiver", func(t *testing.T) {
		var err *AppError
		// This should not panic
		res := err.With(WithReason("should not apply"))
		if res != nil {
			t.Error("With() on nil receiver should return nil")
		}
	})
}

func TestError_Format(t *testing.T) {
	err := NewError(ERR_TEST_CODE).With(
		WithReason("something failed"),
		WithPayload(map[string]int{"count": 42}),
	)

	t.Run("Pretty", func(t *testing.T) {
		pretty := err.Pretty()
		if !strings.Contains(pretty, "something failed") {
			t.Errorf("Pretty() output should contain reason, got:\n%s", pretty)
		}
		if !strings.Contains(pretty, "testing:tRunner") {
			t.Errorf("Pretty() output should contain stack trace, got:\n%s", pretty)
		}
	})

	t.Run("PrettyJSON", func(t *testing.T) {
		jsonStr := err.PrettyJSON()
		var data map[string]any
		if err := json.Unmarshal([]byte(jsonStr), &data); err != nil {
			t.Fatalf("PrettyJSON() returned invalid JSON: %v", err)
		}

		if reason, ok := data["reason"].(string); !ok || reason != "something failed" {
			t.Errorf(`PrettyJSON() reason = %q, want "something failed"`, reason)
		}

		payload, ok := data["payload"].(map[string]any)
		if !ok {
			t.Fatal("PrettyJSON() payload is not a map")
		}
		if count, ok := payload["count"].(float64); !ok || count != 42 {
			t.Errorf(`PrettyJSON() payload.count = %v, want 42`, count)
		}
	})

	t.Run("WriteTo", func(t *testing.T) {
		var buf bytes.Buffer
		n, writeErr := err.WriteTo(&buf)
		if writeErr != nil {
			t.Fatalf("WriteTo() returned an unexpected error: %v", writeErr)
		}

		output := buf.String()
		if n != int64(len(output)) {
			t.Errorf("WriteTo() returned size %d, but output length is %d", n, len(output))
		}
		if !strings.Contains(output, "something failed") {
			t.Errorf("WriteTo() output should contain reason, got:\n%s", output)
		}
	})

	t.Run("nil_error", func(t *testing.T) {
		var nilErr *AppError
		if got := nilErr.Pretty(); got != "" {
			t.Errorf("Pretty() on nil error = %q, want %q", got, "")
		}
		if got := nilErr.PrettyJSON(); got != "{}" {
			t.Errorf("PrettyJSON() on nil error = %q, want %q", got, "{}")
		}
		var buf bytes.Buffer
		n, writeErr := nilErr.WriteTo(&buf)
		if writeErr != nil {
			t.Errorf("WriteTo() on nil error returned an error: %v", writeErr)
		}
		if n != 0 {
			t.Errorf("WriteTo() on nil error returned size %d, want 0", n)
		}
	})
}
