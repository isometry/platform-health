package flags

import (
	"fmt"
	"strings"
	"time"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/anypb"

	"github.com/isometry/platform-health/pkg/platform_health/details"
)

// RenderDetail renders a single protobuf Any detail to human-friendly text.
// Falls back to JSON for unknown types.
func RenderDetail(detail *anypb.Any, indent string) string {
	if detail == nil {
		return ""
	}

	typeURL := detail.GetTypeUrl()

	switch {
	case strings.HasSuffix(typeURL, "Detail_TLS"):
		return renderTLSDetail(detail, indent)
	case strings.HasSuffix(typeURL, "Detail_KStatus"):
		return renderKStatusDetail(detail, indent)
	case strings.HasSuffix(typeURL, "Detail_DNS"):
		return renderDNSDetail(detail, indent)
	case strings.HasSuffix(typeURL, "Detail_Loop"):
		return renderLoopDetail(detail, indent)
	default:
		return renderUnknownDetail(detail, indent)
	}
}

// RenderDetails renders all details to human-friendly text.
func RenderDetails(detailList []*anypb.Any, indent string) string {
	if len(detailList) == 0 {
		return ""
	}

	var sb strings.Builder
	for i, d := range detailList {
		if i > 0 {
			sb.WriteString("\n")
		}
		sb.WriteString(RenderDetail(d, indent))
	}
	return sb.String()
}

func renderTLSDetail(detail *anypb.Any, indent string) string {
	var tls details.Detail_TLS
	if err := detail.UnmarshalTo(&tls); err != nil {
		return renderUnknownDetail(detail, indent)
	}

	var sb strings.Builder
	sb.WriteString(indent + "TLS Certificate:\n")

	if tls.CommonName != "" {
		sb.WriteString(fmt.Sprintf("%s  Common Name: %s\n", indent, tls.CommonName))
	}

	if len(tls.SubjectAltNames) > 0 {
		sb.WriteString(fmt.Sprintf("%s  Subject Alt Names: %s\n", indent, strings.Join(tls.SubjectAltNames, ", ")))
	}

	if tls.ValidUntil != nil {
		validUntil := tls.ValidUntil.AsTime()
		remaining := time.Until(validUntil)
		sb.WriteString(fmt.Sprintf("%s  Valid Until: %s (%s)\n", indent, validUntil.Format(time.RFC3339), formatDuration(remaining)))
	}

	if tls.Version != "" {
		sb.WriteString(fmt.Sprintf("%s  TLS Version: %s\n", indent, tls.Version))
	}

	if tls.Protocol != "" {
		sb.WriteString(fmt.Sprintf("%s  Protocol: %s\n", indent, tls.Protocol))
	}

	if tls.CipherSuite != "" {
		sb.WriteString(fmt.Sprintf("%s  Cipher Suite: %s\n", indent, tls.CipherSuite))
	}

	if tls.SignatureAlgorithm != "" {
		sb.WriteString(fmt.Sprintf("%s  Signature Algorithm: %s\n", indent, tls.SignatureAlgorithm))
	}

	if tls.PublicKeyAlgorithm != "" {
		sb.WriteString(fmt.Sprintf("%s  Public Key Algorithm: %s\n", indent, tls.PublicKeyAlgorithm))
	}

	if len(tls.Chain) > 0 {
		sb.WriteString(fmt.Sprintf("%s  Certificate Chain:\n", indent))
		for i, cert := range tls.Chain {
			sb.WriteString(fmt.Sprintf("%s    [%d] %s\n", indent, i, cert))
		}
	}

	return sb.String()
}

func renderKStatusDetail(detail *anypb.Any, indent string) string {
	var kstatus details.Detail_KStatus
	if err := detail.UnmarshalTo(&kstatus); err != nil {
		return renderUnknownDetail(detail, indent)
	}

	var sb strings.Builder
	sb.WriteString(indent + "Kubernetes Status:\n")

	if kstatus.Status != "" {
		sb.WriteString(fmt.Sprintf("%s  Status: %s\n", indent, kstatus.Status))
	}

	if kstatus.Message != "" {
		sb.WriteString(fmt.Sprintf("%s  Message: %s\n", indent, kstatus.Message))
	}

	if len(kstatus.Conditions) > 0 {
		sb.WriteString(fmt.Sprintf("%s  Conditions:\n", indent))
		for _, cond := range kstatus.Conditions {
			line := fmt.Sprintf("%s=%s", cond.Type, cond.Status)
			if cond.Reason != "" {
				line += fmt.Sprintf(" (%s)", cond.Reason)
			}
			if cond.Message != "" {
				line += fmt.Sprintf(": %s", cond.Message)
			}
			sb.WriteString(fmt.Sprintf("%s    - %s\n", indent, line))
		}
	}

	return sb.String()
}

