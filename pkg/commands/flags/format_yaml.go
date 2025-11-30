package flags

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

	// name: (configurable, default cyan)
	r.writeField(prefix, "name", resp.Name, r.colors.Name)

	// kind: (configurable, default blue)
	r.writeField(prefix, "kind", resp.Kind, r.colors.Kind)

	// status: (configurable per status value)
	r.writeField(prefix, "status", resp.Status.String(), r.statusColor(resp.Status))

	// messages: (configurable, default dim)
	if len(resp.Messages) > 0 {
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

	// duration: (configurable, default magenta)
	if resp.Duration != nil {
		r.writeField(prefix, "duration", resp.Duration.AsDuration().String(), r.colors.Duration)
	}

	// components: (recursive)
	if len(resp.Components) > 0 {
		r.buf.WriteString(prefix)
		r.buf.WriteString("components:\n")
		for _, comp := range resp.Components {
			r.buf.WriteString(prefix)
			r.buf.WriteString("  - ")
			// First field of component (name) goes on same line as dash
			r.writeInlineField("name", comp.Name, r.colors.Name)
			// Rest of component fields
			r.renderComponentRest(comp, indent+2)
		}
	}
}

// renderComponentRest renders a component's fields after the first (name is already written)
func (r *yamlRenderer) renderComponentRest(resp *ph.HealthCheckResponse, indent int) {
	prefix := strings.Repeat("  ", indent)

	r.writeField(prefix, "kind", resp.Kind, r.colors.Kind)
	r.writeField(prefix, "status", resp.Status.String(), r.statusColor(resp.Status))

	if len(resp.Messages) > 0 {
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

	if resp.Duration != nil {
		r.writeField(prefix, "duration", resp.Duration.AsDuration().String(), r.colors.Duration)
	}

	if len(resp.Components) > 0 {
		r.buf.WriteString(prefix)
		r.buf.WriteString("components:\n")
		for _, comp := range resp.Components {
			r.buf.WriteString(prefix)
			r.buf.WriteString("  - ")
			r.writeInlineField("name", comp.Name, r.colors.Name)
			r.renderComponentRest(comp, indent+2)
		}
	}
}

// writeField writes a YAML field with optional colorization of the value.
// Empty values are skipped.
func (r *yamlRenderer) writeField(prefix, key, value, color string) {
	if value == "" {
		return
	}
	r.buf.WriteString(prefix)
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

// writeInlineField writes a field on the same line (after list dash)
func (r *yamlRenderer) writeInlineField(key, value, color string) {
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
