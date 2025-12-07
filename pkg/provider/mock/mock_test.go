package mock_test

import (
	"testing"
	"time"

	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/isometry/platform-health/pkg/checks"
	ph "github.com/isometry/platform-health/pkg/platform_health"
	"github.com/isometry/platform-health/pkg/provider"
	"github.com/isometry/platform-health/pkg/provider/mock"
)

func TestMock(t *testing.T) {
	tests := []struct {
		name   string
		mock   *mock.Component
		expect ph.Status
	}{
		{
			name: "HealthyComponent",
			mock: &mock.Component{
				InstanceName: "TestComponent",
				Health:       ph.Status_HEALTHY,
			},
			expect: ph.Status_HEALTHY,
		},
		{
			name: "HealthyComponentWithSleep",
			mock: &mock.Component{
				InstanceName: "TestComponent",
				Health:       ph.Status_HEALTHY,
				Sleep:        50 * time.Millisecond,
			},
			expect: ph.Status_HEALTHY,
		},

		{
			name: "UnhealthyComponent",
			mock: &mock.Component{
				InstanceName: "TestComponent",
				Health:       ph.Status_UNHEALTHY,
			},
			expect: ph.Status_UNHEALTHY,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.NoError(t, tt.mock.Setup())

			start := time.Now()
			result := tt.mock.GetHealth(t.Context())
			duration := time.Since(start)

			assert.Equal(t, tt.expect, result.GetStatus())
			assert.Greater(t, duration, tt.mock.Sleep)
		})
	}
}

func TestMock_Interfaces(t *testing.T) {
	m := &mock.Component{InstanceName: "test"}
	require.NoError(t, m.Setup())

	t.Run("InstanceWithChecks", func(t *testing.T) {
		assert.True(t, provider.SupportsChecks(m))
		celProvider := provider.AsInstanceWithChecks(m)
		assert.NotNil(t, celProvider)
	})
}

func TestMock_GetCheckConfig(t *testing.T) {
	m := &mock.Component{InstanceName: "test"}
	require.NoError(t, m.Setup())

	celConfig := m.GetCheckConfig()
	assert.NotNil(t, celConfig)
}

func TestMock_GetCheckContext(t *testing.T) {
	tests := []struct {
		name   string
		mock   *mock.Component
		health string
		sleep  string
	}{
		{
			name:   "DefaultValues",
			mock:   &mock.Component{InstanceName: "test", Health: ph.Status_HEALTHY, Sleep: time.Nanosecond},
			health: "HEALTHY",
			sleep:  "1ns",
		},
		{
			name:   "UnhealthyStatus",
			mock:   &mock.Component{InstanceName: "test", Health: ph.Status_UNHEALTHY},
			health: "UNHEALTHY",
			sleep:  "1ns",
		},
		{
			name:   "CustomSleep",
			mock:   &mock.Component{InstanceName: "test", Health: ph.Status_HEALTHY, Sleep: 100 * time.Millisecond},
			health: "HEALTHY",
			sleep:  "100ms",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.NoError(t, tt.mock.Setup())

			ctx, err := tt.mock.GetCheckContext(t.Context())
			require.NoError(t, err)

			mockData, ok := ctx["mock"].(map[string]any)
			require.True(t, ok)
			assert.Equal(t, tt.health, mockData["health"])
			assert.Equal(t, tt.sleep, mockData["sleep"])
		})
	}
}

