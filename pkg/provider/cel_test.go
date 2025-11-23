package provider

import (
	"context"
	"testing"

	"github.com/google/cel-go/cel"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/isometry/platform-health/pkg/checks"
	ph "github.com/isometry/platform-health/pkg/platform_health"
)

// mockCELProvider is a test implementation of CELCapable
type mockCELProvider struct {
	BaseCELProvider
	name      string
	celConfig *checks.CEL
}

func newMockCELProvider() *mockCELProvider {
	return &mockCELProvider{
		celConfig: checks.NewCEL(
			cel.Variable("value", cel.IntType),
			cel.Variable("name", cel.StringType),
		),
	}
}

func (m *mockCELProvider) GetType() string  { return "mock" }
func (m *mockCELProvider) GetName() string  { return m.name }
func (m *mockCELProvider) SetName(n string) { m.name = n }
func (m *mockCELProvider) Setup() error {
	return m.SetupCEL(m.celConfig)
}
func (m *mockCELProvider) GetHealth(ctx context.Context) *ph.HealthCheckResponse {
	celCtx, _ := m.GetCELContext(ctx)
	if err := m.EvaluateCEL(celCtx); err != nil {
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

func (m *mockCELProvider) GetCELConfig() *checks.CEL {
	return m.celConfig
}

func (m *mockCELProvider) GetCELContext(ctx context.Context) (map[string]any, error) {
	return map[string]any{
		"value": int64(42),
		"name":  "test",
	}, nil
}

// mockNonCELProvider is a provider that doesn't implement CELCapable
type mockNonCELProvider struct {
	name string
}

func (m *mockNonCELProvider) GetType() string  { return "nonce" }
func (m *mockNonCELProvider) GetName() string  { return m.name }
func (m *mockNonCELProvider) SetName(n string) { m.name = n }
func (m *mockNonCELProvider) Setup() error     { return nil }
func (m *mockNonCELProvider) GetHealth(ctx context.Context) *ph.HealthCheckResponse {
	return &ph.HealthCheckResponse{Name: m.name, Status: ph.Status_HEALTHY}
}

func TestBaseCELProvider_SetupCEL(t *testing.T) {
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
			b := &BaseCELProvider{Checks: tt.checks}
			err := b.SetupCEL(celConfig)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestBaseCELProvider_EvaluateCEL(t *testing.T) {
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
			b := &BaseCELProvider{Checks: tt.checks}
			err := b.SetupCEL(celConfig)
			require.NoError(t, err)

			err = b.EvaluateCEL(tt.ctx)
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

func TestBaseCELProvider_GetSetChecks(t *testing.T) {
	b := &BaseCELProvider{}

	// Initially empty
	assert.Empty(t, b.GetChecks())
	assert.False(t, b.HasChecks())

	// Set checks
	exprs := []checks.Expression{
		{Expression: "value > 0"},
	}
	b.SetChecks(exprs)

	assert.Equal(t, exprs, b.GetChecks())
	assert.True(t, b.HasChecks())

	// SetChecks should clear evaluator
	celConfig := checks.NewCEL(cel.Variable("value", cel.IntType))
	_ = b.SetupCEL(celConfig)
	assert.NotNil(t, b.evaluator)

	b.SetChecks([]checks.Expression{{Expression: "value < 100"}})
	assert.Nil(t, b.evaluator)
}

func TestIsCELCapable(t *testing.T) {
	celProvider := newMockCELProvider()
	nonCELProvider := &mockNonCELProvider{}

	assert.True(t, IsCELCapable(celProvider))
	assert.False(t, IsCELCapable(nonCELProvider))
}

func TestAsCELCapable(t *testing.T) {
	celProvider := newMockCELProvider()
	nonCELProvider := &mockNonCELProvider{}

	result := AsCELCapable(celProvider)
	assert.NotNil(t, result)

	result = AsCELCapable(nonCELProvider)
	assert.Nil(t, result)
}

func TestMockCELProvider_Integration(t *testing.T) {
	provider := newMockCELProvider()
	provider.SetName("test-instance")

	// Test with passing check
	provider.SetChecks([]checks.Expression{
		{Expression: "value > 0"},
		{Expression: "name == 'test'"},
	})
	require.NoError(t, provider.Setup())

	response := provider.GetHealth(context.Background())
	assert.Equal(t, ph.Status_HEALTHY, response.Status)

	// Test with failing check
	provider.SetChecks([]checks.Expression{
		{Expression: "value > 100", Message: "value must be greater than 100"},
	})
	require.NoError(t, provider.Setup())

	response = provider.GetHealth(context.Background())
	assert.Equal(t, ph.Status_UNHEALTHY, response.Status)
	assert.Contains(t, response.Message, "value must be greater than 100")
}

func TestNewInstance(t *testing.T) {
	// Register a test provider
	Register("testcel", newMockCELProvider())
	defer func() {
		mu.Lock()
		delete(Providers, "testcel")
		mu.Unlock()
	}()

	// Get instance of registered provider
	instance := NewInstance("testcel")
	assert.NotNil(t, instance)
	assert.Equal(t, "mock", instance.GetType())

	// Get instance of unregistered provider
	instance = NewInstance("nonexistent")
	assert.Nil(t, instance)
}

func TestGetCELCapableProviders(t *testing.T) {
	// Register a CEL-capable provider
	Register("testcelcapable", newMockCELProvider())
	defer func() {
		mu.Lock()
		delete(Providers, "testcelcapable")
		mu.Unlock()
	}()

	// Get CEL-capable providers
	providers := GetCELCapableProviders()
	assert.Contains(t, providers, "testcelcapable")

	// Verify it's actually CEL-capable
	instance := NewInstance("testcelcapable")
	assert.NotNil(t, instance)
	assert.True(t, IsCELCapable(instance))
}
