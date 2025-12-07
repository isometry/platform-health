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
	provider.Base
	provider.BaseWithChecks
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

func (m *mockCheckProvider) GetType() string { return "mock" }
func (m *mockCheckProvider) Setup() error    { return nil }
func (m *mockCheckProvider) SetChecks(exprs []checks.Expression) error {
	return m.SetChecksAndCompile(exprs, m.celConfig)
}
func (m *mockCheckProvider) GetHealth(ctx context.Context) *ph.HealthCheckResponse {
	celCtx, _ := m.GetCheckContext(ctx)
	if msgs := m.EvaluateChecks(ctx, celCtx); len(msgs) > 0 {
		return &ph.HealthCheckResponse{
			Name:     m.GetName(),
			Status:   ph.Status_UNHEALTHY,
			Messages: msgs,
		}
	}
	return &ph.HealthCheckResponse{
		Name:   m.GetName(),
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
	provider.Base
}

func (m *mockNonCheckProvider) GetType() string { return "nonce" }
func (m *mockNonCheckProvider) Setup() error    { return nil }
func (m *mockNonCheckProvider) GetHealth(ctx context.Context) *ph.HealthCheckResponse {
	return &ph.HealthCheckResponse{Name: m.GetName(), Status: ph.Status_HEALTHY}
}

// testableCheckProvider wraps BaseWithChecks for testing
type testableCheckProvider struct {
	provider.Base
	provider.BaseWithChecks
	celConfig *checks.CEL
}

func newTestableCheckProvider(celConfig *checks.CEL) *testableCheckProvider {
	return &testableCheckProvider{celConfig: celConfig}
}

func (t *testableCheckProvider) GetType() string { return "testable" }
func (t *testableCheckProvider) Setup() error    { return nil }
func (t *testableCheckProvider) SetChecks(exprs []checks.Expression) error {
	return t.SetChecksAndCompile(exprs, t.celConfig)
}
func (t *testableCheckProvider) GetHealth(ctx context.Context) *ph.HealthCheckResponse {
	return &ph.HealthCheckResponse{Name: t.GetName(), Status: ph.Status_HEALTHY}
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
			err := p.SetChecks(tt.checks)
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
		name     string
		checks   []checks.Expression
		celCtx   map[string]any
		wantFail bool
		failMsg  string
	}{
		{
			name:     "no checks",
			checks:   nil,
			celCtx:   map[string]any{"value": int64(10)},
			wantFail: false,
		},
		{
			name: "passing check",
			checks: []checks.Expression{
				{Expression: "value > 0"},
			},
			celCtx:   map[string]any{"value": int64(10)},
			wantFail: false,
		},
		{
			name: "failing check",
			checks: []checks.Expression{
				{Expression: "value > 100"},
			},
			celCtx:   map[string]any{"value": int64(10)},
			wantFail: true,
		},
		{
			name: "failing check with custom message",
			checks: []checks.Expression{
				{Expression: "value > 100", Message: "value too small"},
			},
			celCtx:   map[string]any{"value": int64(10)},
			wantFail: true,
			failMsg:  "value too small",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := newTestableCheckProvider(celConfig)
			err := p.SetChecks(tt.checks)
			require.NoError(t, err)

			msgs := p.EvaluateChecks(context.Background(), tt.celCtx)
			if tt.wantFail {
				assert.NotEmpty(t, msgs)
				if tt.failMsg != "" {
					assert.Contains(t, msgs[0], tt.failMsg)
				}
			} else {
				assert.Empty(t, msgs)
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
	err := p.SetChecks(exprs)
	require.NoError(t, err)

	assert.Equal(t, exprs, p.GetChecks())
	assert.True(t, p.HasChecks())

	// Evaluation should work since SetChecks compiles immediately
	msgs := p.EvaluateChecks(context.Background(), map[string]any{"value": int64(10)})
	assert.Empty(t, msgs)

	// After SetChecks with new checks, evaluation uses new checks
	err = p.SetChecks([]checks.Expression{{Expression: "value < 100"}})
	require.NoError(t, err)
	// Should fail because value 200 is not less than 100
	msgs = p.EvaluateChecks(context.Background(), map[string]any{"value": int64(200)})
	assert.NotEmpty(t, msgs)
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
	err := p.SetChecks([]checks.Expression{
		{Expression: "value > 0"},
		{Expression: "name == 'test'"},
	})
	require.NoError(t, err)

	response := p.GetHealth(t.Context())
	assert.Equal(t, ph.Status_HEALTHY, response.Status)

	// Test with failing check
	err = p.SetChecks([]checks.Expression{
		{Expression: "value > 100", Message: "value must be greater than 100"},
	})
	require.NoError(t, err)

	response = p.GetHealth(t.Context())
	assert.Equal(t, ph.Status_UNHEALTHY, response.Status)
	require.NotEmpty(t, response.Messages)
	assert.Contains(t, response.Messages[0], "value must be greater than 100")
}
