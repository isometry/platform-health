// Package context provides the `ph context` command for inspecting CEL evaluation contexts.
package context

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"go.yaml.in/yaml/v3"

	"github.com/isometry/platform-health/pkg/commands/flags"
	"github.com/isometry/platform-health/pkg/commands/shared"
	"github.com/isometry/platform-health/pkg/config"
	"github.com/isometry/platform-health/pkg/phctx"
	"github.com/isometry/platform-health/pkg/provider"
)

// New creates the context command with dynamic provider subcommands.
func New() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "context",
		Short: "Inspect CEL evaluation context for providers",
		Long: `Inspect the CEL evaluation context available to health check expressions.

Provide a component name or path (e.g., '<system-name>/<component-name>') for configured components.
Use a provider subcommand for ad-hoc context inspection.

Use --expr to evaluate CEL expressions against the full context.
Use --expr-each to evaluate expressions per-item (for providers returning multiple items).
If no expressions are provided, the full context is displayed.`,
		Args: cobra.ExactArgs(1),
		RunE: runInstanceContext,
	}

	contextFlags.Register(cmd.PersistentFlags(), false)

	cmd.PersistentFlags().StringArray("expr", nil, "CEL expression to evaluate against context (can be specified multiple times)")
	cmd.PersistentFlags().StringArray("expr-each", nil, "CEL expression evaluated per-item (can be specified multiple times)")

	shared.AddProviderSubcommands(cmd, shared.ProviderSubcommandOptions{
		RequireChecks: true,
		SetupFlags: func(cmd *cobra.Command, _ provider.Instance) {
			cmd.Short = fmt.Sprintf("Get check context for %s provider", cmd.Use)
			cmd.Long = fmt.Sprintf("Create an ad-hoc %s provider instance and display its check evaluation context.", cmd.Use)
		},
		RunFunc: runProviderContext,
	})

	return cmd
}

// runInstanceContext gets context for a configured instance from config file.
// Supports both simple instance names and component paths (e.g., "system/subsystem/instance").
func runInstanceContext(cmd *cobra.Command, args []string) error {
	v := phctx.Viper(cmd.Context())
	paths, name := flags.ConfigPaths(v)
	result, err := config.Load(cmd.Context(), paths, name, false)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	instancePath := args[0]

	targetInstance, err := resolveInstancePath(result.GetInstances(), instancePath)
	if err != nil {
		return err
	}

	checkProvider := provider.AsInstanceWithChecks(targetInstance)
	if checkProvider == nil {
		return fmt.Errorf("instance %q (type %s) does not support checks", instancePath, targetInstance.GetType())
	}

	return displayContext(cmd, checkProvider)
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

		// Otherwise, it must be a container to continue traversal
		container := provider.AsContainer(found)
		if container == nil {
			return nil, fmt.Errorf("component %q does not contain sub-components (cannot traverse further)", strings.Join(parts[:i+1], "/"))
		}

		current = container.GetComponents()
	}

	return nil, fmt.Errorf("failed to resolve path %q", path)
}

// runProviderContext creates an ad-hoc provider instance and displays its context.
func runProviderContext(cmd *cobra.Command, providerType string) error {
	instance, err := shared.CreateAndConfigureProvider(cmd, providerType)
	if err != nil {
		return err
	}

	checkProvider := provider.AsInstanceWithChecks(instance)
	if checkProvider == nil {
		return fmt.Errorf("provider type %q does not support checks", providerType)
	}

	return displayContext(cmd, checkProvider)
}

// displayContext fetches and displays the check context in the requested format.
// Uses --expr and --expr-each flags for expression evaluation.
// If no expressions are provided, the full context is displayed.
func displayContext(cmd *cobra.Command, checkProvider provider.InstanceWithChecks) error {
	celCtx, err := checkProvider.GetCheckContext(cmd.Context())
	if err != nil {
		return fmt.Errorf("failed to get check context: %w", err)
	}

	v := phctx.Viper(cmd.Context())
	output := v.GetString("output-format")

	defaultExprs, _ := cmd.Flags().GetStringArray("expr")
	eachExprs, _ := cmd.Flags().GetStringArray("expr-each")

	if len(defaultExprs) > 0 || len(eachExprs) > 0 {
		celConfig := checkProvider.GetCheckConfig()
		results := make(map[string]any)

		for _, expr := range defaultExprs {
			result, err := celConfig.EvaluateAny(expr, celCtx)
			if err != nil {
				return fmt.Errorf("expression %q: %w", expr, err)
			}
			results[expr] = result
		}

		for _, expr := range eachExprs {
			itemResults, err := celConfig.EvaluateEachConfigured(expr, celCtx)
			if err != nil {
				return fmt.Errorf("expression %q: %w", expr, err)
			}
			results[expr] = itemResults
		}

		// Single expression: output result directly; multiple: output as map
		if len(results) == 1 {
			for _, v := range results {
				return outputValue(v, output)
			}
		}
		return outputValue(results, output)
	}

	return outputValue(celCtx, output)
}

// outputValue outputs any value in the requested format.
func outputValue(value any, format string) error {
	switch strings.ToLower(format) {
	case "yaml", "yml":
		var buf bytes.Buffer
		encoder := yaml.NewEncoder(&buf)
		encoder.SetIndent(2)
		if err := encoder.Encode(value); err != nil {
			return fmt.Errorf("failed to marshal YAML: %w", err)
		}
		fmt.Print(buf.String())
		return nil
	case "json":
		var buf bytes.Buffer
		encoder := json.NewEncoder(&buf)
		encoder.SetEscapeHTML(false)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(value); err != nil {
			return fmt.Errorf("failed to marshal JSON: %w", err)
		}
		fmt.Print(buf.String())
		return nil
	default:
		return fmt.Errorf("unsupported output format: %q", format)
	}
}
