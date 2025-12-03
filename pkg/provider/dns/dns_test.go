package dns_test

import (
	"log/slog"
	"net"
	"testing"
	"time"

	mdns "github.com/miekg/dns"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/isometry/platform-health/pkg/checks"
	ph "github.com/isometry/platform-health/pkg/platform_health"
	"github.com/isometry/platform-health/pkg/provider"
	"github.com/isometry/platform-health/pkg/provider/dns"
)

func init() {
	slog.SetLogLoggerLevel(slog.LevelError)
}

// mockDNSHandler returns a DNS handler that responds with configured records.
type mockDNSHandler struct {
	records map[uint16][]mdns.RR // keyed by query type
	dnssec  bool                 // whether to set AD flag
}

func (h *mockDNSHandler) ServeDNS(w mdns.ResponseWriter, r *mdns.Msg) {
	m := new(mdns.Msg)
	m.SetReply(r)
	m.Authoritative = true
	m.AuthenticatedData = h.dnssec

	if len(r.Question) > 0 {
		qtype := r.Question[0].Qtype
		qname := r.Question[0].Name

		// Check for NXDOMAIN test case
		if qname == "nonexistent.example.com." {
			m.Rcode = mdns.RcodeNameError
		} else if rrs, ok := h.records[qtype]; ok {
			m.Answer = rrs
		}
	}

	_ = w.WriteMsg(m)
}

// setupMockDNSServer creates a mock DNS server and returns the port number.
func setupMockDNSServer(t *testing.T, records map[uint16][]mdns.RR, dnssec bool) int {
	t.Helper()

	// Bind to random available port
	addr, err := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	require.NoError(t, err)

	conn, err := net.ListenUDP("udp", addr)
	require.NoError(t, err)

	handler := &mockDNSHandler{records: records, dnssec: dnssec}
	server := &mdns.Server{PacketConn: conn, Handler: handler}

	go func() { _ = server.ActivateAndServe() }()
	t.Cleanup(func() { _ = server.Shutdown() })

	return conn.LocalAddr().(*net.UDPAddr).Port
}

// testRecords returns standard test DNS records using RFC-reserved addresses.
func testRecords() map[uint16][]mdns.RR {
	return map[uint16][]mdns.RR{
		mdns.TypeA: {
			&mdns.A{
				Hdr: mdns.RR_Header{Name: "example.com.", Rrtype: mdns.TypeA, Class: mdns.ClassINET, Ttl: 300},
				A:   net.ParseIP("192.0.2.1"), // TEST-NET-1 (RFC 5737)
			},
		},
		mdns.TypeAAAA: {
			&mdns.AAAA{
				Hdr:  mdns.RR_Header{Name: "example.com.", Rrtype: mdns.TypeAAAA, Class: mdns.ClassINET, Ttl: 300},
				AAAA: net.ParseIP("2001:db8::1"), // Documentation prefix (RFC 3849)
			},
		},
		mdns.TypeMX: {
			&mdns.MX{
				Hdr:        mdns.RR_Header{Name: "example.com.", Rrtype: mdns.TypeMX, Class: mdns.ClassINET, Ttl: 300},
				Preference: 10,
				Mx:         "mail.example.com.",
			},
		},
		mdns.TypeTXT: {
			&mdns.TXT{
				Hdr: mdns.RR_Header{Name: "example.com.", Rrtype: mdns.TypeTXT, Class: mdns.ClassINET, Ttl: 300},
				Txt: []string{"v=spf1 include:example.com ~all"},
			},
		},
		mdns.TypeNS: {
			&mdns.NS{
				Hdr: mdns.RR_Header{Name: "example.com.", Rrtype: mdns.TypeNS, Class: mdns.ClassINET, Ttl: 300},
				Ns:  "ns1.example.com.",
			},
		},
	}
}

