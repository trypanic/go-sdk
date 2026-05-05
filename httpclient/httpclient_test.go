package httpclient

import (
	"crypto/tls"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/trypanic/go-sdk/errorkit"
)

func TestNewClientNoTransportWrapperByDefault(t *testing.T) {
	t.Parallel()

	client := NewDefaultClient()
	if _, ok := client.Transport.(*http.Transport); !ok {
		t.Fatalf("default client should expose *http.Transport, got %T", client.Transport)
	}
}

func TestWithTransportWrapperWraps(t *testing.T) {
	t.Parallel()

	type marker struct{ http.RoundTripper }

	client := NewClientWithOptions(WithTransportWrapper(func(rt http.RoundTripper) http.RoundTripper {
		return marker{rt}
	}))
	if _, ok := client.Transport.(marker); !ok {
		t.Fatalf("expected transport wrapper to be applied, got %T", client.Transport)
	}
}

func TestDefaultClientSetsTimeout(t *testing.T) {
	t.Parallel()

	client := NewDefaultClient()
	if client.Timeout != DefaultTimeout {
		t.Fatalf("expected default timeout %s, got %s", DefaultTimeout, client.Timeout)
	}
}

func TestWithTimeoutOverridesDefault(t *testing.T) {
	t.Parallel()

	client := NewClientWithOptions(WithTimeout(15 * time.Second))
	if client.Timeout != 15*time.Second {
		t.Fatalf("expected timeout override, got %s", client.Timeout)
	}
}

func TestSetupForLLMDisablesClientTimeout(t *testing.T) {
	t.Parallel()

	client := NewClient(SetupForLLM())
	if client.Timeout != 0 {
		t.Fatalf("expected LLM client timeout to be disabled, got %s", client.Timeout)
	}
}

func TestTLSConfigPrecedenceOverInsecureSkipVerify(t *testing.T) {
	t.Parallel()

	custom := &tls.Config{ServerName: "explicit.example.com"}
	client := NewClient(&ClientConfig{
		TLSConfig:          custom,
		InsecureSkipVerify: true,
	})
	tr := client.Transport.(*http.Transport)
	if tr.TLSClientConfig != custom {
		t.Fatalf("expected explicit TLSConfig to win, got %#v", tr.TLSClientConfig)
	}
}

func TestInsecureSkipVerifyBuildsTLSConfig(t *testing.T) {
	t.Parallel()

	client := NewClient(&ClientConfig{InsecureSkipVerify: true})
	tr := client.Transport.(*http.Transport)
	if tr.TLSClientConfig == nil || !tr.TLSClientConfig.InsecureSkipVerify {
		t.Fatalf("expected InsecureSkipVerify to be reflected in TLSClientConfig, got %#v", tr.TLSClientConfig)
	}
	if tr.TLSClientConfig.MinVersion != tls.VersionTLS13 {
		t.Fatalf("expected generated TLS config to require TLS 1.3, got %d", tr.TLSClientConfig.MinVersion)
	}
}

func TestDefaultTLSConfigRequiresTLS13(t *testing.T) {
	t.Parallel()

	client := NewDefaultClient()
	tr := client.Transport.(*http.Transport)
	if tr.TLSClientConfig == nil {
		t.Fatal("expected generated TLS config")
	}
	if tr.TLSClientConfig.MinVersion != tls.VersionTLS13 {
		t.Fatalf("expected generated TLS config to require TLS 1.3, got %d", tr.TLSClientConfig.MinVersion)
	}
}

func TestRedirectPolicyMaxRedirectsPositive(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/loop", http.StatusFound)
	}))
	defer srv.Close()

	client := NewClient(&ClientConfig{MaxRedirects: 2})
	resp, err := client.Get(srv.URL)
	if resp != nil {
		resp.Body.Close()
	}

	var appErr *errorkit.AppError
	var urlErr *url.Error
	if !errors.As(err, &urlErr) {
		t.Fatalf("expected url.Error from redirect ceiling, got %T (%v)", err, err)
	}
	if !errors.As(urlErr.Err, &appErr) {
		t.Fatalf("expected wrapped *errorkit.AppError, got %T", urlErr.Err)
	}
	if appErr.Code() != ERR_EXTERNAL_SERVICE_ERROR {
		t.Fatalf("unexpected error code: %s", appErr.Code())
	}
}

func TestRedirectPolicyMaxRedirectsZeroReturnsRedirectResponse(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/elsewhere", http.StatusFound)
	}))
	defer srv.Close()

	client := NewClient(&ClientConfig{MaxRedirects: 0})
	resp, err := client.Get(srv.URL)
	if err != nil {
		t.Fatalf("expected no error with MaxRedirects=0, got %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusFound {
		t.Fatalf("expected 302 to be returned to caller, got %d", resp.StatusCode)
	}
}

func TestRedirectPolicyMaxRedirectsNegativeUsesGoDefault(t *testing.T) {
	t.Parallel()

	client := NewClient(&ClientConfig{MaxRedirects: -1})
	if client.CheckRedirect != nil {
		t.Fatalf("MaxRedirects=-1 should leave CheckRedirect as Go default (nil)")
	}
}
