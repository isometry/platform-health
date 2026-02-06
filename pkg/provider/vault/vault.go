package vault

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/cel-go/cel"
	vault "github.com/hashicorp/vault/api"
	"github.com/mcuadros/go-defaults"

	"github.com/isometry/platform-health/pkg/checks"
	"github.com/isometry/platform-health/pkg/phctx"
	ph "github.com/isometry/platform-health/pkg/platform_health"
	"github.com/isometry/platform-health/pkg/provider"
)

const (
	ProviderType   = "vault"
	DefaultTimeout = 1 * time.Second
)

type Component struct {
	provider.Base
	provider.BaseWithChecks

	Address  string `mapstructure:"address"`
	Insecure bool   `mapstructure:"insecure"`
}

var _ provider.InstanceWithChecks = (*Component)(nil)

// CEL configuration for Vault provider
var celConfig = checks.NewCEL(
	cel.Variable("health", cel.MapType(cel.StringType, cel.DynType)),
)

func init() {
	provider.Register(ProviderType, new(Component))
}

func (c *Component) LogValue() slog.Value {
	logAttr := []slog.Attr{
		slog.String("name", c.GetName()),
		slog.String("address", c.Address),
		slog.Bool("insecure", c.Insecure),
		slog.Int("checks", len(c.GetChecks())),
	}
	return slog.GroupValue(logAttr...)
}

func (c *Component) Setup() error {
	if c.GetTimeout() == 0 {
		c.SetTimeout(DefaultTimeout)
	}
	defaults.SetDefaults(c)
	return nil
}

// SetChecks sets and compiles CEL expressions.
func (c *Component) SetChecks(exprs []checks.Expression) error {
	return c.SetChecksAndCompile(exprs, celConfig)
}

// GetCheckConfig returns the Vault provider's CEL variable declarations.
func (c *Component) GetCheckConfig() *checks.CEL {
	return celConfig
}

// GetCheckContext fetches Vault health status and returns the CEL evaluation context.
// Returns {"health": healthMap} containing all health endpoint fields.
func (c *Component) GetCheckContext(ctx context.Context) (map[string]any, error) {
	config := vault.DefaultConfig()
	config.Address = c.Address
	if err := config.ConfigureTLS(&vault.TLSConfig{Insecure: c.Insecure}); err != nil {
		return nil, fmt.Errorf("TLS configuration for %s: %w", c.Address, err)
	}

	client, err := vault.NewClient(config)
	if err != nil {
		return nil, fmt.Errorf("vault client creation for %s: %w", c.Address, err)
	}

	health, err := client.Sys().HealthWithContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("vault health check for %s: %w", c.Address, err)
	}

	return map[string]any{
		"health": map[string]any{
			"Initialized":                health.Initialized,
			"Sealed":                     health.Sealed,
			"Standby":                    health.Standby,
			"PerformanceStandby":         health.PerformanceStandby,
			"ReplicationPerformanceMode": health.ReplicationPerformanceMode,
			"ReplicationDRMode":          health.ReplicationDRMode,
			"ServerTimeUTC":              health.ServerTimeUTC,
			"Version":                    health.Version,
			"ClusterName":                health.ClusterName,
			"ClusterID":                  health.ClusterID,
		},
	}, nil
}

func (c *Component) GetType() string {
	return ProviderType
}

func (c *Component) GetHealth(ctx context.Context) *ph.HealthCheckResponse {
	log := phctx.Logger(ctx, slog.String("provider", ProviderType), slog.Any("instance", c))
	log.Debug("checking")

	component := &ph.HealthCheckResponse{
		Type: ProviderType,
		Name: c.GetName(),
	}
	defer component.LogStatus(log)

	// Get check context (single Vault API call)
	checkCtx, err := c.GetCheckContext(ctx)
	if err != nil {
		return component.Unhealthy(err.Error())
	}

	// Extract health data for traditional checks
	healthData, ok := checkCtx["health"].(map[string]any)
	if !ok {
		return component.Unhealthy(fmt.Sprintf("invalid health context: expected map[string]any, got %T", checkCtx["health"]))
	}

	// Check initialization status
	initialized, ok := healthData["Initialized"].(bool)
	if !ok {
		return component.Unhealthy(fmt.Sprintf("missing initialization status in response from %s", c.Address))
	}
	if !initialized {
		return component.Unhealthy("vault is not initialized")
	}

	// Check seal status
	sealed, ok := healthData["Sealed"].(bool)
	if !ok {
		return component.Unhealthy(fmt.Sprintf("missing seal status in response from %s", c.Address))
	}
	if sealed {
		return component.Unhealthy("vault is sealed")
	}

	// Apply CEL checks
	if msgs := c.EvaluateChecks(ctx, checkCtx); len(msgs) > 0 {
		return component.Unhealthy(msgs...)
	}

	return component.Healthy()
}
