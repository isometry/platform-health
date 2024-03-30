package kubernetes_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"

	"github.com/isometry/platform-health/pkg/provider/kubernetes"
)

func TestNewResource(t *testing.T) {
	type testCase struct {
		name    string
		obj     any
		wantErr bool
	}

	testCases := []testCase{
		{
			name: "Valid resource with conditions",
			obj: map[string]any{
				"apiVersion": "v1",
				"kind":       "Pod",
				"metadata": map[string]any{
					"name":      "test-pod",
					"namespace": "default",
				},
				"status": map[string]any{
					"conditions": []map[string]any{
						{
							"type":    "Ready",
							"status":  v1.ConditionTrue,
							"message": "Pod is ready",
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "Valid resource without conditions",
			obj: map[string]any{
				"apiVersion": "v1",
				"kind":       "Pod",
				"metadata": map[string]any{
					"name":      "test-pod",
					"namespace": "default",
				},
				"status": map[string]any{},
			},
			wantErr: false,
		},
		{
			name: "Valid resource without status",
			obj: map[string]any{
				"apiVersion": "v1",
				"kind":       "Pod",
				"metadata": map[string]any{
					"name":      "test-pod",
					"namespace": "default",
				},
			},
			wantErr: false,
		},
		{
			name:    "Nil resource",
			obj:     nil,
			wantErr: true,
		},
		{
			name:    "Invalid resource",
			obj:     "invalid",
			wantErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := kubernetes.NewResource(tc.obj)
			if tc.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
