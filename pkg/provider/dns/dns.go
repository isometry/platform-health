package dns

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/google/cel-go/cel"
	"github.com/mcuadros/go-defaults"
	"github.com/miekg/dns"
	"google.golang.org/protobuf/types/known/anypb"

	"github.com/isometry/platform-health/pkg/checks"
	"github.com/isometry/platform-health/pkg/phctx"
	ph "github.com/isometry/platform-health/pkg/platform_health"
	"github.com/isometry/platform-health/pkg/platform_health/details"
	"github.com/isometry/platform-health/pkg/provider"
)

const (
	ProviderType   = "dns"
	DefaultTimeout = 5 * time.Second
)

type Component struct {
	provider.Base
	provider.BaseWithChecks

	Host       string `mapstructure:"host"`
	Server     string `mapstructure:"server"`           // DNS server IP (no port)
	Port       int    `mapstructure:"port" default:"0"` // 0 = auto (53/853 based on TLS)
	ServerName string `mapstructure:"serverName"`       // TLS server name (for DoT with IP addresses)
	Type       string `mapstructure:"type" default:"A"`
	TLS        bool   `mapstructure:"tls"`
	DNSSEC     bool   `mapstructure:"dnssec"`
	Detail     bool   `mapstructure:"detail"`
}

var _ provider.InstanceWithChecks = (*Component)(nil)

// CEL configuration for DNS provider
var celConfig = checks.NewCEL(
	cel.Variable("records", cel.ListType(cel.MapType(cel.StringType, cel.DynType))),
	cel.Variable("dnssec", cel.MapType(cel.StringType, cel.DynType)),
)

func init() {
	provider.Register(ProviderType, new(Component))
}

func (c *Component) LogValue() slog.Value {
	logAttr := []slog.Attr{
		slog.String("name", c.GetName()),
		slog.String("host", c.Host),
		slog.String("type", c.Type),
	}
	if c.Server != "" {
		logAttr = append(logAttr, slog.String("server", c.Server))
	}
	if c.TLS {
		logAttr = append(logAttr, slog.Bool("tls", c.TLS))
	}
	if c.DNSSEC {
		logAttr = append(logAttr, slog.Bool("dnssec", c.DNSSEC))
	}
	return slog.GroupValue(logAttr...)
}

func (c *Component) Setup() error {
	if c.GetTimeout() == 0 {
		c.SetTimeout(DefaultTimeout)
	}
	defaults.SetDefaults(c)

	// Validate record type
	if _, ok := dns.StringToType[strings.ToUpper(c.Type)]; !ok {
		return fmt.Errorf("invalid record type: %s", c.Type)
	}

	return nil
}

// SetChecks sets and compiles CEL expressions.
func (c *Component) SetChecks(exprs []checks.Expression) error {
	return c.SetChecksAndCompile(exprs, celConfig)
}

func (c *Component) GetType() string {
	return ProviderType
}

// GetCheckConfig returns the DNS provider's CEL variable declarations.
func (c *Component) GetCheckConfig() *checks.CEL {
	return celConfig
}

// GetCheckContext performs a DNS query and returns the CEL evaluation context.
func (c *Component) GetCheckContext(ctx context.Context) (map[string]any, error) {
	resp, err := c.executeQuery(ctx)
	if err != nil {
		return nil, err
	}

	// Check for DNS errors (NXDOMAIN, SERVFAIL, etc.)
	if resp.Rcode != dns.RcodeSuccess {
		return nil, fmt.Errorf("DNS error: %s", dns.RcodeToString[resp.Rcode])
	}

	records := parseRecords(resp)
	dnssecStatus := parseDNSSECStatus(resp, c.DNSSEC)

	return map[string]any{
		"records": records,
		"dnssec":  dnssecStatus,
	}, nil
}

func (c *Component) GetHealth(ctx context.Context) *ph.HealthCheckResponse {
	log := phctx.Logger(ctx, slog.String("provider", ProviderType), slog.Any("instance", c))
	log.Debug("checking")

	component := &ph.HealthCheckResponse{
		Type: ProviderType,
		Name: c.GetName(),
	}
	defer component.LogStatus(log)

	// Get check context (performs DNS query)
	checkCtx, err := c.GetCheckContext(ctx)
	if err != nil {
		return component.Unhealthy(err.Error())
	}

	// Build detail if requested
	if c.Detail {
		records := checkCtx["records"].([]map[string]any)
		dnssecStatus := checkCtx["dnssec"].(map[string]any)
		if detail, err := anypb.New(detailFromContext(c, records, dnssecStatus)); err == nil {
			component.Details = append(component.Details, detail)
		} else {
			slog.Warn("failed to serialize DNS detail", "host", c.Host, "error", err)
		}
	}

	// Apply CEL checks
	if msgs := c.EvaluateChecks(ctx, checkCtx); len(msgs) > 0 {
		return component.Unhealthy(msgs...)
	}

	return component.Healthy()
}

