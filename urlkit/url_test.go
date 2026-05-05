package urlkit

import (
	"testing"

	"github.com/stretchr/testify/assert"
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
