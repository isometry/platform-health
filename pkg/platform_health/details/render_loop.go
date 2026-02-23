package details

import (
	"fmt"
	"strings"
)

// Render implements the Renderer interface for loop detection details.
func (d *Detail_Loop) Render(indent string) string {
	var sb strings.Builder
	sb.WriteString(indent + "Loop Detection:\n")

	if len(d.ServerIds) > 0 {
		fmt.Fprintf(&sb, "%s  Server Chain: %s\n", indent, strings.Join(d.ServerIds, " -> "))
	}

	return sb.String()
}