func renderDNSDetail(detail *anypb.Any, indent string) string {
	var dns details.Detail_DNS
	if err := detail.UnmarshalTo(&dns); err != nil {
		return renderUnknownDetail(detail, indent)
	}

	var sb strings.Builder
	sb.WriteString(indent + "DNS Query:\n")

	if dns.Host != "" {
		sb.WriteString(fmt.Sprintf("%s  Host: %s\n", indent, dns.Host))
	}

	if dns.Server != "" {
		sb.WriteString(fmt.Sprintf("%s  Server: %s\n", indent, dns.Server))
	}

	if dns.QueryType != "" {
		sb.WriteString(fmt.Sprintf("%s  Query Type: %s\n", indent, dns.QueryType))
	}

	if len(dns.Records) > 0 {
		sb.WriteString(fmt.Sprintf("%s  Records:\n", indent))
		for _, rec := range dns.Records {
			sb.WriteString(fmt.Sprintf("%s    - %s\n", indent, formatDNSRecord(rec)))
		}
	}

	if dns.Dnssec != nil {
		status := "disabled"
		if dns.Dnssec.Enabled {
			if dns.Dnssec.Authenticated {
				status = "enabled, authenticated"
			} else {
				status = "enabled, not authenticated"
			}
		}
		sb.WriteString(fmt.Sprintf("%s  DNSSEC: %s\n", indent, status))
	}

	return sb.String()
}

func renderLoopDetail(detail *anypb.Any, indent string) string {
	var loop details.Detail_Loop
	if err := detail.UnmarshalTo(&loop); err != nil {
		return renderUnknownDetail(detail, indent)
	}

	var sb strings.Builder
	sb.WriteString(indent + "Loop Detection:\n")

	if len(loop.ServerIds) > 0 {
		sb.WriteString(fmt.Sprintf("%s  Server Chain: %s\n", indent, strings.Join(loop.ServerIds, " -> ")))
	}

	return sb.String()
}

func renderUnknownDetail(detail *anypb.Any, indent string) string {
	typeURL := detail.GetTypeUrl()
	typeName := typeURL
	if idx := strings.LastIndex(typeURL, "/"); idx >= 0 {
		typeName = typeURL[idx+1:]
	}

	// Use protojson for readable JSON output
	jsonBytes, err := protojson.MarshalOptions{Multiline: true, Indent: "  "}.Marshal(detail)
	if err != nil {
		return fmt.Sprintf("%sDetail (%s): [unmarshal error]\n", indent, typeName)
	}

	// Indent each line of JSON
	lines := strings.Split(string(jsonBytes), "\n")
	for i, line := range lines {
		lines[i] = indent + "  " + line
	}

	return fmt.Sprintf("%sDetail (%s):\n%s\n", indent, typeName, strings.Join(lines, "\n"))
}

func formatDNSRecord(rec *details.DNSRecord) string {
	switch rec.Type {
	case "A", "AAAA":
		return fmt.Sprintf("%s %s (TTL: %ds)", rec.Type, rec.Value, rec.Ttl)
	case "CNAME":
		return fmt.Sprintf("CNAME -> %s (TTL: %ds)", rec.Value, rec.Ttl)
	case "MX":
		return fmt.Sprintf("MX %d %s (TTL: %ds)", rec.Priority, rec.Value, rec.Ttl)
	case "SRV":
		return fmt.Sprintf("SRV %d %d %d %s (TTL: %ds)", rec.Priority, rec.Weight, rec.Port, rec.Target, rec.Ttl)
	case "TXT":
		return fmt.Sprintf("TXT \"%s\" (TTL: %ds)", rec.Value, rec.Ttl)
	default:
		return fmt.Sprintf("%s %s (TTL: %ds)", rec.Type, rec.Value, rec.Ttl)
	}
}

func formatDuration(d time.Duration) string {
	if d < 0 {
		return "expired"
	}

	days := int(d.Hours() / 24)
	if days > 30 {
		return fmt.Sprintf("%dd remaining", days)
	} else if days > 0 {
		return fmt.Sprintf("%dd %dh remaining", days, int(d.Hours())%24)
	} else if d.Hours() >= 1 {
		return fmt.Sprintf("%dh %dm remaining", int(d.Hours()), int(d.Minutes())%60)
	} else if d.Minutes() >= 1 {
		return fmt.Sprintf("%dm remaining", int(d.Minutes()))
	}
	return fmt.Sprintf("%ds remaining", int(d.Seconds()))
}