func TestMock_CELEvaluation(t *testing.T) {
	tests := []struct {
		name         string
		mock         *mock.Component
		checks       []checks.Expression
		expectStatus ph.Status
		expectMsg    string
	}{
		{
			name:         "NoChecks",
			mock:         &mock.Component{InstanceName: "test", Health: ph.Status_HEALTHY},
			expectStatus: ph.Status_HEALTHY,
		},
		{
			name: "PassingCheck",
			mock: &mock.Component{InstanceName: "test", Health: ph.Status_HEALTHY},
			checks: []checks.Expression{
				{Expression: `mock.health == "HEALTHY"`},
			},
			expectStatus: ph.Status_HEALTHY,
		},
		{
			name: "FailingCheck",
			mock: &mock.Component{InstanceName: "test", Health: ph.Status_UNHEALTHY},
			checks: []checks.Expression{
				{Expression: `mock.health == "HEALTHY"`},
			},
			expectStatus: ph.Status_UNHEALTHY,
			expectMsg:    `CEL check failed: mock.health == "HEALTHY"`,
		},
		{
			name: "FailingCheckWithMessage",
			mock: &mock.Component{InstanceName: "test", Health: ph.Status_UNHEALTHY},
			checks: []checks.Expression{
				{Expression: `mock.health == "HEALTHY"`, Message: "mock is not healthy"},
			},
			expectStatus: ph.Status_UNHEALTHY,
			expectMsg:    "mock is not healthy",
		},
		{
			name: "MultipleChecks_AllPass",
			mock: &mock.Component{InstanceName: "test", Health: ph.Status_HEALTHY, Sleep: 10 * time.Millisecond},
			checks: []checks.Expression{
				{Expression: `mock.health == "HEALTHY"`},
				{Expression: `mock.sleep == "10ms"`},
			},
			expectStatus: ph.Status_HEALTHY,
		},
		{
			name: "MultipleChecks_OneFails",
			mock: &mock.Component{InstanceName: "test", Health: ph.Status_HEALTHY},
			checks: []checks.Expression{
				{Expression: `mock.health == "HEALTHY"`},
				{Expression: `mock.health == "UNHEALTHY"`, Message: "second check failed"},
			},
			expectStatus: ph.Status_UNHEALTHY,
			expectMsg:    "second check failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.NoError(t, tt.mock.SetChecks(tt.checks))
			require.NoError(t, tt.mock.Setup())

			result := tt.mock.GetHealth(t.Context())
			assert.Equal(t, tt.expectStatus, result.GetStatus())
			if tt.expectMsg != "" {
				require.NotEmpty(t, result.Messages)
				assert.Contains(t, result.Messages[0], tt.expectMsg)
			}
		})
	}
}

func TestMock_GetProviderFlags(t *testing.T) {
	m := &mock.Component{InstanceName: "test"}

	flags := provider.ProviderFlags(m)

	t.Run("HealthFlag", func(t *testing.T) {
		health, ok := flags["health"]
		require.True(t, ok)
		assert.Equal(t, "string", health.Kind)
		assert.Equal(t, "HEALTHY", health.DefaultValue)
	})

	t.Run("SleepFlag", func(t *testing.T) {
		sleep, ok := flags["sleep"]
		require.True(t, ok)
		assert.Equal(t, "duration", sleep.Kind)
		assert.Equal(t, "1ns", sleep.DefaultValue)
	})
}

func TestMock_ConfigureFromFlags(t *testing.T) {
	tests := []struct {
		name         string
		healthFlag   string
		sleepFlag    time.Duration
		expectHealth ph.Status
		expectSleep  time.Duration
	}{
		{
			name:         "ValidHealthy",
			healthFlag:   "HEALTHY",
			sleepFlag:    10 * time.Millisecond,
			expectHealth: ph.Status_HEALTHY,
			expectSleep:  10 * time.Millisecond,
		},
		{
			name:         "ValidUnhealthy",
			healthFlag:   "UNHEALTHY",
			sleepFlag:    5 * time.Second,
			expectHealth: ph.Status_UNHEALTHY,
			expectSleep:  5 * time.Second,
		},
		{
			name:         "ZeroSleep",
			healthFlag:   "HEALTHY",
			sleepFlag:    0,
			expectHealth: ph.Status_HEALTHY,
			expectSleep:  0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &mock.Component{InstanceName: "test"}

			fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
			fs.String("health", "HEALTHY", "")
			fs.Duration("sleep", time.Nanosecond, "")

			require.NoError(t, fs.Set("health", tt.healthFlag))
			require.NoError(t, fs.Set("sleep", tt.sleepFlag.String()))

			err := provider.ConfigureFromFlags(m, fs)
			require.NoError(t, err)
			assert.Equal(t, tt.expectHealth, m.Health)
			assert.Equal(t, tt.expectSleep, m.Sleep)
		})
	}
}
