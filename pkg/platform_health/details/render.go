package details

import (
	"fmt"
	"strings"
	"time"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/anypb"
)

// Renderer is implemented by detail types that can render to human-readable text.
type Renderer interface {
	Render(indent string) string
}

// RenderAny renders a single protobuf Any detail to human-friendly text.
// Falls back to JSON for unknown types.
func RenderAny(detail *anypb.Any, indent string) string {
	if detail == nil {
		return ""
	}

	typeURL := detail.GetTypeUrl()

	switch {
	case strings.HasSuffix(typeURL, "Detail_TLS"):
		var tls Detail_TLS
		if err := detail.UnmarshalTo(&tls); err != nil {
			return renderUnknown(detail, indent)
		}
		return tls.Render(indent)

	case strings.HasSuffix(typeURL, "Detail_KStatus"):
		var kstatus Detail_KStatus
		if err := detail.UnmarshalTo(&kstatus); err != nil {
			return renderUnknown(detail, indent)
		}
		return kstatus.Render(indent)

	case strings.HasSuffix(typeURL, "Detail_Loop"):
		var loop Detail_Loop
		if err := detail.UnmarshalTo(&loop); err != nil {
			return renderUnknown(detail, indent)
		}
		return loop.Render(indent)

	default:
		return renderUnknown(detail, indent)
	}
}

// RenderAll renders all details to human-friendly text.
func RenderAll(detailList []*anypb.Any, indent string) string {
	if len(detailList) == 0 {
		return ""
	}

	var sb strings.Builder
	for i, d := range detailList {
		if i > 0 {
			sb.WriteString("\n")
		}
		sb.WriteString(RenderAny(d, indent))
	}
	return sb.String()
}

func renderUnknown(detail *anypb.Any, indent string) string {
	typeURL := detail.GetTypeUrl()
	typeName := typeURL
	if idx := strings.LastIndex(typeURL, "/"); idx >= 0 {
		typeName = typeURL[idx+1:]
	}

	jsonBytes, err := protojson.MarshalOptions{Multiline: true, Indent: "  "}.Marshal(detail)
	if err != nil {
		return fmt.Sprintf("%sDetail (%s): [unmarshal error]\n", indent, typeName)
	}

	lines := strings.Split(string(jsonBytes), "\n")
	for i, line := range lines {
		lines[i] = indent + "  " + line
	}

	return fmt.Sprintf("%sDetail (%s):\n%s\n", indent, typeName, strings.Join(lines, "\n"))
}

// FormatDuration formats a duration for certificate expiry display.
func FormatDuration(d time.Duration) string {
	if d < 0 {
		return "expired"
	}
	days := int(d.Hours() / 24)
	if days > 30 {
		return fmt.Sprintf("%dd remaining", days)
	}
	if days > 0 {
		return fmt.Sprintf("%dd %dh remaining", days, int(d.Hours())%24)
	}
	if d.Hours() >= 1 {
		return fmt.Sprintf("%dh %dm remaining", int(d.Hours()), int(d.Minutes())%60)
	}
	if d.Minutes() >= 1 {
		return fmt.Sprintf("%dm remaining", int(d.Minutes()))
	}
	return fmt.Sprintf("%ds remaining", int(d.Seconds()))
}
