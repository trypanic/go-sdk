package grpc

import (
	"time"

	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/keepalive"
)

// ServerKeepalive configures server-side keepalive pings and their
// enforcement policy. Zero-valued fields fall back to gRPC's own defaults
// (Time 2h, Timeout 20s, MinTime 5m), so leaving the struct empty preserves
// stock gRPC behavior.
//
// The MaxConnection* fields default to 0, which gRPC treats as infinity.
// They are opt-in on purpose: setting them places a hard cap on connection
// lifetime and will terminate otherwise-healthy long-lived streams. Do not
// set them unless you specifically want connections recycled.
type ServerKeepalive struct {
	// Time is the interval after which, if the server sees no activity, it
	// pings the client to check liveness. Maps to keepalive.ServerParameters.Time.
	Time time.Duration
	// Timeout is how long the server waits for a ping ack before considering
	// the connection dead. Maps to keepalive.ServerParameters.Timeout.
	Timeout time.Duration

	// MinTime is the minimum interval a client is allowed between pings before
	// the server flags it as abusive. Maps to keepalive.EnforcementPolicy.MinTime.
	MinTime time.Duration
	// PermitWithoutStream allows clients to send keepalive pings even when
	// there are no active streams. Maps to keepalive.EnforcementPolicy.PermitWithoutStream.
	PermitWithoutStream bool

	// MaxConnectionIdle caps how long a connection may stay idle before the
	// server closes it. Opt-in; 0 = infinity.
	MaxConnectionIdle time.Duration
	// MaxConnectionAge caps the total lifetime of a connection. Opt-in;
	// 0 = infinity. Terminates long-lived streams — use with care.
	MaxConnectionAge time.Duration
	// MaxConnectionAgeGrace is the grace period after MaxConnectionAge during
	// which in-flight RPCs may finish. Opt-in; 0 = infinity.
	MaxConnectionAgeGrace time.Duration
}

// ServerConfig holds the settings for a gRPC server.
type ServerConfig struct {
	// Address is the listen address in host:port form. Required.
	Address string
	// Keepalive configures server-side keepalive and its enforcement policy.
	Keepalive ServerKeepalive
	// MaxRecvMsgSize overrides the max received message size in bytes.
	// 0 = gRPC default (4 MiB).
	MaxRecvMsgSize int
	// MaxSendMsgSize overrides the max sent message size in bytes.
	// 0 = gRPC default (math.MaxInt32).
	MaxSendMsgSize int
}

// ClientKeepalive configures client-side keepalive pings.
type ClientKeepalive struct {
	// Time is the interval after which, if the client sees no activity, it
	// pings the server. Maps to keepalive.ClientParameters.Time.
	Time time.Duration
	// Timeout is how long the client waits for a ping ack before considering
	// the connection dead. Maps to keepalive.ClientParameters.Timeout.
	Timeout time.Duration
	// PermitWithoutStream allows the client to send pings even when there are
	// no active streams. Required to keep long-lived idle connections warm.
	PermitWithoutStream bool
}

// ClientConfig holds the settings for a gRPC client connection.
type ClientConfig struct {
	// Target is the dial target passed to grpc.NewClient. Required.
	Target string
	// Keepalive configures client-side keepalive.
	Keepalive ClientKeepalive
	// Creds is the transport security. When nil, an insecure credential is
	// used. This is the seam for wiring TLS.
	Creds credentials.TransportCredentials
	// MaxRecvMsgSize overrides the max received message size in bytes (per call).
	MaxRecvMsgSize int
	// MaxSendMsgSize overrides the max sent message size in bytes (per call).
	MaxSendMsgSize int
}

// serverParameters builds the keepalive.ServerParameters for this config.
func (k ServerKeepalive) serverParameters() keepalive.ServerParameters {
	return keepalive.ServerParameters{
		MaxConnectionIdle:     k.MaxConnectionIdle,
		MaxConnectionAge:      k.MaxConnectionAge,
		MaxConnectionAgeGrace: k.MaxConnectionAgeGrace,
		Time:                  k.Time,
		Timeout:               k.Timeout,
	}
}

// enforcementPolicy builds the keepalive.EnforcementPolicy for this config.
func (k ServerKeepalive) enforcementPolicy() keepalive.EnforcementPolicy {
	return keepalive.EnforcementPolicy{
		MinTime:             k.MinTime,
		PermitWithoutStream: k.PermitWithoutStream,
	}
}

// hasParams reports whether any keepalive.ServerParameters field is set, so we
// only override gRPC's defaults when the consumer asked for it.
func (k ServerKeepalive) hasParams() bool {
	return k.Time != 0 || k.Timeout != 0 ||
		k.MaxConnectionIdle != 0 || k.MaxConnectionAge != 0 || k.MaxConnectionAgeGrace != 0
}

// hasEnforcement reports whether any enforcement-policy field is set.
func (k ServerKeepalive) hasEnforcement() bool {
	return k.MinTime != 0 || k.PermitWithoutStream
}

// clientParameters builds the keepalive.ClientParameters for this config.
func (k ClientKeepalive) clientParameters() keepalive.ClientParameters {
	return keepalive.ClientParameters{
		Time:                k.Time,
		Timeout:             k.Timeout,
		PermitWithoutStream: k.PermitWithoutStream,
	}
}

// isSet reports whether any client keepalive field is set.
func (k ClientKeepalive) isSet() bool {
	return k.Time != 0 || k.Timeout != 0 || k.PermitWithoutStream
}
