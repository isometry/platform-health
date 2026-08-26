package client_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/isometry/platform-health/pkg/client"
)

func TestDialConfigUseTLS(t *testing.T) {
	tests := []struct {
		name     string
		config   client.DialConfig
		expected bool
	}{
		{"plain port", client.DialConfig{Host: "localhost", Port: 8080}, false},
		{"explicit tls", client.DialConfig{Host: "localhost", Port: 8080, TLS: true}, true},
		{"implied by 443", client.DialConfig{Host: "example.com", Port: 443}, true},
		{"implied by 8443", client.DialConfig{Host: "example.com", Port: 8443}, true},
		{"insecure does not imply tls", client.DialConfig{Host: "localhost", Port: 8080, Insecure: true}, false},
		{"explicit tls on implied port", client.DialConfig{Host: "example.com", Port: 443, TLS: true}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.config.UseTLS())
		})
	}
}

func TestDialConfigAddress(t *testing.T) {
	tests := []struct {
		name     string
		config   client.DialConfig
		expected string
	}{
		{"hostname", client.DialConfig{Host: "example.com", Port: 8080}, "example.com:8080"},
		{"ipv4", client.DialConfig{Host: "127.0.0.1", Port: 443}, "127.0.0.1:443"},
		{"ipv6 is bracketed", client.DialConfig{Host: "::1", Port: 8080}, "[::1]:8080"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.config.Address())
		})
	}
}

func TestDialIsLazy(t *testing.T) {
	// grpc.NewClient performs no I/O, so dialling an address nothing listens on
	// must still succeed. This is the property ph ui relies on to hold one
	// connection for the process lifetime.
	conn, err := client.Dial(client.DialConfig{Host: "127.0.0.1", Port: 1})
	assert.NoError(t, err)
	assert.NotNil(t, conn)
	assert.NoError(t, conn.Close())
}
