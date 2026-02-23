package checks

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/cel-go/cel"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/isometry/platform-health/internal/testutil"
)

// getTestdataPath returns the path to the testdata directory
func getTestdataPath(t *testing.T) string {
	t.Helper()
	return testutil.TestdataPath(t)
}

// loadContextFixture loads a context JSON fixture
func loadContextFixture(t *testing.T, filename string) map[string]any {
	t.Helper()
	path := filepath.Join(getTestdataPath(t), filename)
	data, err := os.ReadFile(path)
	require.NoError(t, err, "failed to read fixture %s", filename)

	var result map[string]any
	require.NoError(t, json.Unmarshal(data, &result), "failed to unmarshal fixture")
	return result
}

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

func TestCheckEvaluation(t *testing.T) {
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
			// Compile the expression into a Check
			check, err := celConfig.Compile(Expression{Expression: tt.expression})
			assert.NoError(t, err)

			// Evaluate using the Check
			msg, err := check.Evaluate(tt.context)
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				if tt.expected {
					assert.Empty(t, msg, "expected check to pass")
				} else {
					assert.NotEmpty(t, msg, "expected check to fail")
				}
			}
		})
	}
}

// Fixture-based tests

func TestParseConfigMode(t *testing.T) {
	tests := []struct {
		name        string
		input       []any
		expected    []Expression
		expectError bool
		errorMsg    string
	}{
		{
			name: "simple string expression defaults to empty mode",
			input: []any{
				"items.size() > 0",
			},
			expected: []Expression{
				{Expression: "items.size() > 0", Mode: ""},
			},
		},
		{
			name: "map expression without mode defaults to empty",
			input: []any{
				map[string]any{
					"check":   "resource.status.phase == 'Running'",
					"message": "Pod must be running",
				},
			},
			expected: []Expression{
				{Expression: "resource.status.phase == 'Running'", Message: "Pod must be running", Mode: ""},
			},
		},
		{
			name: "map expression with mode: each",
			input: []any{
				map[string]any{
					"check":   "resource.status.phase == 'Running'",
					"message": "Pod must be running",
					"mode":    "each",
				},
			},
			expected: []Expression{
				{Expression: "resource.status.phase == 'Running'", Message: "Pod must be running", Mode: "each"},
			},
		},
		{
			name: "mixed expressions with different modes",
			input: []any{
				"items.size() >= 3",
				map[string]any{
					"check": "resource.status.phase == 'Running'",
					"mode":  "each",
				},
				map[string]any{
					"check":   "items.all(i, has(i.metadata.labels))",
					"message": "All items must have labels",
				},
			},
			expected: []Expression{
				{Expression: "items.size() >= 3", Mode: ""},
				{Expression: "resource.status.phase == 'Running'", Mode: "each"},
				{Expression: "items.all(i, has(i.metadata.labels))", Message: "All items must have labels", Mode: ""},
			},
		},
		{
			name: "invalid mode value",
			input: []any{
				map[string]any{
					"check": "resource.status.phase == 'Running'",
					"mode":  "invalid",
				},
			},
			expectError: true,
			errorMsg:    "invalid mode",
		},
		{
			name: "explicit empty mode is valid",
			input: []any{
				map[string]any{
					"check": "items.size() > 0",
					"mode":  "",
				},
			},
			expected: []Expression{
				{Expression: "items.size() > 0", Mode: ""},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseConfig(tt.input)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestExpressionEach(t *testing.T) {
	tests := []struct {
		name     string
		expr     Expression
		expected bool
	}{
		{
			name:     "empty mode returns false",
			expr:     Expression{Expression: "items.size() > 0", Mode: ""},
			expected: false,
		},
		{
			name:     "mode: each returns true",
			expr:     Expression{Expression: "resource.status.phase == 'Running'", Mode: "each"},
			expected: true,
		},
		{
			name:     "default expression (no mode set) returns false",
			expr:     Expression{Expression: "items.size() > 0"},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.expr.Each())
		})
	}
}

func TestCheckFixtures(t *testing.T) {
	// Create CEL config with test variables
	celConfig := NewCEL(
		cel.Variable("request", cel.MapType(cel.StringType, cel.DynType)),
		cel.Variable("response", cel.MapType(cel.StringType, cel.DynType)),
	)

	tests := []struct {
		name     string
		fixture  string
		expr     string
		expected bool
	}{
		{
			name:     "healthy status check",
			fixture:  "context-healthy.json",
			expr:     `response.body.status == "healthy"`,
			expected: true,
		},
		{
			name:     "healthy status code",
			fixture:  "context-healthy.json",
			expr:     `response.status >= 200 && response.status < 300`,
			expected: true,
		},
		{
			name:     "healthy nested data",
			fixture:  "context-healthy.json",
			expr:     `response.body.data.value > 100`,
			expected: true,
		},
		{
			name:     "healthy body contains",
			fixture:  "context-healthy.json",
			expr:     `response.bodyText.contains("success")`,
			expected: true,
		},
		{
			name:     "healthy content type header",
			fixture:  "context-healthy.json",
			expr:     `response.headers["Content-Type"] == "application/json"`,
			expected: true,
		},
		{
			name:     "unhealthy status check",
			fixture:  "context-unhealthy.json",
			expr:     `response.body.status == "healthy"`,
			expected: false,
		},
		{
			name:     "unhealthy status code",
			fixture:  "context-unhealthy.json",
			expr:     `response.status >= 200 && response.status < 300`,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			context := loadContextFixture(t, tt.fixture)

			check, err := celConfig.Compile(Expression{Expression: tt.expr})
			require.NoError(t, err)

			msg, err := check.Evaluate(context)
			require.NoError(t, err)
			if tt.expected {
				assert.Empty(t, msg, "expected check to pass")
			} else {
				assert.NotEmpty(t, msg, "expected check to fail")
			}
		})
	}
}

func TestNormalizeValue(t *testing.T) {
	tests := []struct {
		name     string
		input    any
		expected any
	}{
		{
			name:     "nil",
			input:    nil,
			expected: nil,
		},
		{
			name:     "string scalar",
			input:    "hello",
			expected: "hello",
		},
		{
			name:     "int scalar",
			input:    42,
			expected: 42,
		},
		{
			name:     "bool scalar",
			input:    true,
			expected: true,
		},
		{
			name:     "empty map[any]any",
			input:    map[any]any{},
			expected: map[string]any{},
		},
		{
			name:     "map[any]any with string keys",
			input:    map[any]any{"a": 1, "b": 2},
			expected: map[string]any{"a": 1, "b": 2},
		},
		{
			name:     "map[any]any with non-string keys",
			input:    map[any]any{1: "one", true: "yes"},
			expected: map[string]any{"1": "one", "true": "yes"},
		},
		{
			name:  "map[string]any with nested map[any]any",
			input: map[string]any{"inner": map[any]any{"k": "v"}},
			expected: map[string]any{"inner": map[string]any{"k": "v"}},
		},
		{
			name:  "deeply nested: map[any]any -> []any -> map[any]any",
			input: map[any]any{"list": []any{map[any]any{"nested": true}}},
			expected: map[string]any{"list": []any{map[string]any{"nested": true}}},
		},
		{
			name:     "[]any with map[any]any elements",
			input:    []any{map[any]any{"a": 1}, "plain", map[any]any{"b": 2}},
			expected: []any{map[string]any{"a": 1}, "plain", map[string]any{"b": 2}},
		},
		{
			name:     "[]any without maps unchanged",
			input:    []any{1, "two", true},
			expected: []any{1, "two", true},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := normalizeValue(tt.input)
			assert.Equal(t, tt.expected, result)

			// All results must be JSON-serializable
			_, err := json.Marshal(result)
			assert.NoError(t, err, "normalized value should be JSON-serializable")
		})
	}
}

