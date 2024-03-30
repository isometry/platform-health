package client

import (
	"context"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	ph "github.com/isometry/platform-health/pkg/platform_health"
)

// Client interface to make requests to the platform.Health/Check endpoint
type Client struct {
	phc ph.HealthClient
}

func NewClient(ctx context.Context, target string) (*Client, error) {
	conn, err := grpc.DialContext(ctx, target, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, err
	}

	// Close the connection when the context is done
	go func() {
		<-ctx.Done()
		conn.Close()
	}()

	return &Client{
		phc: ph.NewHealthClient(conn),
	}, nil
}

func (c *Client) Check(ctx context.Context, request *ph.HealthCheckRequest) (*ph.HealthCheckResponse, error) {
	return c.phc.Check(ctx, request)
}
