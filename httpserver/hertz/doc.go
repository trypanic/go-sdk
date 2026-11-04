// Package hertz implements the SDK HTTP contracts using CloudWeGo Hertz.
//
// NewServer applies default middleware and functional options. Engine exposes
// the upstream server for advanced routing. Run installs no signal handlers;
// the caller owns Shutdown and its deadline.
package hertz
