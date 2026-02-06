package output

import (
	"strings"
	"testing"

	ph "github.com/isometry/platform-health/pkg/platform_health"
)

func TestJSONFormatter_Healthy(t *testing.T) {
	resp := &ph.HealthCheckResponse{
		Name:   "test",
		Type:   "http",
		Status: ph.Status_HEALTHY,
	}

	formatter, _ := GetFormatter("json")
	output, err := formatter.Format(resp, Config{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(string(output), `"status"`) || !strings.Contains(string(output), `"HEALTHY"`) {
		t.Errorf("expected HEALTHY status in output: %s", output)
	}
}
