package cli

import (
	"bytes"
	"strings"

	"github.com/goccy/go-yaml"
	"google.golang.org/protobuf/encoding/protojson"

	ph "github.com/isometry/platform-health/pkg/platform_health"
)

func init() {
	RegisterFormatter("yaml", &YAMLFormatter{})
}

// YAMLFormatter formats health check responses as YAML.
type YAMLFormatter struct{}

// Format converts the health check response to YAML.
func (f *YAMLFormatter) Format(status *ph.HealthCheckResponse, cfg OutputConfig) ([]byte, error) {
	// Use semantic colorized renderer when colors are enabled
	if cfg.Colorize {
		r := &yamlRenderer{colorize: true, colors: cfg.Colors}
		return []byte(strings.TrimRight(r.render(status, 0), "\n")), nil
	}

	// Plain YAML: use standard marshaling
	jsonBytes, err := protojson.Marshal(status)
	if err != nil {
		return nil, err
	}

	var data any
	if err := yaml.UnmarshalWithOptions(jsonBytes, &data, yaml.UseOrderedMap()); err != nil {
		return nil, err
	}

	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf, yaml.Indent(2), yaml.IndentSequence(true))
	if err := enc.Encode(data); err != nil {
		return nil, err
	}

	return bytes.TrimRight(buf.Bytes(), "\n"), nil
}

// yamlRenderer renders HealthCheckResponse to YAML with semantic colorization.
// It walks the response structure directly to apply field-specific colors.
type yamlRenderer struct {
	buf      strings.Builder
	colorize bool
	colors   ResolvedColors
}

// statusColor returns the appropriate color for a status value.
func (r *yamlRenderer) statusColor(s ph.Status) string {
	switch s {
	case ph.Status_HEALTHY:
		return r.colors.StatusHealthy
	case ph.Status_UNHEALTHY:
		return r.colors.StatusUnhealthy
	case ph.Status_LOOP_DETECTED:
		return r.colors.StatusLoop
	default: // UNKNOWN and others
		return r.colors.StatusUnknown
	}
}

func (r *yamlRenderer) render(resp *ph.HealthCheckResponse, indent int) string {
	r.buf.Reset()
	r.renderResponse(resp, indent)
	return r.buf.String()
}

func (r *yamlRenderer) renderResponse(resp *ph.HealthCheckResponse, indent int) {
	prefix := strings.Repeat("  ", indent)

	r.writeField(prefix, "name", resp.Name, r.colors.Name, false)
	r.renderFields(resp, prefix, indent)
}

// renderFields renders common fields (type, status, messages, duration, components)
func (r *yamlRenderer) renderFields(resp *ph.HealthCheckResponse, prefix string, indent int) {
	r.writeField(prefix, "type", resp.Type, r.colors.Type, false)
	r.writeField(prefix, "status", resp.Status.String(), r.statusColor(resp.Status), false)
	r.renderMessages(resp, prefix)

	if resp.Duration != nil {
		r.writeField(prefix, "duration", resp.Duration.AsDuration().String(), r.colors.Duration, false)
	}

	r.renderComponents(resp, prefix, indent)
}

// renderMessages renders the messages list
func (r *yamlRenderer) renderMessages(resp *ph.HealthCheckResponse, prefix string) {
	if len(resp.Messages) == 0 {
		return
	}
	r.buf.WriteString(prefix)
	r.buf.WriteString("messages:\n")
	for _, msg := range resp.Messages {
		r.buf.WriteString(prefix)
		r.buf.WriteString("  - ")
		if r.colorize {
			r.buf.WriteString(r.colors.Message)
			r.buf.WriteString(msg)
			r.buf.WriteString(r.colors.Reset)
		} else {
			r.buf.WriteString(msg)
		}
		r.buf.WriteString("\n")
	}
}

// renderComponents renders nested components recursively
func (r *yamlRenderer) renderComponents(resp *ph.HealthCheckResponse, prefix string, indent int) {
	if len(resp.Components) == 0 {
		return
	}
	r.buf.WriteString(prefix)
	r.buf.WriteString("components:\n")
	for _, comp := range resp.Components {
		r.buf.WriteString(prefix)
		r.buf.WriteString("  - ")
		r.writeField("", "name", comp.Name, r.colors.Name, true)
		// Render remaining fields at increased indent
		childPrefix := strings.Repeat("  ", indent+2)
		r.renderFields(comp, childPrefix, indent+2)
	}
}

// writeField writes a YAML field with optional colorization.
// If inline is true, omits the prefix (used after list dash).
// Empty values are skipped unless inline.
func (r *yamlRenderer) writeField(prefix, key, value, color string, inline bool) {
	if value == "" && !inline {
		return
	}
	if !inline {
		r.buf.WriteString(prefix)
	}
	r.buf.WriteString(key)
	r.buf.WriteString(": ")
	if r.colorize && color != "" {
		r.buf.WriteString(color)
		r.buf.WriteString(value)
		r.buf.WriteString(r.colors.Reset)
	} else {
		r.buf.WriteString(value)
	}
	r.buf.WriteString("\n")
}