func TestEvaluateAnyMapNormalization(t *testing.T) {
	celConfig := NewCEL(
		cel.Variable("response", cel.MapType(cel.StringType, cel.DynType)),
	)

	ctx := map[string]any{
		"response": map[string]any{
			"status": int64(200),
			"body":   "ok",
		},
	}

	tests := []struct {
		name  string
		expr  string
		check func(t *testing.T, result any)
	}{
		{
			name: "map literal returns map[string]any",
			expr: `{"ok": response.status == 200}`,
			check: func(t *testing.T, result any) {
				m, ok := result.(map[string]any)
				require.True(t, ok, "expected map[string]any, got %T", result)
				assert.Equal(t, true, m["ok"])
			},
		},
		{
			name: "nested map literal is normalized",
			expr: `{"outer": {"inner": response.status}}`,
			check: func(t *testing.T, result any) {
				m, ok := result.(map[string]any)
				require.True(t, ok, "expected map[string]any, got %T", result)
				inner, ok := m["outer"].(map[string]any)
				require.True(t, ok, "expected nested map[string]any, got %T", m["outer"])
				assert.Equal(t, int64(200), inner["inner"])
			},
		},
		{
			name: "map inside list is normalized",
			expr: `[{"status": response.status}]`,
			check: func(t *testing.T, result any) {
				list, ok := result.([]any)
				require.True(t, ok, "expected []any, got %T", result)
				require.Len(t, list, 1)
				m, ok := list[0].(map[string]any)
				require.True(t, ok, "expected map[string]any in list, got %T", list[0])
				assert.Equal(t, int64(200), m["status"])
			},
		},
		{
			name: "scalar result unchanged",
			expr: `response.status`,
			check: func(t *testing.T, result any) {
				assert.Equal(t, int64(200), result)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := celConfig.EvaluateAny(tt.expr, ctx)
			require.NoError(t, err)

			tt.check(t, result)

			// All results must be JSON-serializable
			_, err = json.Marshal(result)
			assert.NoError(t, err, "result should be JSON-serializable")
		})
	}
}

