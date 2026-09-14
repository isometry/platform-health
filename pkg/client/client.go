// Package client provides shared gRPC dialling for platform-health clients.
package client

import (
	"crypto/tls"
	"net"
	"slices"
	"strconv"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
)

// TLSPorts are ports on which TLS is implied when the TLS mode is TLSAuto.
var TLSPorts = []int{443, 8443}

// TLSMode selects transport security for a dial.
type TLSMode int

const (
	TLSAuto TLSMode = iota // TLS when the port is in TLSPorts, plaintext otherwise
	TLSOn                  // always TLS
	TLSOff                 // always plaintext, even on a TLSPorts port
)

// TLSModeFromPtr maps a provider's optional tls field: nil is TLSAuto, an
// explicit true or false is TLSOn or TLSOff.
func TLSModeFromPtr(b *bool) TLSMode {
	switch {
	case b == nil:
		return TLSAuto
	case *b:
		return TLSOn
	default:
		return TLSOff
	}
}

// TLSModeFromFlag maps a boolean CLI flag: true is TLSOn, false is TLSAuto.
func TLSModeFromFlag(b bool) TLSMode {
	if b {
		return TLSOn
	}
	return TLSAuto
}

// DialConfig describes how to reach a platform-health gRPC server.
type DialConfig struct {
	Host     string
	Port     int
	TLS      TLSMode
	Insecure bool // skip certificate verification
}

// Address returns the host:port dial target.
func (c DialConfig) Address() string {
	return net.JoinHostPort(c.Host, strconv.Itoa(c.Port))
}

// UseTLS reports whether the connection should use TLS.
// Pure function of immutable fields: do not cache or write back the result.
func (c DialConfig) UseTLS() bool {
	switch c.TLS {
	case TLSOn:
		return true
	case TLSOff:
		return false
	}
	return slices.Contains(TLSPorts, c.Port)
}

// Dial returns a lazily-connecting ClientConn. No I/O until the first RPC.
// Callers own Close() and any backoff, keepalive or call-size policy.
func Dial(cfg DialConfig, opts ...grpc.DialOption) (*grpc.ClientConn, error) {
	if cfg.UseTLS() {
		opts = append(opts, grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{
			ServerName:         cfg.Host,
			InsecureSkipVerify: cfg.Insecure,
		})))
	} else {
		opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}
	return grpc.NewClient(cfg.Address(), opts...)
}