func (c *Component) executeQuery(ctx context.Context) (*dns.Msg, error) {
	msg := new(dns.Msg)
	msg.SetQuestion(dns.Fqdn(c.Host), dns.StringToType[strings.ToUpper(c.Type)])
	if c.DNSSEC {
		msg.SetEdns0(4096, true) // Enable DO flag
	}

	client := &dns.Client{}

	server := c.Server
	if server == "" {
		// Use system resolver
		config, err := dns.ClientConfigFromFile("/etc/resolv.conf")
		if err != nil {
			return nil, fmt.Errorf("failed to read system DNS config: %w", err)
		}
		if len(config.Servers) == 0 {
			return nil, fmt.Errorf("no DNS servers configured")
		}
		server = config.Servers[0]
	}

	// Determine port: explicit > auto-select based on TLS
	port := c.Port
	if port == 0 {
		if c.TLS {
			port = 853
		} else {
			port = 53
		}
	}
	serverAddr := net.JoinHostPort(server, strconv.Itoa(port))

	if c.TLS {
		client.Net = "tcp-tls"
		// Determine TLS server name
		serverName := c.ServerName
		if serverName == "" {
			serverName = server // Use server IP/hostname directly
		}
		client.TLSConfig = &tls.Config{
			ServerName: serverName,
		}
	}

	resp, _, err := client.ExchangeContext(ctx, msg, serverAddr)
	return resp, err
}

func parseRecords(resp *dns.Msg) []map[string]any {
	records := make([]map[string]any, 0, len(resp.Answer))

	for _, rr := range resp.Answer {
		record := map[string]any{
			"type": dns.TypeToString[rr.Header().Rrtype],
			"ttl":  rr.Header().Ttl,
		}

		switch r := rr.(type) {
		case *dns.A:
			record["value"] = r.A.String()
		case *dns.AAAA:
			record["value"] = r.AAAA.String()
		case *dns.CNAME:
			record["value"] = r.Target
			record["target"] = r.Target
		case *dns.MX:
			record["value"] = r.Mx
			record["target"] = r.Mx
			record["priority"] = r.Preference
		case *dns.TXT:
			record["value"] = strings.Join(r.Txt, "")
		case *dns.NS:
			record["value"] = r.Ns
			record["target"] = r.Ns
		case *dns.SOA:
			record["value"] = r.Ns
			record["target"] = r.Ns
		case *dns.SRV:
			record["value"] = r.Target
			record["target"] = r.Target
			record["priority"] = r.Priority
			record["weight"] = r.Weight
			record["port"] = r.Port
		case *dns.PTR:
			record["value"] = r.Ptr
			record["target"] = r.Ptr
		default:
			// For unsupported types, use string representation
			record["value"] = rr.String()
		}

		records = append(records, record)
	}

	return records
}

func parseDNSSECStatus(resp *dns.Msg, enabled bool) map[string]any {
	return map[string]any{
		"enabled":       enabled,
		"authenticated": resp.AuthenticatedData,
	}
}

func detailFromContext(c *Component, records []map[string]any, dnssecStatus map[string]any) *details.Detail_DNS {
	detail := &details.Detail_DNS{
		Host:      c.Host,
		Server:    c.Server,
		QueryType: c.Type,
		Records:   make([]*details.DNSRecord, 0, len(records)),
		Dnssec: &details.DNSSECStatus{
			Enabled:       dnssecStatus["enabled"].(bool),
			Authenticated: dnssecStatus["authenticated"].(bool),
		},
	}

	for _, r := range records {
		rec := &details.DNSRecord{
			Type:  r["type"].(string),
			Value: r["value"].(string),
			Ttl:   r["ttl"].(uint32),
		}
		if priority, ok := r["priority"].(uint16); ok {
			rec.Priority = uint32(priority)
		}
		if weight, ok := r["weight"].(uint16); ok {
			rec.Weight = uint32(weight)
		}
		if port, ok := r["port"].(uint16); ok {
			rec.Port = uint32(port)
		}
		if target, ok := r["target"].(string); ok {
			rec.Target = target
		}
		detail.Records = append(detail.Records, rec)
	}

	return detail
}
