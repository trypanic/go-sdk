package grpc

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/wrapperspb"

	"github.com/trypanic/go-sdk/errorkit"
	"github.com/trypanic/go-sdk/grpc/internal/testsvc"
)

// testTLS generates, in memory, a self-signed certificate for the bufconn
// authority "bufnet" and returns matching server and client credentials. No
// key material is written to disk.
func testTLS(t *testing.T) (server, client credentials.TransportCredentials) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "bufnet"},
		DNSNames:              []string{"bufnet"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create certificate: %v", err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("parse certificate: %v", err)
	}
	pool := x509.NewCertPool()
	pool.AddCert(cert)
	server = credentials.NewServerTLSFromCert(&tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key})
	client = credentials.NewClientTLSFromCert(pool, "bufnet")
	return server, client
}

func TestTLS_RoundTripWithConfigCreds(t *testing.T) {
	t.Parallel()
	serverCreds, clientCreds := testTLS(t)
	srv, lis := serve(t, ServerConfig{Creds: serverCreds}, &echoImpl{})
	t.Cleanup(srv.Stop)
	cc := connect(t, lis, ClientConfig{Creds: clientCreds})
	t.Cleanup(func() { _ = cc.Close() })

	out := new(wrapperspb.StringValue)
	if err := cc.Invoke(testCtx(t), testsvc.UnaryMethod, wrapperspb.String("ping"), out); err != nil {
		t.Fatalf("Invoke over TLS: %v", err)
	}
	if out.GetValue() != "ping-pong" {
		t.Fatalf("got %q, want %q", out.GetValue(), "ping-pong")
	}
}

// The server really enforces TLS: a plaintext client cannot call it.
func TestTLS_PlaintextClientRejectedByTLSServer(t *testing.T) {
	t.Parallel()
	serverCreds, _ := testTLS(t)
	srv, lis := serve(t, ServerConfig{Creds: serverCreds}, &echoImpl{})
	t.Cleanup(srv.Stop)
	cc := connect(t, lis, ClientConfig{}, WithClientInsecure())
	t.Cleanup(func() { _ = cc.Close() })

	err := cc.Invoke(testCtx(t), testsvc.UnaryMethod, wrapperspb.String("ping"), new(wrapperspb.StringValue))
	if status.Code(err) != codes.Unavailable {
		t.Fatalf("plaintext call to a TLS server: code = %s (err %v), want %s", status.Code(err), err, codes.Unavailable)
	}
}

// Asking for credentials and for plaintext at the same time is a
// configuration error, not a silent pick.
func TestCreds_ConflictWithInsecureIsConfigError(t *testing.T) {
	t.Parallel()
	serverCreds, clientCreds := testTLS(t)

	_, err := New(ServerConfig{Address: "bufnet", Creds: serverCreds}, WithServerInsecure())
	wantCode(t, err, errorkit.ERR_SYSTEM_CONFIG_INVALID)

	_, err = Dial(ClientConfig{Target: "passthrough:///bufnet", Creds: clientCreds}, WithClientInsecure())
	wantCode(t, err, errorkit.ERR_SYSTEM_CONFIG_INVALID)
}

// Plaintext is never a silent default: nil credentials without the explicit
// insecure option are a configuration error on both sides.
func TestCreds_NilWithoutInsecureIsConfigError(t *testing.T) {
	t.Parallel()
	_, err := New(ServerConfig{Address: "bufnet"})
	wantCode(t, err, errorkit.ERR_SYSTEM_CONFIG_INVALID)

	_, err = Dial(ClientConfig{Target: "passthrough:///bufnet"})
	wantCode(t, err, errorkit.ERR_SYSTEM_CONFIG_INVALID)
}

func TestCreds_NilWithInsecureIsAccepted(t *testing.T) {
	t.Parallel()
	srv, err := New(ServerConfig{Address: "bufnet"}, WithServerInsecure())
	if err != nil {
		t.Fatalf("New with WithServerInsecure: %v", err)
	}
	srv.Stop()

	cc, err := Dial(ClientConfig{Target: "passthrough:///bufnet"}, WithClientInsecure())
	if err != nil {
		t.Fatalf("Dial with WithClientInsecure: %v", err)
	}
	_ = cc.Close()
}
