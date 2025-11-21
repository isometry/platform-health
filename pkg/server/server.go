package server

import (
	"context"
	"fmt"
	"net"
	"slices"
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/durationpb"

	ph "github.com/isometry/platform-health/pkg/platform_health"
	"github.com/isometry/platform-health/pkg/platform_health/details"
	"github.com/isometry/platform-health/pkg/provider"
)

type PlatformHealthServer struct {
	ph.UnimplementedHealthServer
	Config     provider.Config
	serverId   *string
	grpcServer *grpc.Server
	grpcHealth *gRPCHealthServer
}

type gRPCHealthServer struct {
	grpc_health_v1.UnimplementedHealthServer
}

type Hops []string // IDs of the platform-health servers that have been visited
type HopsKey string

const hopsKey = HopsKey("hops")

func ContextWithHops(ctx context.Context, hops Hops) context.Context {
	return context.WithValue(ctx, hopsKey, hops)
}

func HopsFromContext(ctx context.Context) Hops {
	if hops, ok := ctx.Value(hopsKey).(Hops); ok {
		return hops
	} else {
		return Hops{}
	}
}

// ComponentPaths represents hierarchical component paths for filtering health checks
type ComponentPaths [][]string // Each path like ["system", "subcomponent"]
type componentPathsKeyType string

const componentPathsKey = componentPathsKeyType("componentPaths")

func ContextWithComponentPaths(ctx context.Context, paths ComponentPaths) context.Context {
	return context.WithValue(ctx, componentPathsKey, paths)
}

func ComponentPathsFromContext(ctx context.Context) ComponentPaths {
	if paths, ok := ctx.Value(componentPathsKey).(ComponentPaths); ok {
		return paths
	}
	return nil
}

// filterInstances filters provider instances based on component paths
// Returns the filtered instances and any invalid component names
func filterInstances(instances []provider.Instance, components []string) ([]provider.Instance, []string) {
	// Empty components means check all instances
	if len(components) == 0 {
		return instances, nil
	}

	// Build map for quick lookup
	instanceMap := make(map[string]provider.Instance)
	for _, inst := range instances {
		instanceMap[inst.GetName()] = inst
	}

	// Track which instances to include and their sub-components
	type componentMatch struct {
		instance      provider.Instance
		subComponents [][]string
	}
	matched := make(map[string]*componentMatch)
	var invalidComponents []string

	for _, component := range components {
		parts := strings.Split(component, "/")
		topLevel := parts[0]

		inst, ok := instanceMap[topLevel]
		if !ok {
			invalidComponents = append(invalidComponents, component)
			continue
		}

		if _, exists := matched[topLevel]; !exists {
			matched[topLevel] = &componentMatch{instance: inst}
		}

		// If there are sub-components, track them for hierarchical filtering
		if len(parts) > 1 {
			matched[topLevel].subComponents = append(matched[topLevel].subComponents, parts[1:])
		}
	}

	// Build result slice
	result := make([]provider.Instance, 0, len(matched))
	for _, cm := range matched {
		if len(cm.subComponents) > 0 {
			// Wrap instance with component filtering capability
			result = append(result, &filteredInstance{
				Instance: cm.instance,
				subPaths: cm.subComponents,
			})
		} else {
			result = append(result, cm.instance)
		}
	}

	return result, invalidComponents
}

// filteredInstance wraps a provider.Instance to pass sub-component paths via context
type filteredInstance struct {
	provider.Instance
	subPaths [][]string
}

func (f *filteredInstance) GetHealth(ctx context.Context) *ph.HealthCheckResponse {
	// Pass sub-component paths via context for hierarchical filtering
	ctx = ContextWithComponentPaths(ctx, f.subPaths)
	return f.Instance.GetHealth(ctx)
}

type Option func(*PlatformHealthServer)

func WithReflection() Option {
	return func(s *PlatformHealthServer) {
		reflection.Register(s.grpcServer)
	}
}

func WithHealthService() Option {
	return func(s *PlatformHealthServer) {
		if s.grpcHealth == nil {
			s.grpcHealth = &gRPCHealthServer{}
		}
		grpc_health_v1.RegisterHealthServer(s.grpcServer, s.grpcHealth)
	}
}

func NewPlatformHealthServer(serverId *string, conf provider.Config, options ...Option) (*PlatformHealthServer, error) {
	phs := &PlatformHealthServer{
		Config:     conf,
		serverId:   serverId,
		grpcServer: grpc.NewServer(),
	}

	for _, option := range options {
		option(phs)
	}

	ph.RegisterHealthServer(phs.grpcServer, phs)

	return phs, nil
}

func (s *PlatformHealthServer) Serve(lis net.Listener) error {
	return s.grpcServer.Serve(lis)
}

func (s *PlatformHealthServer) Stop() {
	s.grpcServer.Stop()
}

func (s *PlatformHealthServer) alreadyVisitedServer(hops []string) int {
	if s.serverId == nil {
		return -1
	}

	return slices.Index[Hops, string](hops, *s.serverId)
}

func (s *PlatformHealthServer) Check(ctx context.Context, req *ph.HealthCheckRequest) (*ph.HealthCheckResponse, error) {
	hops := req.GetHops()
	if i := s.alreadyVisitedServer(hops); i != -1 {
		response := &ph.HealthCheckResponse{
			ServerId: s.serverId,
			Status:   ph.Status_LOOP_DETECTED,
		}
		if detail, err := anypb.New(&details.Detail_Loop{ServerIds: append(hops[i:], *s.serverId)}); err == nil {
			response.Details = append(response.Details, detail)
		}
		return response, nil
	}

	// Add this server to the list of visited servers and push to context for consumption by satellite instances
	hops = append(hops, *s.serverId)
	ctx = ContextWithHops(ctx, hops)

	providerServices := s.Config.GetInstances()

	// Apply component filtering if specified
	components := req.GetComponents()
	var invalidComponents []string
	if len(components) > 0 {
		providerServices, invalidComponents = filterInstances(providerServices, components)
	}

	// Return error for invalid components
	if len(invalidComponents) > 0 {
		return &ph.HealthCheckResponse{
			Status:  ph.Status_UNHEALTHY,
			Message: fmt.Sprintf("invalid components: %s", strings.Join(invalidComponents, ", ")),
		}, nil
	}

	start := time.Now()
	platformServices, health := provider.Check(ctx, providerServices)
	duration := durationpb.New(time.Since(start))

	component := ph.HealthCheckResponse{
		Status:     health,
		Components: platformServices,
		Duration:   duration,
	}

	// If a loop was detected, expose serverId to assist debugging
	if health == ph.Status_LOOP_DETECTED {
		component.ServerId = s.serverId
	}

	return &component, nil
}

func (s *gRPCHealthServer) Check(ctx context.Context, req *grpc_health_v1.HealthCheckRequest) (*grpc_health_v1.HealthCheckResponse, error) {
	return &grpc_health_v1.HealthCheckResponse{
		Status: grpc_health_v1.HealthCheckResponse_SERVING,
	}, nil
}
