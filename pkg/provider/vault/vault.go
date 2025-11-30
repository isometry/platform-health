package vault

import (
	"context"
	"log/slog"

	"github.com/google/cel-go/cel"
	vault "github.com/hashicorp/vault/api"
	"github.com/mcuadros/go-defaults"

	"github.com/isometry/platform-health/pkg/checks"
	"github.com/isometry/platform-health/pkg/phctx"
	ph "github.com/isometry/platform-health/pkg/platform_health"
	"github.com/isometry/platform-health/pkg/provider"
)

const ProviderKind = "vault"

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
	provider.Register(ProviderKind, new(Component))
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
		return nil, err
	}

	client, err := vault.NewClient(config)
	if err != nil {
		return nil, err
	}

	health, err := client.Sys().HealthWithContext(ctx)
	if err != nil {
		return nil, err
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

func (c *Component) GetKind() string {
	return ProviderKind
}

func (c *Component) GetHealth(ctx context.Context) *ph.HealthCheckResponse {
	log := phctx.Logger(ctx, slog.String("provider", ProviderKind), slog.Any("instance", c))
	log.Debug("checking")

	component := &ph.HealthCheckResponse{
		Kind: ProviderKind,
		Name: c.GetName(),
	}
	defer component.LogStatus(log)

	// Get check context (single Vault API call)
	checkCtx, err := c.GetCheckContext(ctx)
	if err != nil {
		return component.Unhealthy(err.Error())
	}

	// Extract health data for traditional checks
	healthData := checkCtx["health"].(map[string]any)

	// Check initialization status
	if !healthData["Initialized"].(bool) {
		return component.Unhealthy("vault is not initialized")
	}

	// Check seal status
	if healthData["Sealed"].(bool) {
		return component.Unhealthy("vault is sealed")
	}

	// Apply CEL checks
	if msgs := c.EvaluateChecks(ctx, checkCtx); len(msgs) > 0 {
		return component.Unhealthy(msgs...)
	}

	return component.Healthy()
}
