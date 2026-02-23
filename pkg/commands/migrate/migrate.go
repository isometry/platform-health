package migrate

import (
	"bytes"
	"fmt"
	"maps"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"go.yaml.in/yaml/v3"

	"github.com/isometry/platform-health/internal/cliflags"
	"github.com/isometry/platform-health/pkg/phctx"
)

// frameworkKeys are component-level keys that should NOT go under spec.
var frameworkKeys = map[string]bool{
	"checks":     true,
	"components": true,
	"timeout":    true,
	"includes":   true,
}

// typeRewrites maps obsolete provider type names to their replacements.
var typeRewrites = map[string]string{
	"rest": "http",
}

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

// transformRest promotes fields from the "request" sub-map to the top level,
// matching the HTTP provider's spec shape (url, method, body, headers).
func transformRest(config map[string]any) {
	reqValue, ok := config["request"]
	if !ok {
		return
	}
	reqMap, ok := reqValue.(map[string]any)
	if !ok {
		return
	}
	maps.Copy(config, reqMap)
	delete(config, "request")
}

// transformChecks rewrites legacy "expr"/"expression" keys to "check" in checks entries.
func transformChecks(config map[string]any) {
	checksValue, ok := config["checks"]
	if !ok {
		return
	}
	checksSlice, ok := checksValue.([]any)
	if !ok {
		return
	}
	for _, item := range checksSlice {
		itemMap, ok := item.(map[string]any)
		if !ok {
			continue
		}
		for _, oldKey := range []string{"expr", "expression"} {
			if val, ok := itemMap[oldKey]; ok {
				itemMap["check"] = val
				delete(itemMap, oldKey)
				break
			}
		}
	}
}

// transformHTTPStatus converts a legacy HTTP "status" field to a CEL check expression.
// Returns a migration note if a transformation was performed.
func transformHTTPStatus(config map[string]any) string {
	statusValue, ok := config["status"]
	if !ok {
		return ""
	}
	statusSlice, ok := statusValue.([]any)
	if !ok {
		return ""
	}
	if len(statusSlice) == 0 {
		return ""
	}

	// Build CEL expression
	var expr string
	if len(statusSlice) == 1 {
		expr = fmt.Sprintf("response.status == %v", statusSlice[0])
	} else {
		parts := make([]string, len(statusSlice))
		for i, s := range statusSlice {
			parts[i] = fmt.Sprintf("%v", s)
		}
		expr = fmt.Sprintf("response.status in [%s]", strings.Join(parts, ", "))
	}

	// Create check entry
	checkEntry := map[string]any{
		"check":   expr,
		"message": "unexpected HTTP status",
	}

	// Append to existing checks or create new
	if existingChecks, ok := config["checks"].([]any); ok {
		config["checks"] = append(existingChecks, checkEntry)
	} else {
		config["checks"] = []any{checkEntry}
	}

	delete(config, "status")
	return fmt.Sprintf("status %v -> CEL check: %s", statusSlice, expr)
}

func run(cmd *cobra.Command, args []string) error {
	v := phctx.Viper(cmd.Context())
	cliflags.BindFlags(cmd, v)

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

			// Rewrite obsolete type names
			providerType := inst.providerType
			if newType, ok := typeRewrites[providerType]; ok {
				renames = append(renames, fmt.Sprintf("  %s: type %s -> %s", finalName, providerType, newType))
				providerType = newType
			}

			// Transform provider-specific config (e.g. flatten rest's request sub-map)
			if inst.providerType == "rest" {
				transformRest(inst.config)
			}

			// Convert legacy HTTP status field to CEL check
			if providerType == "http" {
				if note := transformHTTPStatus(inst.config); note != "" {
					renames = append(renames, fmt.Sprintf("  %s: %s", finalName, note))
				}
			}

			// Rewrite legacy check expression keys (expr/expression -> check)
			transformChecks(inst.config)

			// Build new instance config, routing keys to framework level or spec
			newInstance := make(map[string]any)
			newInstance["type"] = providerType

			spec := make(map[string]any)
			for key, val := range inst.config {
				switch {
				case key == "name":
					// skip: becomes the map key
				case frameworkKeys[key]:
					newInstance[key] = val
				default:
					spec[key] = val
				}
			}
			if len(spec) > 0 {
				newInstance["spec"] = spec
			}

			components[finalName] = newInstance
		}
	}

	// Print warnings
	if len(renames) > 0 {
		_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "Warning: migration notes:")
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
