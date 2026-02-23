package output

import (
	"google.golang.org/protobuf/encoding/protojson"

	ph "github.com/isometry/platform-health/pkg/platform_health"
)

func init() {
	RegisterFormatter("json", &JSONFormatter{})
}

// JSONFormatter formats health check responses as JSON.
type JSONFormatter struct{}

// Format converts the health check response to JSON.
func (f *JSONFormatter) Format(status *ph.HealthCheckResponse, cfg Config) ([]byte, error) {
	opts := protojson.MarshalOptions{}
	if !cfg.Compact {
		opts.Multiline = true
		opts.Indent = "  "
	}
	return opts.Marshal(status)
}
