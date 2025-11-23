// Package context provides the `ph context` command for inspecting CEL evaluation contexts.
package context

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	slogctx "github.com/veqryn/slog-context"
	"go.yaml.in/yaml/v3"

	"github.com/isometry/platform-health/pkg/commands/flags"
	"github.com/isometry/platform-health/pkg/commands/shared"
	"github.com/isometry/platform-health/pkg/config"
	"github.com/isometry/platform-health/pkg/provider"
	"github.com/isometry/platform-health/pkg/provider/system"
)

var log *slog.Logger

// New creates the context command with dynamic provider subcommands.
func New() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "context",
		Short: "Inspect CEL evaluation context for providers",
		Long: `Inspect the CEL evaluation context available to health check expressions.

Provide a component name or path (e.g., 'system/child') for configured components.
Use a provider subcommand for ad-hoc context inspection.`,
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			// Ensure root setup runs (logging, etc.)
			if parent := cmd.Root(); parent.PersistentPreRun != nil {
				parent.PersistentPreRun(cmd, args)
			}
			flags.BindFlags(cmd)
			log = slog.Default()
			cmd.SetContext(slogctx.NewCtx(cmd.Context(), log))
		},
		RunE: runList,
	}

	// Register persistent flags
	contextFlags.Register(cmd.PersistentFlags(), false)

	// Add dynamic provider subcommands
	shared.AddProviderSubcommands(cmd, shared.ProviderSubcommandOptions{
		RequireCEL: true,
		SetupFlags: func(cmd *cobra.Command, _ provider.CELCapable) {
			cmd.Short = fmt.Sprintf("Get CEL context for %s provider", cmd.Use)
			cmd.Long = fmt.Sprintf("Create an ad-hoc %s provider instance and display its CEL evaluation context.", cmd.Use)
		},
		RunFunc: runProviderContext,
	})

	return cmd
}

// runList handles the base context command for configured instance lookup.
func runList(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("instance name required; use --help to see available providers")
	}

	if len(args) > 1 {
		return fmt.Errorf("expected 1 argument, got %d", len(args))
	}

	// Look up configured instance
	return runInstanceContext(cmd, args[0])
}

// runInstanceContext gets context for a configured instance from config file.
// Supports both simple instance names and component paths (e.g., "system/subsystem/instance").
func runInstanceContext(cmd *cobra.Command, instancePath string) error {
	// Load configuration
	paths, name := flags.ConfigPaths()
	conf, err := config.Load(cmd.Context(), paths, name)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Resolve the instance path
	targetInstance, err := resolveInstancePath(conf.GetInstances(), instancePath)
	if err != nil {
		return err
	}

	// Check if CEL-capable
	celCapable := provider.AsCELCapable(targetInstance)
	if celCapable == nil {
		return fmt.Errorf("instance %q (type %s) does not support CEL", instancePath, targetInstance.GetType())
	}

	// Get and display context
	return displayContext(cmd, celCapable)
}

// resolveInstancePath resolves a path like "system/subsystem/instance" to the target instance.
func resolveInstancePath(instances []provider.Instance, path string) (provider.Instance, error) {
	parts := strings.Split(path, "/")
	current := instances

	for i, part := range parts {
		var found provider.Instance
		for _, inst := range current {
			if inst.GetName() == part {
				found = inst
				break
			}
		}

		if found == nil {
			return nil, fmt.Errorf("component %q not found in configuration", strings.Join(parts[:i+1], "/"))
		}

		// If this is the last part, return it
		if i == len(parts)-1 {
			return found, nil
		}

		// Otherwise, it must be a system provider to continue traversal
		sys, ok := found.(*system.System)
		if !ok {
			return nil, fmt.Errorf("component %q is not a system provider (cannot traverse further)", strings.Join(parts[:i+1], "/"))
		}

		current = sys.GetResolved()
	}

	return nil, fmt.Errorf("failed to resolve path %q", path)
}

// runProviderContext creates an ad-hoc provider instance and displays its context.
func runProviderContext(cmd *cobra.Command, providerType string) error {
	// Create and configure provider from flags
	instance, celCapable, err := shared.CreateAndConfigureProvider(cmd, providerType)
	if err != nil {
		return err
	}

	if celCapable == nil {
		return fmt.Errorf("provider type %q does not support CEL", providerType)
	}

	// Setup the provider
	if err := instance.Setup(); err != nil {
		return fmt.Errorf("failed to setup provider: %w", err)
	}

	return displayContext(cmd, celCapable)
}

// displayContext fetches and displays the CEL context in the requested format.
func displayContext(cmd *cobra.Command, celCapable provider.CELCapable) error {
	ctx, err := celCapable.GetCELContext(cmd.Context())
	if err != nil {
		return fmt.Errorf("failed to get CEL context: %w", err)
	}

	output := viper.GetString("output")
	switch strings.ToLower(output) {
	case "yaml", "yml":
		return outputYAML(ctx)
	case "json":
		fallthrough
	default:
		return outputJSON(ctx)
	}
}

// outputJSON prints context as formatted JSON.
func outputJSON(ctx map[string]any) error {
	data, err := json.MarshalIndent(ctx, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal JSON: %w", err)
	}
	fmt.Println(string(data))
	return nil
}

// outputYAML prints context as YAML.
func outputYAML(ctx map[string]any) error {
	var buf bytes.Buffer
	encoder := yaml.NewEncoder(&buf)
	encoder.SetIndent(2)
	if err := encoder.Encode(ctx); err != nil {
		return fmt.Errorf("failed to marshal YAML: %w", err)
	}
	fmt.Print(buf.String())
	return nil
}
