package migrate

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"

	"github.com/isometry/platform-health/pkg/phctx"
)

func TestTransformRest(t *testing.T) {
	tests := []struct {
		name     string
		input    map[string]any
		wantKeys []string
		noKeys   []string
	}{
		{
			name: "promotes request fields to top level",
			input: map[string]any{
				"request": map[string]any{
					"url":    "https://example.com",
					"method": "GET",
				},
			},
			wantKeys: []string{"url", "method"},
			noKeys:   []string{"request"},
		},
		{
			name:     "no request key is a no-op",
			input:    map[string]any{"url": "https://example.com"},
			wantKeys: []string{"url"},
		},
		{
			name:     "non-map request is a no-op",
			input:    map[string]any{"request": "not-a-map"},
			wantKeys: []string{"request"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transformRest(tt.input)
			for _, k := range tt.wantKeys {
				if _, ok := tt.input[k]; !ok {
					t.Errorf("expected key %q to be present", k)
				}
			}
			for _, k := range tt.noKeys {
				if _, ok := tt.input[k]; ok {
					t.Errorf("expected key %q to be absent", k)
				}
			}
		})
	}
}

func TestFrameworkKeys(t *testing.T) {
	expected := []string{"checks", "components", "timeout", "includes"}
	for _, key := range expected {
		if !frameworkKeys[key] {
			t.Errorf("expected %q to be a framework key", key)
		}
	}

	notFramework := []string{"type", "spec", "name", "url", "host", "port"}
	for _, key := range notFramework {
		if frameworkKeys[key] {
			t.Errorf("expected %q to NOT be a framework key", key)
		}
	}
}

func TestTypeRewrites(t *testing.T) {
	if newType, ok := typeRewrites["rest"]; !ok || newType != "http" {
		t.Errorf("expected rest -> http rewrite, got %q (ok=%v)", newType, ok)
	}
}

func TestTransformChecks(t *testing.T) {
	tests := []struct {
		name   string
		input  map[string]any
		wantCh string // expected "check" value in first entry
	}{
		{
			name: "rewrites expr to check",
			input: map[string]any{
				"checks": []any{
					map[string]any{"expr": "response.status == 200", "message": "bad status"},
				},
			},
			wantCh: "response.status == 200",
		},
		{
			name: "rewrites expression to check",
			input: map[string]any{
				"checks": []any{
					map[string]any{"expression": "response.status == 200", "message": "bad status"},
				},
			},
			wantCh: "response.status == 200",
		},
		{
			name: "leaves existing check key alone",
			input: map[string]any{
				"checks": []any{
					map[string]any{"check": "response.status == 200", "message": "ok"},
				},
			},
			wantCh: "response.status == 200",
		},
		{
			name:  "no checks key is a no-op",
			input: map[string]any{"url": "https://example.com"},
		},
		{
			name:  "non-slice checks is a no-op",
			input: map[string]any{"checks": "not-a-slice"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transformChecks(tt.input)
			if tt.wantCh == "" {
				return
			}
			checksSlice, ok := tt.input["checks"].([]any)
			if !ok {
				t.Fatal("checks is not []any after transform")
			}
			entry, ok := checksSlice[0].(map[string]any)
			if !ok {
				t.Fatal("first check entry is not map[string]any")
			}
			if got, ok := entry["check"].(string); !ok || got != tt.wantCh {
				t.Errorf("expected check=%q, got %q", tt.wantCh, got)
			}
			if _, ok := entry["expr"]; ok {
				t.Error("expected expr key to be removed")
			}
			if _, ok := entry["expression"]; ok {
				t.Error("expected expression key to be removed")
			}
		})
	}
}

