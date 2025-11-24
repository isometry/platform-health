package provider_test

import (
	"context"
	"testing"

	"github.com/google/cel-go/cel"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/isometry/platform-health/pkg/checks"
	ph "github.com/isometry/platform-health/pkg/platform_health"
	"github.com/isometry/platform-health/pkg/provider"
)

// mockCheckProvider is a test implementation of InstanceWithChecks
type mockCheckProvider struct {
	provider.BaseWithChecks
	name      string
	celConfig *checks.CEL
}

func newMockCheckProvider() *mockCheckProvider {
	return &mockCheckProvider{
		celConfig: checks.NewCEL(
			cel.Variable("value", cel.IntType),
			cel.Variable("name", cel.StringType),
		),
	}
}

func (m *mockCheckProvider) GetType() string  { return "mock" }
func (m *mockCheckProvider) GetName() string  { return m.name }
func (m *mockCheckProvider) SetName(n string) { m.name = n }
func (m *mockCheckProvider) Setup() error {
	return m.SetupChecks(m.celConfig)
}
func (m *mockCheckProvider) GetHealth(ctx context.Context) *ph.HealthCheckResponse {
	celCtx, _ := m.GetCheckContext(ctx)
	if err := m.EvaluateChecks(celCtx); err != nil {
		return &ph.HealthCheckResponse{
			Name:    m.name,
			Status:  ph.Status_UNHEALTHY,
			Message: err.Error(),
		}
	}
	return &ph.HealthCheckResponse{
		Name:   m.name,
		Status: ph.Status_HEALTHY,
	}
}

func (m *mockCheckProvider) GetCheckConfig() *checks.CEL {
	return m.celConfig
}

func (m *mockCheckProvider) GetCheckContext(ctx context.Context) (map[string]any, error) {
	return map[string]any{
		"value": int64(42),
		"name":  "test",
	}, nil
}

// mockNonCheckProvider is a provider that doesn't implement InstanceWithChecks
type mockNonCheckProvider struct {
	name string
}

func (m *mockNonCheckProvider) GetType() string  { return "nonce" }
func (m *mockNonCheckProvider) GetName() string  { return m.name }
func (m *mockNonCheckProvider) SetName(n string) { m.name = n }
func (m *mockNonCheckProvider) Setup() error     { return nil }
func (m *mockNonCheckProvider) GetHealth(ctx context.Context) *ph.HealthCheckResponse {
	return &ph.HealthCheckResponse{Name: m.name, Status: ph.Status_HEALTHY}
}

// testableCheckProvider wraps BaseWithChecks for testing
type testableCheckProvider struct {
	provider.BaseWithChecks
	name      string
	celConfig *checks.CEL
}

func newTestableCheckProvider(celConfig *checks.CEL) *testableCheckProvider {
	return &testableCheckProvider{celConfig: celConfig}
}

func (t *testableCheckProvider) GetType() string  { return "testable" }
func (t *testableCheckProvider) GetName() string  { return t.name }
func (t *testableCheckProvider) SetName(n string) { t.name = n }
func (t *testableCheckProvider) Setup() error {
	return t.SetupChecks(t.celConfig)
}
func (t *testableCheckProvider) GetHealth(ctx context.Context) *ph.HealthCheckResponse {
	return &ph.HealthCheckResponse{Name: t.name, Status: ph.Status_HEALTHY}
}

