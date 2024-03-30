package server

import (
	"context"
	"net"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"
	"google.golang.org/protobuf/types/known/durationpb"

	ph "github.com/isometry/platform-health/pkg/platform_health"
	"github.com/isometry/platform-health/pkg/provider"
)

type PlatformHealthServer struct {
	ph.UnimplementedHealthServer
	Config     provider.Config
	grpcServer *grpc.Server
	grpcHealth *gRPCHealthServer
}

type gRPCHealthServer struct {
	grpc_health_v1.UnimplementedHealthServer
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

func NewPlatformHealthServer(conf provider.Config, options ...Option) (*PlatformHealthServer, error) {
	phs := &PlatformHealthServer{
		Config:     conf,
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

func (s *PlatformHealthServer) Check(ctx context.Context, req *ph.HealthCheckRequest) (*ph.HealthCheckResponse, error) {
	providerServices := s.Config.GetInstances()

	start := time.Now()
	platformServices, healthy := provider.Check(ctx, providerServices)
	duration := durationpb.New(time.Since(start))

	component := ph.HealthCheckResponse{
		Status:     healthy,
		Duration:   duration,
		Components: platformServices,
	}

	return &component, nil
}

func (s *gRPCHealthServer) Check(ctx context.Context, req *grpc_health_v1.HealthCheckRequest) (*grpc_health_v1.HealthCheckResponse, error) {
	return &grpc_health_v1.HealthCheckResponse{
		Status: grpc_health_v1.HealthCheckResponse_SERVING,
	}, nil
}
