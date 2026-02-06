package output

import (
	"strings"
	"testing"

	ph "github.com/isometry/platform-health/pkg/platform_health"
)

func TestYAMLFormatter_Healthy(t *testing.T) {
	resp := &ph.HealthCheckResponse{
		Name:   "test",
		Type:   "http",
		Status: ph.Status_HEALTHY,
	}

	formatter, _ := GetFormatter("yaml")
	output, err := formatter.Format(resp, Config{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Check YAML contains expected fields
	outputStr := string(output)
	if !strings.Contains(outputStr, "name: test") {
		t.Errorf("expected 'name: test' in YAML output: %s", outputStr)
	}
	if !strings.Contains(outputStr, "type: http") {
		t.Errorf("expected 'type: http' in YAML output: %s", outputStr)
	}
	if !strings.Contains(outputStr, "status: HEALTHY") {
		t.Errorf("expected 'status: HEALTHY' in YAML output: %s", outputStr)
	}
}

func TestYAMLFormatter_Unhealthy(t *testing.T) {
	resp := &ph.HealthCheckResponse{
		Name:     "test",
		Type:     "tcp",
		Status:   ph.Status_UNHEALTHY,
		Messages: []string{"connection refused"},
	}

	formatter, _ := GetFormatter("yaml")
	output, err := formatter.Format(resp, Config{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	outputStr := string(output)
	if !strings.Contains(outputStr, "status: UNHEALTHY") {
		t.Errorf("expected 'status: UNHEALTHY' in YAML output: %s", outputStr)
	}
	// In plain (non-colorized) mode, protojson uses the proto field name 'messages' with a list
	if !strings.Contains(outputStr, "messages:") || !strings.Contains(outputStr, "connection refused") {
		t.Errorf("expected messages in YAML output: %s", outputStr)
	}
}

func TestYAMLFormatter_Nested(t *testing.T) {
	resp := &ph.HealthCheckResponse{
		Name:   "root",
		Type:   "system",
		Status: ph.Status_HEALTHY,
		Components: []*ph.HealthCheckResponse{
			{
				Name:   "child",
				Type:   "http",
				Status: ph.Status_HEALTHY,
			},
		},
	}

	formatter, _ := GetFormatter("yaml")
	output, err := formatter.Format(resp, Config{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	outputStr := string(output)
	if !strings.Contains(outputStr, "components:") {
		t.Errorf("expected 'components:' in YAML output: %s", outputStr)
	}
	if !strings.Contains(outputStr, "name: child") {
		t.Errorf("expected nested 'name: child' in YAML output: %s", outputStr)
	}
}

func TestYAMLFormatter_Colorized(t *testing.T) {
	resp := &ph.HealthCheckResponse{
		Name:   "test",
		Type:   "http",
		Status: ph.Status_HEALTHY,
	}

	formatter, _ := GetFormatter("yaml")
	colors := DefaultColorConfig().Resolve()

	// Test without colorization
	plainOutput, err := formatter.Format(resp, Config{Colorize: false})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(string(plainOutput), "\x1b[") {
		t.Error("expected no ANSI escape codes in plain output")
	}

	// Test with colorization
	colorOutput, err := formatter.Format(resp, Config{Colorize: true, Colors: colors})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(string(colorOutput), "\x1b[") {
		t.Error("expected ANSI escape codes in colorized output")
	}

	// Verify colorized output still contains the data
	if !strings.Contains(string(colorOutput), "name") {
		t.Error("expected 'name' in colorized output")
	}
	if !strings.Contains(string(colorOutput), "HEALTHY") {
		t.Error("expected 'HEALTHY' in colorized output")
	}
}

func TestYAMLFormatter_ColorizedMessages(t *testing.T) {
	tests := []struct {
		name     string
		messages []string
	}{
		{"single_message", []string{"connection refused"}},
		{"multiple_messages", []string{"error one", "error two"}},
	}

	formatter, _ := GetFormatter("yaml")
	colors := DefaultColorConfig().Resolve()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := &ph.HealthCheckResponse{
				Name:     "test",
				Type:     "tcp",
				Status:   ph.Status_UNHEALTHY,
				Messages: tt.messages,
			}

			output, err := formatter.Format(resp, Config{Colorize: true, Colors: colors})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			outputStr := string(output)

			// Should always use "messages:" (list format), never "message:" (singular)
			if strings.Contains(outputStr, "message:") && !strings.Contains(outputStr, "messages:") {
				t.Errorf("expected 'messages:' (list format), got singular 'message:': %s", outputStr)
			}
			if !strings.Contains(outputStr, "messages:") {
				t.Errorf("expected 'messages:' in output: %s", outputStr)
			}

			// Verify all messages are present
			for _, msg := range tt.messages {
				if !strings.Contains(outputStr, msg) {
					t.Errorf("expected message %q in output: %s", msg, outputStr)
				}
			}

			// Verify list format (messages prefixed with "- ")
			if !strings.Contains(outputStr, "- ") {
				t.Errorf("expected list format with '- ' prefix: %s", outputStr)
			}
		})
	}
}

func TestYAMLFormatter_SemanticColors(t *testing.T) {
	colors := DefaultColorConfig().Resolve()

	tests := []struct {
		name          string
		status        ph.Status
		expectedColor string
		colorName     string
	}{
		{"healthy_green", ph.Status_HEALTHY, colors.StatusHealthy, "green"},
		{"unhealthy_red", ph.Status_UNHEALTHY, colors.StatusUnhealthy, "red"},
		{"unknown_yellow", ph.Status_UNKNOWN, colors.StatusUnknown, "yellow"},
		{"loop_yellow", ph.Status_LOOP_DETECTED, colors.StatusLoop, "yellow"},
	}

	formatter, _ := GetFormatter("yaml")

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := &ph.HealthCheckResponse{
				Name:   "test",
				Type:   "http",
				Status: tt.status,
			}

			output, err := formatter.Format(resp, Config{Colorize: true, Colors: colors})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			outputStr := string(output)

			// Verify status is colored with the correct semantic color
			expectedColoredStatus := tt.expectedColor + tt.status.String() + colors.Reset
			if !strings.Contains(outputStr, expectedColoredStatus) {
				t.Errorf("expected %s status to be %s colored, got: %s",
					tt.status.String(), tt.colorName, outputStr)
			}
		})
	}
}
