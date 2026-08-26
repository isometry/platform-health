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

// TLSPorts are ports for which TLS is implied even when DialConfig.TLS is false.
var TLSPorts = []int{443, 8443}

// DialConfig describes how to reach a platform-health gRPC server.
type DialConfig struct {
	Host     string
	Port     int
	TLS      bool // force TLS; also implied by TLSPorts
	Insecure bool // skip certificate verification
}

// Address returns the host:port dial target.
func (c DialConfig) Address() string {
	return net.JoinHostPort(c.Host, strconv.Itoa(c.Port))
}

// UseTLS reports whether the connection should use TLS.
// Pure function of immutable fields: do not cache or write back the result.
func (c DialConfig) UseTLS() bool {
	return c.TLS || slices.Contains(TLSPorts, c.Port)
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
