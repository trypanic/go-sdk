package urlkit

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/trypanic/go-sdk/errorkit"
)

func TestBuildURL(t *testing.T) {
	tests := []struct {
		name    string
		base    string
		path    string
		params  map[string]string
		wantURL string
		wantErr bool
	}{
		{
			name:    "basic case",
			base:    "https://api.com",
			path:    "/users",
			params:  map[string]string{"page": "1"},
			wantURL: "https://api.com/users?page=1",
		},
		{
			name:    "invalid base URL",
			base:    "://invalid",
			path:    "/users",
			params:  nil,
			wantErr: true,
		},
		{
			name:    "empty params",
			base:    "https://api.com",
			path:    "/users",
			params:  nil,
			wantURL: "https://api.com/users",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel() // Safe with pure functions!

			got, err := BuildURL(tt.base, tt.path, tt.params)

			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)
			assert.Equal(t, tt.wantURL, got.String())
		})
	}
}

func TestJoinPath(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		base  string
		parts []string
		want  string
	}{
		{
			name:  "trailing slash on base, leading slash on segment",
			base:  "https://api.example.com/",
			parts: []string{"/v1/", "/users/"},
			want:  "https://api.example.com/v1/users",
		},
		{
			name:  "no slashes anywhere",
			base:  "https://api.example.com",
			parts: []string{"v1", "users"},
			want:  "https://api.example.com/v1/users",
		},
		{
			name:  "single segment",
			base:  "https://api.example.com",
			parts: []string{"health"},
			want:  "https://api.example.com/health",
		},
		{
			name:  "empty segments are skipped",
			base:  "https://api.example.com",
			parts: []string{"", "v1", "", "users"},
			want:  "https://api.example.com/v1/users",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := JoinPath(tc.base, tc.parts...)
			assert.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestJoinPathRejectsInvalidBase(t *testing.T) {
	t.Parallel()

	_, err := JoinPath("://bad", "users")
	assert.Error(t, err)
}

func TestBuildURLTreatsPathAsPath(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		base     string
		path     string
		params   map[string]string
		wantURL  string
		wantHost string
	}{
		{
			name:     "userinfo-looking path cannot change the host",
			base:     "https://api.good.com",
			path:     "@evil.com/x",
			params:   map[string]string{"p": "1"},
			wantURL:  "https://api.good.com/@evil.com/x?p=1",
			wantHost: "api.good.com",
		},
		{
			name:     "host-suffix-looking path cannot change the host",
			base:     "https://api.good.com",
			path:     ".evil.com/x",
			wantURL:  "https://api.good.com/.evil.com/x",
			wantHost: "api.good.com",
		},
		{
			name:     "missing leading slash is added",
			base:     "https://api.example.com",
			path:     "users",
			params:   map[string]string{"p": "1"},
			wantURL:  "https://api.example.com/users?p=1",
			wantHost: "api.example.com",
		},
		{
			name:     "base query is preserved and path is appended to base path",
			base:     "https://api.example.com/v1?key=abc",
			path:     "/users",
			params:   map[string]string{"p": "1"},
			wantURL:  "https://api.example.com/v1/users?key=abc&p=1",
			wantHost: "api.example.com",
		},
		{
			name:     "base trailing slash is not doubled",
			base:     "https://api.example.com/v1/",
			path:     "/users/",
			wantURL:  "https://api.example.com/v1/users/",
			wantHost: "api.example.com",
		},
		{
			name:     "double slash path stays a path",
			base:     "https://api.example.com",
			path:     "//evil.com/x",
			wantURL:  "https://api.example.com//evil.com/x",
			wantHost: "api.example.com",
		},
		{
			name:     "percent escapes in the path are kept",
			base:     "https://api.example.com",
			path:     "/files/a%2Fb",
			wantURL:  "https://api.example.com/files/a%2Fb",
			wantHost: "api.example.com",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := BuildURL(tc.base, tc.path, tc.params)
			require.NoError(t, err)
			assert.Equal(t, tc.wantURL, got.String())
			assert.Equal(t, tc.wantHost, got.Host)
		})
	}
}

func TestBuildURLRejectsPathWithQueryFragmentOrDotSegments(t *testing.T) {
	t.Parallel()

	for _, path := range []string{
		"/users/1?admin=true",
		"/users#frag",
		"/v1/../admin",
		"..",
		"/./users",
		"/a/%2e%2e/b",
		"/bad%zzescape",
	} {
		t.Run(path, func(t *testing.T) {
			t.Parallel()

			_, err := BuildURL("https://api.example.com", path, map[string]string{"p": "1"})
			require.Error(t, err)
			appErr, ok := errors.AsType[*errorkit.AppError](err)
			require.True(t, ok)
			assert.Equal(t, errorkit.ERR_VALIDATION_INVALID_FORMAT, appErr.Code())
		})
	}
}
