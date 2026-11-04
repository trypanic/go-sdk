package redact

import (
	"context"
	"errors"
	"net"
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want string
	}{
		{"plain", "https://api.example.com/v1/items", "https://api.example.com/v1/items"},
		{"userinfo", "https://user:pass@api.example.com/v1", "https://api.example.com/v1"},
		{"username only", "https://sk-key@api.example.com/v1", "https://api.example.com/v1"},
		{"query values", "https://h/p?key=SECRET&token=x&flag", "https://h/p?key=REDACTED&token=REDACTED&flag=REDACTED"},
		{"fragment", "https://h/p#access_token=x", "https://h/p"},
		{"port and path kept", "http://127.0.0.1:8080/a/b", "http://127.0.0.1:8080/a/b"},
		{"empty", "", ""},
		{"invalid", "http://user:pass@h/\x7f?key=SECRET", "[invalid URL]"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.want, URL(tt.in))
		})
	}
}

func TestURLErrorKeepsCause(t *testing.T) {
	t.Parallel()

	opErr := &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("refused")}
	in := &url.Error{Op: "Get", URL: "http://u:p@h/x?key=SECRET", Err: opErr}

	got := URLError(in)

	require.NotContains(t, got.Error(), "SECRET")
	require.NotContains(t, got.Error(), "u:p")
	var gotOp *net.OpError
	require.ErrorAs(t, got, &gotOp)
	require.Equal(t, "http://u:p@h/x?key=SECRET", in.URL, "input must not be mutated")

	deadline := &url.Error{Op: "Get", URL: "http://h/?key=SECRET", Err: context.DeadlineExceeded}
	require.ErrorIs(t, URLError(deadline), context.DeadlineExceeded)

	plain := errors.New("plain")
	require.Same(t, plain, URLError(plain))
}
