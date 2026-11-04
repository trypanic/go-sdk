package mongodb

import (
	"testing"
	"time"
)

type effectiveOptions struct {
	connectTimeout, serverSelection, maxIdle time.Duration
	maxPool, minPool                         uint64
	retryWrites                              bool
}

func effective(t *testing.T, cfg Config) effectiveOptions {
	t.Helper()

	opts := clientOptions(cfg)
	if err := opts.Validate(); err != nil {
		t.Fatalf("options for %+v do not validate: %v", cfg, err)
	}
	if opts.ConnectTimeout == nil || opts.ServerSelectionTimeout == nil || opts.MaxConnIdleTime == nil ||
		opts.MaxPoolSize == nil || opts.MinPoolSize == nil || opts.RetryWrites == nil {
		t.Fatalf("an option is unset: %+v", opts)
	}
	return effectiveOptions{
		connectTimeout:  *opts.ConnectTimeout,
		serverSelection: *opts.ServerSelectionTimeout,
		maxIdle:         *opts.MaxConnIdleTime,
		maxPool:         *opts.MaxPoolSize,
		minPool:         *opts.MinPoolSize,
		retryWrites:     *opts.RetryWrites,
	}
}

// TestClientOptionsPrecedence covers C4-011: SDK defaults overwrote every
// option set in the URI, and the default MinPoolSize of 10 made a
// MaxPoolSize below 10 fail validation.
func TestClientOptionsPrecedence(t *testing.T) {
	t.Parallel()

	const tunedURI = "mongodb://h1:27017/?maxPoolSize=50&minPoolSize=0&retryWrites=false" +
		"&connectTimeoutMS=1000&serverSelectionTimeoutMS=30000&maxIdleTimeMS=2000"
	f := false

	tests := []struct {
		name string
		cfg  Config
		want effectiveOptions
	}{
		{
			name: "SDK defaults when neither Config nor URI sets an option",
			cfg:  Config{URI: "mongodb://h1:27017"},
			want: effectiveOptions{10 * time.Second, 5 * time.Second, 30 * time.Second, 100, 10, true},
		},
		{
			name: "URI options survive",
			cfg:  Config{URI: tunedURI},
			want: effectiveOptions{time.Second, 30 * time.Second, 2 * time.Second, 50, 0, false},
		},
		{
			name: "Config fields override the URI",
			cfg: Config{
				URI: tunedURI, ConnectTimeout: 3 * time.Second, ServerSelectionTimeout: 4 * time.Second,
				MaxConnIdleTime: 5 * time.Second, MaxPoolSize: 20, MinPoolSize: 2, RetryWrites: new(true),
			},
			want: effectiveOptions{3 * time.Second, 4 * time.Second, 5 * time.Second, 20, 2, true},
		},
		{
			name: "default MinPoolSize clamped to a small MaxPoolSize",
			cfg:  Config{URI: "mongodb://h1:27017", MaxPoolSize: 5},
			want: effectiveOptions{10 * time.Second, 5 * time.Second, 30 * time.Second, 5, 5, true},
		},
		{
			name: "default MinPoolSize clamped to a small URI maxPoolSize",
			cfg:  Config{URI: "mongodb://h1:27017/?maxPoolSize=3"},
			want: effectiveOptions{10 * time.Second, 5 * time.Second, 30 * time.Second, 3, 3, true},
		},
		{
			name: "explicit RetryWrites false",
			cfg:  Config{URI: "mongodb://h1:27017", RetryWrites: &f},
			want: effectiveOptions{10 * time.Second, 5 * time.Second, 30 * time.Second, 100, 10, false},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := effective(t, tt.cfg); got != tt.want {
				t.Fatalf("effective options\n got  %+v\n want %+v", got, tt.want)
			}
		})
	}
}

// TestStartupPingTimeout covers C4-019: the start-up ping was cut at 5 s
// even when ServerSelectionTimeout (from Config or the URI) was larger.
func TestStartupPingTimeout(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		cfg  Config
		want time.Duration
	}{
		{"default", Config{URI: "mongodb://h1"}, 5 * time.Second},
		{"smaller selection timeout keeps the floor", Config{URI: "mongodb://h1", ServerSelectionTimeout: 2 * time.Second}, 5 * time.Second},
		{"larger Config selection timeout", Config{URI: "mongodb://h1", ServerSelectionTimeout: 30 * time.Second}, 30 * time.Second},
		{"larger URI selection timeout", Config{URI: "mongodb://h1/?serverSelectionTimeoutMS=20000"}, 20 * time.Second},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := startupPingTimeout(clientOptions(tt.cfg)); got != tt.want {
				t.Fatalf("startupPingTimeout = %v, want %v", got, tt.want)
			}
		})
	}
}