func TestTransformHTTPStatus(t *testing.T) {
	tests := []struct {
		name      string
		input     map[string]any
		wantExpr  string
		wantNote  bool
		wantCount int // expected number of checks entries
	}{
		{
			name:      "single status value",
			input:     map[string]any{"status": []any{200}},
			wantExpr:  "response.status == 200",
			wantNote:  true,
			wantCount: 1,
		},
		{
			name:      "multiple status values",
			input:     map[string]any{"status": []any{200, 201}},
			wantExpr:  "response.status in [200, 201]",
			wantNote:  true,
			wantCount: 1,
		},
		{
			name: "merges with existing checks",
			input: map[string]any{
				"status": []any{200},
				"checks": []any{
					map[string]any{"check": "response.json.ok == true", "message": "not ok"},
				},
			},
			wantExpr:  "response.status == 200",
			wantNote:  true,
			wantCount: 2,
		},
		{
			name:  "no status key is a no-op",
			input: map[string]any{"url": "https://example.com"},
		},
		{
			name:  "non-slice status is a no-op",
			input: map[string]any{"status": "200"},
		},
		{
			name:  "empty status slice is a no-op",
			input: map[string]any{"status": []any{}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			note := transformHTTPStatus(tt.input)
			if tt.wantNote && note == "" {
				t.Error("expected a migration note, got empty")
			}
			if !tt.wantNote && note != "" {
				t.Errorf("expected no migration note, got %q", note)
			}
			if tt.wantExpr == "" {
				return
			}
			// status key should be removed
			if _, ok := tt.input["status"]; ok {
				t.Error("expected status key to be removed")
			}
			checksSlice, ok := tt.input["checks"].([]any)
			if !ok {
				t.Fatal("checks is not []any after transform")
			}
			if len(checksSlice) != tt.wantCount {
				t.Fatalf("expected %d checks entries, got %d", tt.wantCount, len(checksSlice))
			}
			// The generated check is always the last entry
			last, ok := checksSlice[len(checksSlice)-1].(map[string]any)
			if !ok {
				t.Fatal("last check entry is not map[string]any")
			}
			if got := last["check"]; got != tt.wantExpr {
				t.Errorf("expected check=%q, got %q", tt.wantExpr, got)
			}
		})
	}
}

func TestRunIntegration(t *testing.T) {
	const input = `http:
  - name: google
    url: https://google.com
    status: [200]
rest:
  - name: api
    request:
      url: https://api.example.com/health
    checks:
      - expr: "response.status == 200"
        message: "HTTP request failed"
      - expr: 'response.json.status == "ok"'
        message: "API unhealthy"
`

	// Write input to temp file
	tmpDir := t.TempDir()
	inputPath := filepath.Join(tmpDir, "old-config.yaml")
	if err := os.WriteFile(inputPath, []byte(input), 0644); err != nil {
		t.Fatal(err)
	}

	// Build command with context
	cmd := New()
	ctx := phctx.ContextWithViper(context.Background(), phctx.NewViper())
	cmd.SetContext(ctx)

	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{inputPath})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("run failed: %v\nstderr: %s", err, stderr.String())
	}

	// Parse output
	var result map[string]any
	if err := yaml.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("failed to parse output YAML: %v\noutput: %s", err, stdout.String())
	}

	components, ok := result["components"].(map[string]any)
	if !ok {
		t.Fatalf("expected components map, got %T", result["components"])
	}

	// Verify google component: status should be converted to check, no spec.status
	google, ok := components["google"].(map[string]any)
	if !ok {
		t.Fatal("missing google component")
	}
	if google["type"] != "http" {
		t.Errorf("expected google type=http, got %v", google["type"])
	}
	if spec, ok := google["spec"].(map[string]any); ok {
		if _, hasStatus := spec["status"]; hasStatus {
			t.Error("expected status to be removed from spec")
		}
	}
	googleChecks, ok := google["checks"].([]any)
	if !ok || len(googleChecks) == 0 {
		t.Fatal("expected google to have checks")
	}
	firstCheck, ok := googleChecks[0].(map[string]any)
	if !ok {
		t.Fatal("first google check is not a map")
	}
	if firstCheck["check"] != "response.status == 200" {
		t.Errorf("expected status CEL check, got %v", firstCheck["check"])
	}

	// Verify api component: expr should be rewritten to check
	api, ok := components["api"].(map[string]any)
	if !ok {
		t.Fatal("missing api component")
	}
	if api["type"] != "http" {
		t.Errorf("expected api type=http, got %v", api["type"])
	}
	apiChecks, ok := api["checks"].([]any)
	if !ok || len(apiChecks) != 2 {
		t.Fatalf("expected api to have 2 checks, got %v", apiChecks)
	}
	for i, c := range apiChecks {
		entry, ok := c.(map[string]any)
		if !ok {
			t.Fatalf("api check[%d] is not a map", i)
		}
		if _, hasExpr := entry["expr"]; hasExpr {
			t.Errorf("api check[%d] still has legacy 'expr' key", i)
		}
		if _, hasCheck := entry["check"]; !hasCheck {
			t.Errorf("api check[%d] missing 'check' key", i)
		}
	}

	// Verify stderr contains migration notes
	stderrStr := stderr.String()
	if !strings.Contains(stderrStr, "migration notes") {
		t.Errorf("expected migration notes in stderr, got: %s", stderrStr)
	}
}
