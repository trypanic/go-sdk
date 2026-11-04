package llmclient

import (
	"testing"

	"github.com/trypanic/go-sdk/v2/errorkit"
)

func TestExecuteValidatesKeyAndEndpoint(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		cfg     Config
		wantErr bool
	}{
		{name: "https endpoint with key", cfg: Config{APIKey: "k", Endpoint: "https://llm.example/v1/chat"}},
		{name: "empty key", cfg: Config{Endpoint: "https://llm.example/v1/chat"}, wantErr: true},
		{name: "empty endpoint", cfg: Config{APIKey: "k"}, wantErr: true},
		{name: "relative endpoint", cfg: Config{APIKey: "k", Endpoint: "v1/chat"}, wantErr: true},
		{name: "unparseable endpoint", cfg: Config{APIKey: "k", Endpoint: "://x"}, wantErr: true},
		{name: "plain http endpoint", cfg: Config{APIKey: "k", Endpoint: "http://llm.example/v1/chat"}, wantErr: true},
		{
			name: "plain http endpoint with opt-in",
			cfg:  Config{APIKey: "k", Endpoint: "http://localhost:11434/v1/chat", AllowInsecureEndpoint: true},
		},
		{
			name: "opt-in does not allow other schemes",
			cfg:  Config{APIKey: "k", Endpoint: "ftp://llm.example/v1", AllowInsecureEndpoint: true}, wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			stub := &captureRequester{}
			_, err := NewClient(stub, tt.cfg).Execute(t.Context(), RequestConfig{Model: "m"})

			if !tt.wantErr {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			appErr, ok := err.(*errorkit.AppError)
			if !ok {
				t.Fatalf("want *errorkit.AppError, got %T (%v)", err, err)
			}
			if appErr.Code() != errorkit.ERR_SYSTEM_CONFIG_INVALID {
				t.Fatalf("code = %s, want ERR_SYSTEM_CONFIG_INVALID", appErr.Code())
			}
			if stub.ctx != nil {
				t.Fatal("an invalid config must not reach the requester: the key would be sent")
			}
		})
	}
}
