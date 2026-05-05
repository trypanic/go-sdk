package mongodb

import "testing"

func TestApplyDefaultsRetryWritesNilEnablesIt(t *testing.T) {
	t.Parallel()

	cfg := Config{}
	applyDefaults(&cfg)
	if cfg.RetryWrites == nil || *cfg.RetryWrites != true {
		t.Fatalf("nil RetryWrites should default to true, got %v", cfg.RetryWrites)
	}
}

func TestApplyDefaultsRetryWritesFalsePreserved(t *testing.T) {
	t.Parallel()

	f := false
	cfg := Config{RetryWrites: &f}
	applyDefaults(&cfg)
	if cfg.RetryWrites == nil || *cfg.RetryWrites != false {
		t.Fatalf("explicit RetryWrites=false must be preserved, got %v", cfg.RetryWrites)
	}
}

func TestApplyDefaultsRetryWritesTruePreserved(t *testing.T) {
	t.Parallel()

	tr := true
	cfg := Config{RetryWrites: &tr}
	applyDefaults(&cfg)
	if cfg.RetryWrites == nil || *cfg.RetryWrites != true {
		t.Fatalf("explicit RetryWrites=true must be preserved, got %v", cfg.RetryWrites)
	}
}

func TestNormalizeURI(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		uri  string
		want string
	}{
		{
			name: "preserves mongodb scheme",
			uri:  "mongodb://localhost:27017",
			want: "mongodb://localhost:27017",
		},
		{
			name: "preserves mongodb srv scheme",
			uri:  "mongodb+srv://cluster0.example.mongodb.net",
			want: "mongodb+srv://cluster0.example.mongodb.net",
		},
		{
			name: "prepends mongodb scheme when missing",
			uri:  "localhost:27017",
			want: "mongodb://localhost:27017",
		},
		{
			name: "trims accidental leading equals",
			uri:  "=\"mongodb://localhost:27017\"",
			want: "mongodb://localhost:27017",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := normalizeURI(tt.uri); got != tt.want {
				t.Errorf("normalizeURI(%q) = %q, want %q", tt.uri, got, tt.want)
			}
		})
	}
}