func TestBaseWithChecks_SetupChecks(t *testing.T) {
	celConfig := checks.NewCEL(
		cel.Variable("value", cel.IntType),
	)

	tests := []struct {
		name    string
		checks  []checks.Expression
		wantErr bool
	}{
		{
			name:    "no checks",
			checks:  nil,
			wantErr: false,
		},
		{
			name: "valid expression",
			checks: []checks.Expression{
				{Expression: "value > 0"},
			},
			wantErr: false,
		},
		{
			name: "invalid expression",
			checks: []checks.Expression{
				{Expression: "invalid_syntax +++"},
			},
			wantErr: true,
		},
		{
			name: "unknown variable",
			checks: []checks.Expression{
				{Expression: "unknown > 0"},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := newTestableCheckProvider(celConfig)
			p.SetChecks(tt.checks)
			err := p.Setup()
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestBaseWithChecks_EvaluateChecks(t *testing.T) {
	celConfig := checks.NewCEL(
		cel.Variable("value", cel.IntType),
	)

	tests := []struct {
		name    string
		checks  []checks.Expression
		ctx     map[string]any
		wantErr bool
		errMsg  string
	}{
		{
			name:    "no evaluator",
			checks:  nil,
			ctx:     map[string]any{"value": int64(10)},
			wantErr: false,
		},
		{
			name: "passing check",
			checks: []checks.Expression{
				{Expression: "value > 0"},
			},
			ctx:     map[string]any{"value": int64(10)},
			wantErr: false,
		},
		{
			name: "failing check",
			checks: []checks.Expression{
				{Expression: "value > 100"},
			},
			ctx:     map[string]any{"value": int64(10)},
			wantErr: true,
		},
		{
			name: "failing check with custom message",
			checks: []checks.Expression{
				{Expression: "value > 100", Message: "value too small"},
			},
			ctx:     map[string]any{"value": int64(10)},
			wantErr: true,
			errMsg:  "value too small",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := newTestableCheckProvider(celConfig)
			p.SetChecks(tt.checks)
			err := p.Setup()
			require.NoError(t, err)

			err = p.EvaluateChecks(tt.ctx)
			if tt.wantErr {
				assert.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestBaseWithChecks_GetSetChecks(t *testing.T) {
	celConfig := checks.NewCEL(cel.Variable("value", cel.IntType))
	p := newTestableCheckProvider(celConfig)

	// Initially empty
	assert.Empty(t, p.GetChecks())
	assert.False(t, p.HasChecks())

	// Set checks
	exprs := []checks.Expression{
		{Expression: "value > 0"},
	}
	p.SetChecks(exprs)

	assert.Equal(t, exprs, p.GetChecks())
	assert.True(t, p.HasChecks())

	// SetChecks should clear evaluator - verify by checking evaluation behavior
	err := p.Setup()
	require.NoError(t, err)

	// Evaluation should work
	err = p.EvaluateChecks(map[string]any{"value": int64(10)})
	assert.NoError(t, err)

	// After SetChecks, evaluator should be cleared (evaluation should pass without checks)
	p.SetChecks([]checks.Expression{{Expression: "value < 100"}})
	// Without calling Setup again, EvaluateChecks should pass (no evaluator)
	err = p.EvaluateChecks(map[string]any{"value": int64(200)})
	assert.NoError(t, err) // Would fail if evaluator wasn't cleared
}

func TestSupportsChecks(t *testing.T) {
	celProvider := newMockCheckProvider()
	nonCELProvider := &mockNonCheckProvider{}

	assert.True(t, provider.SupportsChecks(celProvider))
	assert.False(t, provider.SupportsChecks(nonCELProvider))
}

func TestAsInstanceWithChecks(t *testing.T) {
	celProvider := newMockCheckProvider()
	nonCELProvider := &mockNonCheckProvider{}

	result := provider.AsInstanceWithChecks(celProvider)
	assert.NotNil(t, result)

	result = provider.AsInstanceWithChecks(nonCELProvider)
	assert.Nil(t, result)
}

func TestMockCELProvider_Integration(t *testing.T) {
	p := newMockCheckProvider()
	p.SetName("test-instance")

	// Test with passing check
	p.SetChecks([]checks.Expression{
		{Expression: "value > 0"},
		{Expression: "name == 'test'"},
	})
	require.NoError(t, p.Setup())

	response := p.GetHealth(context.Background())
	assert.Equal(t, ph.Status_HEALTHY, response.Status)

	// Test with failing check
	p.SetChecks([]checks.Expression{
		{Expression: "value > 100", Message: "value must be greater than 100"},
	})
	require.NoError(t, p.Setup())

	response = p.GetHealth(context.Background())
	assert.Equal(t, ph.Status_UNHEALTHY, response.Status)
	assert.Contains(t, response.Message, "value must be greater than 100")
}