func TestEvaluateAnyErrors(t *testing.T) {
	celConfig := NewCEL(
		cel.Variable("response", cel.MapType(cel.StringType, cel.DynType)),
	)

	ctx := map[string]any{
		"response": map[string]any{"status": int64(200)},
	}

	t.Run("invalid syntax returns compile error", func(t *testing.T) {
		_, err := celConfig.EvaluateAny("invalid syntax!!!", ctx)
		assert.Error(t, err)
	})

	t.Run("missing field returns eval error", func(t *testing.T) {
		_, err := celConfig.EvaluateAny("response.nonexistent.deep", ctx)
		assert.Error(t, err)
	})
}

func TestEvaluateEach(t *testing.T) {
	celConfig := NewCEL(
		cel.Variable("item", cel.DynType),
		cel.Variable("items", cel.ListType(cel.DynType)),
	)

	t.Run("iterates over items", func(t *testing.T) {
		ctx := map[string]any{
			"items": []any{
				map[string]any{"value": int64(1)},
				map[string]any{"value": int64(2)},
				map[string]any{"value": int64(3)},
			},
		}
		results, err := celConfig.EvaluateEach("item.value * 2", ctx, "items", "item")
		require.NoError(t, err)
		assert.Equal(t, []any{int64(2), int64(4), int64(6)}, results)
	})

	t.Run("falls back to single eval when key absent", func(t *testing.T) {
		ctx := map[string]any{
			"items": []any{
				map[string]any{"value": int64(10)},
			},
		}
		results, err := celConfig.EvaluateEach("items.size()", ctx, "missing_key", "item")
		require.NoError(t, err)
		assert.Equal(t, []any{int64(1)}, results)
	})

	t.Run("map results in each mode are normalized", func(t *testing.T) {
		ctx := map[string]any{
			"items": []any{
				map[string]any{"v": int64(1)},
			},
		}
		results, err := celConfig.EvaluateEach(`{"doubled": item.v * 2}`, ctx, "items", "item")
		require.NoError(t, err)
		require.Len(t, results, 1)
		m, ok := results[0].(map[string]any)
		require.True(t, ok, "expected map[string]any, got %T", results[0])
		assert.Equal(t, int64(2), m["doubled"])

		_, err = json.Marshal(results)
		assert.NoError(t, err, "results should be JSON-serializable")
	})

	t.Run("error in item wraps index", func(t *testing.T) {
		ctx := map[string]any{
			"items": []any{
				map[string]any{"v": int64(1)},
				"not_a_map",
			},
		}
		_, err := celConfig.EvaluateEach("item.v", ctx, "items", "item")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "items[1]")
	})
}

func TestEvaluateEachConfigured(t *testing.T) {
	t.Run("nil iteration keys evaluates once", func(t *testing.T) {
		celConfig := NewCEL(
			cel.Variable("x", cel.IntType),
		)
		ctx := map[string]any{"x": int64(42)}
		results, err := celConfig.EvaluateEachConfigured("x + 1", ctx)
		require.NoError(t, err)
		assert.Equal(t, []any{int64(43)}, results)
	})

	t.Run("with iteration keys iterates over items", func(t *testing.T) {
		celConfig := NewCEL(
			cel.Variable("item", cel.DynType),
			cel.Variable("items", cel.ListType(cel.DynType)),
		).WithIterationKeys("items", "item")

		ctx := map[string]any{
			"items": []any{
				map[string]any{"n": int64(10)},
				map[string]any{"n": int64(20)},
			},
		}
		results, err := celConfig.EvaluateEachConfigured("item.n", ctx)
		require.NoError(t, err)
		assert.Equal(t, []any{int64(10), int64(20)}, results)
	})
}
