package server

import (
	"context"
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

type Option func(*PlatformHealthServer)

func WithReflection() Option {
	return func(s *PlatformHealthServer) {
		reflection.Register(s.grpcServer)
	}
}

func WithHealthService() Option {
	return func(s *PlatformHealthServer) {
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

	componentPath := strings.Split(req.Component, "/")
	if len(componentPath) > 1 {
		switch len(componentPath) {
		case 1:
			// invalid component path
		case 2:
			for _, instance := range providerServices {
				if instance.GetType() == componentPath[0] && instance.GetName() == componentPath[1] {
					providerServices = []provider.Instance{instance}
					goto singleComponent
				}
			}
			// invalid component path
		default:
			for _, instance := range providerServices {
				// "satellite" is a special case, as it is the only provider that can have multiple components
				if instance.GetType() == "satellite" && instance.GetName() == componentPath[0] {
					providerServices = []provider.Instance{instance}
					instance.SetComponent(strings.Join(componentPath[1:], "/"))
					goto singleComponent
				}
			}
			// invalid component path
		}
		return &ph.HealthCheckResponse{
			Status:  ph.Status_UNKNOWN,
			Message: "invalid component path",
		}, nil
	}
singleComponent:

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
