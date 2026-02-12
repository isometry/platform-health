package migrate

import (
	"testing"
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
