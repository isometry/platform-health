package migrate

import (
	"bytes"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"go.yaml.in/yaml/v3"

	"github.com/isometry/platform-health/pkg/commands/flags"
)

func New() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "migrate <input-file>",
		Short: "Migrate old format config to new format",
		Long:  "Convert YAML config from old provider-array format to new components format",
		Args:  cobra.ExactArgs(1),
		RunE:  run,
	}

	migrateFlags.Register(cmd.Flags(), false)

	return cmd
}

func run(cmd *cobra.Command, args []string) error {
	flags.BindFlags(cmd)

	inputPath := args[0]
	outputPath, _ := cmd.Flags().GetString("output")

	// Read input file
	data, err := os.ReadFile(inputPath)
	if err != nil {
		return fmt.Errorf("failed to read input file: %w", err)
	}

	// Parse as generic map
	var oldConfig map[string]any
	if err := yaml.Unmarshal(data, &oldConfig); err != nil {
		return fmt.Errorf("failed to parse YAML: %w", err)
	}

	// First pass: collect all instances and track name usage
	type instanceData struct {
		providerType string
		config       map[string]any
	}
	nameToInstances := make(map[string][]instanceData)

	for providerType, value := range oldConfig {
		// Check if value is a slice (old format)
		instances, ok := value.([]any)
		if !ok {
			// Not old format, skip
			continue
		}

		for _, instance := range instances {
			instanceMap, ok := instance.(map[string]any)
			if !ok {
				return fmt.Errorf("invalid instance in provider %s: expected map", providerType)
			}

			// Extract name
			nameValue, ok := instanceMap["name"]
			if !ok {
				return fmt.Errorf("instance in provider %s missing 'name' field", providerType)
			}
			name, ok := nameValue.(string)
			if !ok {
				return fmt.Errorf("instance name in provider %s is not a string", providerType)
			}

			nameToInstances[name] = append(nameToInstances[name], instanceData{
				providerType: providerType,
				config:       instanceMap,
			})
		}
	}

	// Second pass: build components, renaming clashes
	components := make(map[string]any)
	var renames []string

	for name, instances := range nameToInstances {
		needsRename := len(instances) > 1

		for _, inst := range instances {
			// Determine final name
			finalName := name
			if needsRename {
				finalName = fmt.Sprintf("%s-%s", name, inst.providerType)
				renames = append(renames, fmt.Sprintf("  %s -> %s (%s)", name, finalName, inst.providerType))
			}

			// Build new instance config
			newInstance := make(map[string]any)
			newInstance["type"] = inst.providerType

			// Copy all fields except "name"
			for key, val := range inst.config {
				if key != "name" {
					newInstance[key] = val
				}
			}

			components[finalName] = newInstance
		}
	}

	// Print rename warnings
	if len(renames) > 0 {
		_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "Warning: renamed clashing instances:")
		for _, r := range renames {
			_, _ = fmt.Fprintln(cmd.ErrOrStderr(), r)
		}
	}

	// Build output
	output := map[string]any{
		"components": components,
	}

	// Marshal to YAML with 2-space indent
	var buf bytes.Buffer
	encoder := yaml.NewEncoder(&buf)
	encoder.SetIndent(2)
	if err := encoder.Encode(output); err != nil {
		return fmt.Errorf("failed to marshal output: %w", err)
	}
	_ = encoder.Close()
	outData := buf.Bytes()

	// Write output
	if outputPath == "" {
		if _, err := fmt.Fprint(cmd.OutOrStdout(), string(outData)); err != nil {
			return fmt.Errorf("failed to write output: %w", err)
		}
	} else {
		if err := os.WriteFile(outputPath, outData, 0644); err != nil {
			return fmt.Errorf("failed to write output file: %w", err)
		}
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Migrated config written to %s\n", outputPath)
	}

	return nil
}
