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

func TestDropKubernetesConditions(t *testing.T) {
	tests := []struct {
		name     string
		input    map[string]any
		wantNote bool
		noKeys   []string
	}{
		{
			name: "drops flat conditionType and conditionStatus",
			input: map[string]any{
				"conditionType":   "Available",
				"conditionStatus": "True",
			},
			wantNote: true,
			noKeys:   []string{"conditionType", "conditionStatus"},
		},
		{
			name: "drops condition sub-map",
			input: map[string]any{
				"condition": map[string]any{
					"type":   "Ready",
					"status": "True",
				},
			},
			wantNote: true,
			noKeys:   []string{"condition"},
		},
		{
			name:  "no condition fields is a no-op",
			input: map[string]any{"kind": "deployment"},
		},
		{
			name: "drops conditionType even without conditionStatus",
			input: map[string]any{
				"conditionType": "Available",
			},
			wantNote: true,
			noKeys:   []string{"conditionType"},
		},
		{
			name:  "non-map condition value is a no-op",
			input: map[string]any{"condition": "not-a-map"},
		},
		{
			name: "does not modify existing checks",
			input: map[string]any{
				"conditionType":   "Available",
				"conditionStatus": "True",
				"checks": []any{
					map[string]any{"check": "resource.metadata.name != \"\"", "message": "no name"},
				},
			},
			wantNote: true,
			noKeys:   []string{"conditionType", "conditionStatus"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Snapshot existing checks count before
			var checksBefore int
			if cs, ok := tt.input["checks"].([]any); ok {
				checksBefore = len(cs)
			}

			note := dropKubernetesConditions(tt.input)
			if tt.wantNote && note == "" {
				t.Error("expected a migration note, got empty")
			}
			if !tt.wantNote && note != "" {
				t.Errorf("expected no migration note, got %q", note)
			}
			for _, k := range tt.noKeys {
				if _, ok := tt.input[k]; ok {
					t.Errorf("expected key %q to be removed", k)
				}
			}
			// Verify no checks were added
			if cs, ok := tt.input["checks"].([]any); ok {
				if len(cs) != checksBefore {
					t.Errorf("expected checks count to remain %d, got %d", checksBefore, len(cs))
				}
			}
		})
	}
}

func TestRunIntegrationKubernetes(t *testing.T) {
	const input = `kubernetes:
  - name: my-deploy
    kind: deployment
    namespace: default
    conditionType: Available
    conditionStatus: "True"
  - name: my-pod
    kind: pod
    namespace: kube-system
    condition:
      type: Ready
      status: "True"
`

	tmpDir := t.TempDir()
	inputPath := filepath.Join(tmpDir, "k8s-config.yaml")
	if err := os.WriteFile(inputPath, []byte(input), 0644); err != nil {
		t.Fatal(err)
	}

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

	var result map[string]any
	if err := yaml.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("failed to parse output YAML: %v\noutput: %s", err, stdout.String())
	}

	components, ok := result["components"].(map[string]any)
	if !ok {
		t.Fatalf("expected components map, got %T", result["components"])
	}

	// Verify flat conditionType/conditionStatus with spec.name preserved
	deploy, ok := components["my-deploy"].(map[string]any)
	if !ok {
		t.Fatal("missing my-deploy component")
	}
	if deploy["type"] != "kubernetes" {
		t.Errorf("expected my-deploy type=kubernetes, got %v", deploy["type"])
	}
	deploySpec, ok := deploy["spec"].(map[string]any)
	if !ok {
		t.Fatal("missing my-deploy spec")
	}
	if deploySpec["name"] != "my-deploy" {
		t.Errorf("expected spec.name=my-deploy, got %v", deploySpec["name"])
	}
	// condition fields should NOT be in spec
	if _, ok := deploySpec["conditionType"]; ok {
		t.Error("conditionType should not be in spec")
	}
	if _, ok := deploySpec["conditionStatus"]; ok {
		t.Error("conditionStatus should not be in spec")
	}
	// No CEL checks should be generated from conditions (kstatus covers them)
	if _, ok := deploy["checks"]; ok {
		t.Error("expected no checks to be generated from conditions")
	}

	// Verify condition sub-map format with spec.name preserved
	pod, ok := components["my-pod"].(map[string]any)
	if !ok {
		t.Fatal("missing my-pod component")
	}
	if pod["type"] != "kubernetes" {
		t.Errorf("expected my-pod type=kubernetes, got %v", pod["type"])
	}
	podSpec, ok := pod["spec"].(map[string]any)
	if !ok {
		t.Fatal("missing my-pod spec")
	}
	if podSpec["name"] != "my-pod" {
		t.Errorf("expected spec.name=my-pod, got %v", podSpec["name"])
	}
	if _, ok := podSpec["condition"]; ok {
		t.Error("condition sub-map should not be in spec")
	}
	// No CEL checks should be generated from conditions (kstatus covers them)
	if _, ok := pod["checks"]; ok {
		t.Error("expected no checks to be generated from conditions")
	}

	// Verify migration notes in stderr
	stderrStr := stderr.String()
	if !strings.Contains(stderrStr, "migration notes") {
		t.Errorf("expected migration notes in stderr, got: %s", stderrStr)
	}
}

func TestRunIntegrationHelm(t *testing.T) {
	const input = `helm:
  - name: cilium
    namespace: kube-system
`

	tmpDir := t.TempDir()
	inputPath := filepath.Join(tmpDir, "helm-config.yaml")
	if err := os.WriteFile(inputPath, []byte(input), 0644); err != nil {
		t.Fatal(err)
	}

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

	var result map[string]any
	if err := yaml.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("failed to parse output YAML: %v\noutput: %s", err, stdout.String())
	}

	components, ok := result["components"].(map[string]any)
	if !ok {
		t.Fatalf("expected components map, got %T", result["components"])
	}

	cilium, ok := components["cilium"].(map[string]any)
	if !ok {
		t.Fatal("missing cilium component")
	}
	if cilium["type"] != "helm" {
		t.Errorf("expected cilium type=helm, got %v", cilium["type"])
	}
	ciliumSpec, ok := cilium["spec"].(map[string]any)
	if !ok {
		t.Fatal("missing cilium spec")
	}
	if ciliumSpec["release"] != "cilium" {
		t.Errorf("expected spec.release=cilium, got %v", ciliumSpec["release"])
	}
	if _, ok := ciliumSpec["name"]; ok {
		t.Error("spec should not contain name for helm provider")
	}
}
