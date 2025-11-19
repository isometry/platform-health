package checks

import (
	"testing"

	"github.com/google/cel-go/cel"
	"github.com/stretchr/testify/assert"
)

func TestValidateExpression(t *testing.T) {
	tests := []struct {
		name        string
		expression  string
		expectError bool
		errorMsg    string
	}{
		{
			name:        "valid boolean expression",
			expression:  `response.body.status == "healthy"`,
			expectError: false,
		},
		{
			name:        "valid comparison with number",
			expression:  `response.status == 200`,
			expectError: false,
		},
		{
			name:        "valid nested field access",
			expression:  `response.body.data.database.connected == true`,
			expectError: false,
		},
		{
			name:        "valid logical AND",
			expression:  `response.body.status == "ok" && response.status == 200`,
			expectError: false,
		},
		{
			name:        "valid logical OR",
			expression:  `response.status == 200 || response.status == 201`,
			expectError: false,
		},
		{
			name:        "valid array size check",
			expression:  `size(response.body.items) > 0`,
			expectError: false,
		},
		{
			name:        "valid string contains",
			expression:  `response.bodyText.contains("healthy")`,
			expectError: false,
		},
		{
			name:        "invalid syntax",
			expression:  `invalid syntax here!!!`,
			expectError: true,
		},
		{
			name:        "non-boolean return type",
			expression:  `response.status`,
			expectError: true,
			errorMsg:    "must return boolean",
		},
		{
			name:        "string instead of boolean",
			expression:  `"hello world"`,
			expectError: true,
			errorMsg:    "must return boolean",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateExpression(tt.expression,
				cel.Variable("response", cel.MapType(cel.StringType, cel.DynType)),
			)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestEvaluatorEvaluation(t *testing.T) {
	tests := []struct {
		name        string
		expression  string
		context     map[string]any
		expected    bool
		expectError bool
	}{
		{
			name:       "simple equality check - true",
			expression: `response.body.status == "healthy"`,
			context: map[string]any{
				"response": map[string]any{
					"body": map[string]any{
						"status": "healthy",
					},
				},
			},
			expected:    true,
			expectError: false,
		},
		{
			name:       "simple equality check - false",
			expression: `response.body.status == "healthy"`,
			context: map[string]any{
				"response": map[string]any{
					"body": map[string]any{
						"status": "unhealthy",
					},
				},
			},
			expected:    false,
			expectError: false,
		},
		{
			name:       "numeric comparison",
			expression: `response.status >= 200 && response.status < 300`,
			context: map[string]any{
				"response": map[string]any{
					"status": 200,
				},
			},
			expected:    true,
			expectError: false,
		},
		{
			name:       "nested field access",
			expression: `response.body.data.value > 100`,
			context: map[string]any{
				"response": map[string]any{
					"body": map[string]any{
						"data": map[string]any{
							"value": 150,
						},
					},
				},
			},
			expected:    true,
			expectError: false,
		},
		{
			name:       "string contains",
			expression: `response.bodyText.contains("success")`,
			context: map[string]any{
				"response": map[string]any{
					"bodyText": "operation completed successfully",
				},
			},
			expected:    true,
			expectError: false,
		},
		{
			name:       "logical OR",
			expression: `response.status == 200 || response.status == 201`,
			context: map[string]any{
				"response": map[string]any{
					"status": 201,
				},
			},
			expected:    true,
			expectError: false,
		},
		{
			name:       "header check",
			expression: `response.headers["Content-Type"] == "application/json"`,
			context: map[string]any{
				"response": map[string]any{
					"headers": map[string]string{
						"Content-Type": "application/json",
					},
				},
			},
			expected:    true,
			expectError: false,
		},
	}

	// Create CEL config with test variables
	celConfig := NewCEL(
		cel.Variable("request", cel.MapType(cel.StringType, cel.DynType)),
		cel.Variable("response", cel.MapType(cel.StringType, cel.DynType)),
	)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create evaluator with the test expression
			evaluator, err := celConfig.NewEvaluator(
				[]Expression{{Expression: tt.expression}},
			)
			assert.NoError(t, err)

			// Evaluate using the shared evaluator
			err = evaluator.Evaluate(tt.context)
			if tt.expectError {
				assert.Error(t, err)
			} else {
				if tt.expected {
					assert.NoError(t, err)
				} else {
					assert.Error(t, err)
				}
			}
		})
	}
}
