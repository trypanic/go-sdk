package envs

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/trypanic/go-sdk/errorkit"
)

type tableNested struct {
	Name string `env:"ENVS_TBL_NESTED_NAME"`
}

type tableConfig struct {
	Port    int           `env:"ENVS_TBL_PORT,required"`
	Host    string        `env:"ENVS_TBL_HOST" envDefault:"localhost"`
	Timeout time.Duration `env:"ENVS_TBL_TIMEOUT" envDefault:"5s"`
	Tags    []string      `env:"ENVS_TBL_TAGS"`
	Nested  tableNested
}

// Not parallel: t.Setenv forbids it.
func TestNewLoaderTable(t *testing.T) {
	cases := []struct {
		name    string
		env     map[string]string
		target  func() any
		want    *tableConfig
		wantErr bool
	}{
		{
			name:   "success with defaults",
			env:    map[string]string{"ENVS_TBL_PORT": "8080"},
			target: func() any { return &tableConfig{} },
			want:   &tableConfig{Port: 8080, Host: "localhost", Timeout: 5 * time.Second},
		},
		{
			name: "success with overrides, slice and nested struct",
			env: map[string]string{
				"ENVS_TBL_PORT":        "1",
				"ENVS_TBL_HOST":        "h",
				"ENVS_TBL_TIMEOUT":     "1m",
				"ENVS_TBL_TAGS":        "a,b",
				"ENVS_TBL_NESTED_NAME": "n",
			},
			target: func() any { return &tableConfig{} },
			want: &tableConfig{
				Port: 1, Host: "h", Timeout: time.Minute,
				Tags: []string{"a", "b"}, Nested: tableNested{Name: "n"},
			},
		},
		{
			name:    "type conversion failure",
			env:     map[string]string{"ENVS_TBL_PORT": "not-a-number"},
			target:  func() any { return &tableConfig{} },
			wantErr: true,
		},
		{
			name:    "bad duration",
			env:     map[string]string{"ENVS_TBL_PORT": "1", "ENVS_TBL_TIMEOUT": "soon"},
			target:  func() any { return &tableConfig{} },
			wantErr: true,
		},
		{
			name:    "nil target",
			target:  func() any { return nil },
			wantErr: true,
		},
		{
			name:    "non-pointer target",
			target:  func() any { return tableConfig{} },
			wantErr: true,
		},
		{
			name:    "pointer to non-struct",
			target:  func() any { n := 0; return &n },
			wantErr: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for _, k := range []string{"ENVS_TBL_PORT", "ENVS_TBL_HOST", "ENVS_TBL_TIMEOUT", "ENVS_TBL_TAGS", "ENVS_TBL_NESTED_NAME"} {
				t.Setenv(k, tc.env[k])
			}
			target := tc.target()
			err := LoadInto(target)
			if tc.wantErr {
				appErr, ok := errors.AsType[*errorkit.AppError](err)
				if !ok {
					t.Fatalf("error = %v (%T), want *errorkit.AppError", err, err)
				}
				if appErr.Code() != errorkit.ERR_SYSTEM_CONFIG_INVALID {
					t.Fatalf("code = %s, want %s", appErr.Code(), errorkit.ERR_SYSTEM_CONFIG_INVALID)
				}
				return
			}
			if err != nil {
				t.Fatalf("error = %v, want nil", err)
			}
			if got := target.(*tableConfig); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %+v, want %+v", *got, *tc.want)
			}
		})
	}
}