func TestDNS(t *testing.T) {
	// Setup mock DNS server with test records
	serverPort := setupMockDNSServer(t, testRecords(), false)
	dnssecServerPort := setupMockDNSServer(t, testRecords(), true)

	tests := []struct {
		name       string
		host       string
		recordType string
		port       int
		tls        bool
		dnssec     bool
		timeout    time.Duration
		expected   ph.Status
	}{
		{
			name:       "A record lookup",
			host:       "example.com",
			recordType: "A",
			port:       serverPort,
			timeout:    5 * time.Second,
			expected:   ph.Status_HEALTHY,
		},
		{
			name:       "AAAA record lookup",
			host:       "example.com",
			recordType: "AAAA",
			port:       serverPort,
			timeout:    5 * time.Second,
			expected:   ph.Status_HEALTHY,
		},
		{
			name:       "MX record lookup",
			host:       "example.com",
			recordType: "MX",
			port:       serverPort,
			timeout:    5 * time.Second,
			expected:   ph.Status_HEALTHY,
		},
		{
			name:       "TXT record lookup",
			host:       "example.com",
			recordType: "TXT",
			port:       serverPort,
			timeout:    5 * time.Second,
			expected:   ph.Status_HEALTHY,
		},
		{
			name:       "NS record lookup",
			host:       "example.com",
			recordType: "NS",
			port:       serverPort,
			timeout:    5 * time.Second,
			expected:   ph.Status_HEALTHY,
		},
		{
			name:       "Custom DNS server",
			host:       "example.com",
			recordType: "A",
			port:       serverPort,
			timeout:    5 * time.Second,
			expected:   ph.Status_HEALTHY,
		},
		{
			name:       "DNSSEC enabled",
			host:       "example.com",
			recordType: "A",
			port:       dnssecServerPort,
			dnssec:     true,
			timeout:    5 * time.Second,
			expected:   ph.Status_HEALTHY,
		},
		{
			name:       "NXDOMAIN - nonexistent domain",
			host:       "nonexistent.example.com",
			recordType: "A",
			port:       serverPort,
			timeout:    5 * time.Second,
			expected:   ph.Status_UNHEALTHY,
		},
		{
			name:       "Timeout - very short",
			host:       "example.com",
			recordType: "A",
			port:       serverPort,
			timeout:    1 * time.Nanosecond,
			expected:   ph.Status_UNHEALTHY,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			instance := &dns.Component{
				Host:   tt.host,
				Type:   tt.recordType,
				Server: "127.0.0.1",
				Port:   tt.port,
				TLS:    tt.tls,
				DNSSEC: tt.dnssec,
			}
			instance.SetName(tt.name)
			instance.SetTimeout(tt.timeout)
			require.NoError(t, instance.Setup())

			// Use GetHealthWithDuration which applies the timeout
			result := provider.GetHealthWithDuration(t.Context(), instance)

			assert.NotNil(t, result)
			assert.Equal(t, dns.ProviderKind, result.GetKind())
			assert.Equal(t, tt.name, result.GetName())
			assert.Equal(t, tt.expected, result.GetStatus())
		})
	}
}

