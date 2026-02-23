package details

import (
	"fmt"
	"strings"
	"time"
)

// Render implements the Renderer interface for TLS certificate details.
func (d *Detail_TLS) Render(indent string) string {
	var sb strings.Builder
	sb.WriteString(indent + "TLS Certificate:\n")

	if d.CommonName != "" {
		fmt.Fprintf(&sb, "%s  Common Name: %s\n", indent, d.CommonName)
	}

	if len(d.SubjectAltNames) > 0 {
		fmt.Fprintf(&sb, "%s  Subject Alt Names: %s\n", indent, strings.Join(d.SubjectAltNames, ", "))
	}

	if d.ValidUntil != nil {
		validUntil := d.ValidUntil.AsTime()
		remaining := time.Until(validUntil)
		fmt.Fprintf(&sb, "%s  Valid Until: %s (%s)\n", indent, validUntil.Format(time.RFC3339), FormatDuration(remaining))
	}

	if d.Version != "" {
		fmt.Fprintf(&sb, "%s  TLS Version: %s\n", indent, d.Version)
	}

	if d.Protocol != "" {
		fmt.Fprintf(&sb, "%s  Protocol: %s\n", indent, d.Protocol)
	}

	if d.CipherSuite != "" {
		fmt.Fprintf(&sb, "%s  Cipher Suite: %s\n", indent, d.CipherSuite)
	}

	if d.SignatureAlgorithm != "" {
		fmt.Fprintf(&sb, "%s  Signature Algorithm: %s\n", indent, d.SignatureAlgorithm)
	}

	if d.PublicKeyAlgorithm != "" {
		fmt.Fprintf(&sb, "%s  Public Key Algorithm: %s\n", indent, d.PublicKeyAlgorithm)
	}

	if len(d.Chain) > 0 {
		fmt.Fprintf(&sb, "%s  Certificate Chain:\n", indent)
		for i, cert := range d.Chain {
			fmt.Fprintf(&sb, "%s    [%d] %s\n", indent, i, cert)
		}
	}

	return sb.String()
}
