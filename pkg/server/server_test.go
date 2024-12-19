package server

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	ph "github.com/isometry/platform-health/pkg/platform_health"
	"github.com/isometry/platform-health/pkg/platform_health/details"
	"github.com/isometry/platform-health/pkg/provider"
	"github.com/isometry/platform-health/pkg/provider/mock"
)

type mockConfig []provider.Instance

func (c mockConfig) GetInstances() []provider.Instance {
	return c
}

func TestNewPlatformHealthServer(t *testing.T) {
	tests := []struct {
		name             string
		options          []Option
		expectedServices []string
	}{
		{
			name:    "No Options",
			options: []Option{},
			expectedServices: []string{
				"platform_health.v1.Health",
			},
		},
		{
			name:    "With HealthService",
			options: []Option{WithHealthService()},
			expectedServices: []string{
				"grpc.health.v1.Health",
				"platform_health.v1.Health",
			},
		},
		{
			name:    "With Reflection",
			options: []Option{WithReflection()},
			expectedServices: []string{
				"grpc.reflection.v1.ServerReflection",
				"grpc.reflection.v1alpha.ServerReflection",
				"platform_health.v1.Health",
			},
		},
		{
			name:    "With HealthService and Reflection",
			options: []Option{WithHealthService(), WithReflection()},
			expectedServices: []string{
				"grpc.health.v1.Health",
				"grpc.reflection.v1.ServerReflection",
				"grpc.reflection.v1alpha.ServerReflection",
				"platform_health.v1.Health",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			serverId := "test-server"
			conf := mockConfig{}
			phs, err := NewPlatformHealthServer(&serverId, conf, tt.options...)
			if err != nil {
				t.Fatalf("NewPlatformHealthServer() error = %v", err)
			}
			services := phs.grpcServer.GetServiceInfo()
			assert.Equal(t, len(tt.expectedServices), len(services), "expected number of services to be registered")
			for _, service := range tt.expectedServices {
				if _, ok := services[service]; !ok {
					t.Errorf("expected %s to be registered, but it was not", service)
				}
			}
		})
	}
}

func TestPlatformHealthServer_Check(t *testing.T) {
	tests := []struct {
		name               string
		serverId           string
		hops               []string
		providerConfig     mockConfig
		expectedStatus     ph.Status
		expectedComponents int
		expectedDetailLoop *details.Detail_Loop
	}{
		{
			name:               "No Providers",
			serverId:           "server-1",
			expectedStatus:     ph.Status_HEALTHY,
			expectedComponents: 0,
		},
		{
			name:     "All Healthy Providers",
			serverId: "server-1",
			providerConfig: mockConfig{
				&mock.Mock{Name: "m1", Health: ph.Status_HEALTHY},
				&mock.Mock{Name: "m2", Health: ph.Status_HEALTHY},
			},
			expectedStatus:     ph.Status_HEALTHY,
			expectedComponents: 2,
		},
		{
			name:     "Unhealthy Provider First",
			serverId: "server-1",
			providerConfig: mockConfig{
				&mock.Mock{Name: "m1", Health: ph.Status_UNHEALTHY},
				&mock.Mock{Name: "m2", Health: ph.Status_HEALTHY},
			},
			expectedStatus:     ph.Status_UNHEALTHY,
			expectedComponents: 2,
		},
		{
			name:     "Unhealthy Provider Last",
			serverId: "server-1",
			providerConfig: mockConfig{
				&mock.Mock{Name: "m1", Health: ph.Status_HEALTHY},
				&mock.Mock{Name: "m2", Health: ph.Status_UNHEALTHY},
			},
			expectedStatus:     ph.Status_UNHEALTHY,
			expectedComponents: 2,
		},
		{
			name:     "Simple Loop",
			serverId: "server-1",
			hops:     []string{"server-1"},
			providerConfig: mockConfig{
				&mock.Mock{Name: "m1", Health: ph.Status_HEALTHY},
			},
			expectedStatus:     ph.Status_LOOP_DETECTED,
			expectedComponents: 0,
			expectedDetailLoop: &details.Detail_Loop{ServerIds: []string{"server-1", "server-1"}},
		},
		{
			name:     "Complex Loop",
			serverId: "server-1",
			hops:     []string{"server-1", "server-2", "server-3"},
			providerConfig: mockConfig{
				&mock.Mock{Name: "m1", Health: ph.Status_HEALTHY},
			},
			expectedStatus:     ph.Status_LOOP_DETECTED,
			expectedComponents: 0,
			expectedDetailLoop: &details.Detail_Loop{ServerIds: []string{"server-1", "server-2", "server-3", "server-1"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			serverId := tt.serverId

			phs, err := NewPlatformHealthServer(&serverId, tt.providerConfig)
			if err != nil {
				t.Fatalf("NewPlatformHealthServer() error = %v", err)
			}

			req := &ph.HealthCheckRequest{Hops: tt.hops}
			resp, err := phs.Check(context.Background(), req)
			if err != nil {
				t.Fatalf("Check() error = %v", err)
			}

			assert.Equal(t, tt.expectedStatus, resp.Status, "expected status to match")

			assert.Equal(t, tt.expectedComponents, len(resp.Components), "expected number of components to match")

			if tt.expectedDetailLoop != nil {
				var detail details.Detail_Loop
				assert.Equal(t, 1, len(resp.Details), "expected exactly one detail")
				err := resp.Details[0].UnmarshalTo(&detail)
				if err != nil {
					t.Fatalf("UnmarshalTo() error = %v", err)
				}
				assert.Equal(t, tt.expectedDetailLoop.ServerIds, detail.ServerIds, "expected detail to match")
			}
		})
	}
}