func TestDNS_CELChecks(t *testing.T) {
	// Setup mock DNS server with test records
	serverPort := setupMockDNSServer(t, testRecords(), false)

	tests := []struct {
		name     string
		host     string
		checks   []checks.Expression
		expected ph.Status
	}{
		{
			name: "Check passes - records exist",
			host: "example.com",
			checks: []checks.Expression{
				{Expression: "size(records) > 0", Message: "No records found"},
			},
			expected: ph.Status_HEALTHY,
		},
		{
			name: "Check passes - A record exists",
			host: "example.com",
			checks: []checks.Expression{
				{Expression: `records.exists(r, r.type == "A")`, Message: "No A records found"},
			},
			expected: ph.Status_HEALTHY,
		},
		{
			name: "Check fails - no AAAA when expecting",
			host: "example.com",
			checks: []checks.Expression{
				{Expression: `records.all(r, r.type == "AAAA")`, Message: "Expected only AAAA records"},
			},
			expected: ph.Status_UNHEALTHY,
		},
		{
			name: "Check TTL",
			host: "example.com",
			checks: []checks.Expression{
				{Expression: "records.all(r, r.ttl > 0)", Message: "TTL must be positive"},
			},
			expected: ph.Status_HEALTHY,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			instance := &dns.Component{
				Host:   tt.host,
				Type:   "A",
				Server: "127.0.0.1",
				Port:   serverPort,
			}
			instance.SetName(tt.name)
			instance.SetTimeout(5 * time.Second)
			require.NoError(t, instance.SetChecks(tt.checks))
			require.NoError(t, instance.Setup())

			result := instance.GetHealth(t.Context())

			assert.NotNil(t, result)
			assert.Equal(t, tt.expected, result.GetStatus())
		})
	}
}

func TestDNS_InvalidRecordType(t *testing.T) {
	instance := &dns.Component{
		Host: "google.com",
		Type: "INVALID",
	}
	instance.SetName("invalid-type")
	instance.SetTimeout(5 * time.Second)

	err := instance.Setup()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid record type")
}

func TestDNS_GetCheckContext(t *testing.T) {
	// Setup mock DNS server with DNSSEC enabled
	serverPort := setupMockDNSServer(t, testRecords(), true)

	instance := &dns.Component{
		Host:   "example.com",
		Type:   "A",
		Server: "127.0.0.1",
		Port:   serverPort,
		DNSSEC: true,
	}
	instance.SetName("check-context")
	instance.SetTimeout(5 * time.Second)
	require.NoError(t, instance.Setup())

	ctx, err := instance.GetCheckContext(t.Context())
	require.NoError(t, err)

	// Verify records are present
	records, ok := ctx["records"].([]map[string]any)
	require.True(t, ok, "records should be a slice of maps")
	assert.Greater(t, len(records), 0, "should have at least one record")

	// Verify first record has expected fields
	if len(records) > 0 {
		assert.Contains(t, records[0], "type")
		assert.Contains(t, records[0], "value")
		assert.Contains(t, records[0], "ttl")
	}

	// Verify dnssec status
	dnssec, ok := ctx["dnssec"].(map[string]any)
	require.True(t, ok, "dnssec should be a map")
	assert.Contains(t, dnssec, "enabled")
	assert.Contains(t, dnssec, "authenticated")
	assert.True(t, dnssec["enabled"].(bool), "DNSSEC should be enabled")
}

func TestDNS_ProviderInterface(t *testing.T) {
	instance := &dns.Component{
		Host: "example.com",
		Type: "A",
	}
	instance.SetName("test-instance")
	require.NoError(t, instance.Setup())

	assert.Equal(t, dns.ProviderKind, instance.GetKind())
	assert.Equal(t, "test-instance", instance.GetName())

	instance.SetName("renamed")
	assert.Equal(t, "renamed", instance.GetName())
}

func TestDNS_DoT(t *testing.T) {
	// DNS-over-TLS test - requires port 853 to be open
	// Skip if DoT is not available in the network environment
	instance := &dns.Component{
		Host:       "cloudflare.com",
		Type:       "A",
		Server:     "1.1.1.1",            // Port auto-selected as 853 due to TLS
		ServerName: "cloudflare-dns.com", // Required for TLS with IP address
		TLS:        true,
	}
	instance.SetName("dot-test")
	instance.SetTimeout(5 * time.Second)
	require.NoError(t, instance.Setup())

	result := instance.GetHealth(t.Context())

	if result.GetStatus() == ph.Status_UNHEALTHY {
		t.Skip("DNS-over-TLS not available (port 853 may be blocked)")
	}

	assert.Equal(t, ph.Status_HEALTHY, result.GetStatus())
}
