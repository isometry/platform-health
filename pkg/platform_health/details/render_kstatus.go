package details

import (
	"fmt"
	"strings"
)

// Render implements the Renderer interface for Kubernetes status details.
func (d *Detail_KStatus) Render(indent string) string {
	var sb strings.Builder
	sb.WriteString(indent + "Kubernetes Status:\n")

	if d.Status != "" {
		fmt.Fprintf(&sb, "%s  Status: %s\n", indent, d.Status)
	}

	if d.Message != "" {
		fmt.Fprintf(&sb, "%s  Message: %s\n", indent, d.Message)
	}

	if len(d.Conditions) > 0 {
		fmt.Fprintf(&sb, "%s  Conditions:\n", indent)
		for _, cond := range d.Conditions {
			line := fmt.Sprintf("%s=%s", cond.Type, cond.Status)
			if cond.Reason != "" {
				line += fmt.Sprintf(" (%s)", cond.Reason)
			}
			if cond.Message != "" {
				line += fmt.Sprintf(": %s", cond.Message)
			}
			fmt.Fprintf(&sb, "%s    - %s\n", indent, line)
		}
	}

	return sb.String()
}
