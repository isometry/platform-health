package validate

import (
	"cmp"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"slices"

	"github.com/spf13/cobra"
	slogctx "github.com/veqryn/slog-context"
	"golang.org/x/term"

	"github.com/isometry/platform-health/pkg/commands/flags"
	"github.com/isometry/platform-health/pkg/config"
	"github.com/isometry/platform-health/pkg/phctx"
	"github.com/isometry/platform-health/pkg/provider"
)

var log *slog.Logger

func New() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "validate",
		Short: "Validate configuration without running health checks",
		Long: `Validate all component configurations for structural correctness.

This command checks required fields, value constraints, and configuration rules.
It does NOT perform connectivity checks - only structural validation.

Examples:
  # Validate default config
  ph validate

  # Validate specific config file
  ph validate --config-name=myconfig

  # Output as JSON for CI/tooling
  ph validate -o json`,
		Args:    cobra.NoArgs,
		PreRunE: setup,
		RunE:    run,
	}

	validateFlags.Register(cmd.Flags(), false)

	return cmd
}

func setup(cmd *cobra.Command, _ []string) error {
	v := phctx.Viper(cmd.Context())
	flags.BindFlags(cmd, v)

	log = slog.Default()
	cmd.SetContext(slogctx.NewCtx(cmd.Context(), log))

	return nil
}

func run(cmd *cobra.Command, _ []string) error {
	v := phctx.Viper(cmd.Context())
	paths, name := flags.ConfigPaths(v)
	outputFormat := v.GetString("output")

	// Always use strict mode for validation
	result, err := config.Load(cmd.Context(), paths, name, true)
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	// Collect results
	summary := collectResults(result)

	// Output based on format
	switch outputFormat {
	case "json":
		return outputJSON(summary)
	default:
		return outputText(summary)
	}
}

// ValidationSummary holds the overall validation results
type ValidationSummary struct {
	Results []provider.ValidationResult `json:"results"`
	Valid   int                         `json:"valid"`
	Invalid int                         `json:"invalid"`
}

func collectResults(result *config.LoadResult) ValidationSummary {
	var results []provider.ValidationResult

	// Add errors from config loading (instances that failed to load)
	for _, err := range result.ValidationErrors {
		// Try to extract instance info from InstanceError
		if instErr, ok := err.(provider.InstanceError); ok {
			results = append(results, provider.ValidationResult{
				Kind:   instErr.Kind,
				Name:   instErr.Name,
				Valid:  false,
				Errors: []string{instErr.Err.Error()},
			})
		} else {
			results = append(results, provider.ValidationResult{
				Kind:   "unknown",
				Name:   "unknown",
				Valid:  false,
				Errors: []string{err.Error()},
			})
		}
	}

	// Recursively collect from all instances (including nested components)
	for _, inst := range result.GetInstances() {
		collectComponentResults(inst, "", &results)
	}

	// Aggregate results by (Kind, Name), merging errors for the same component
	results = aggregateResults(results)

	// Sort results by name for deterministic output
	slices.SortFunc(results, func(a, b provider.ValidationResult) int {
		return cmp.Compare(a.Name, b.Name)
	})

	// Count valid/invalid
	var valid, invalid int
	for _, r := range results {
		if r.Valid {
			valid++
		} else {
			invalid++
		}
	}

	return ValidationSummary{
		Results: results,
		Valid:   valid,
		Invalid: invalid,
	}
}

// collectComponentResults recursively collects validation results from nested components
func collectComponentResults(inst provider.Instance, pathPrefix string, results *[]provider.ValidationResult) {
	// Build full path
	name := inst.GetName()
	if pathPrefix != "" {
		name = pathPrefix + "/" + name
	}

	// Add this instance as valid (it loaded successfully)
	*results = append(*results, provider.ValidationResult{
		Kind:  inst.GetKind(),
		Name:  name,
		Valid: true,
	})

	// If it has components, collect their errors and recurse
	container := provider.AsContainer(inst)
	if container == nil {
		return
	}

	// Add component errors (with full path)
	for _, err := range container.ComponentErrors() {
		if instErr, ok := err.(provider.InstanceError); ok {
			errName := name + "/" + instErr.Name
			*results = append(*results, provider.ValidationResult{
				Kind:   instErr.Kind,
				Name:   errName,
				Valid:  false,
				Errors: []string{instErr.Err.Error()},
			})
		}
	}

	// Recurse into children with current path as prefix
	for _, child := range container.GetComponents() {
		collectComponentResults(child, name, results)
	}
}

// aggregateResults merges validation results by (Kind, Name), combining errors.
// If a component has any errors, it is marked invalid (errors take precedence over valid).
func aggregateResults(results []provider.ValidationResult) []provider.ValidationResult {
	grouped := make(map[string]*provider.ValidationResult)

	for _, r := range results {
		key := r.Kind + ":" + r.Name
		if existing, ok := grouped[key]; ok {
			// Merge errors
			existing.Errors = append(existing.Errors, r.Errors...)
			// Mark invalid if this result has errors
			if !r.Valid {
				existing.Valid = false
			}
		} else {
			// First occurrence - copy to map
			copy := r
			grouped[key] = &copy
		}
	}

	// Convert back to slice
	merged := make([]provider.ValidationResult, 0, len(grouped))
	for _, r := range grouped {
		merged = append(merged, *r)
	}
	return merged
}

func outputJSON(summary ValidationSummary) error {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(summary); err != nil {
		return fmt.Errorf("failed to encode JSON: %w", err)
	}

	if summary.Invalid > 0 {
		return fmt.Errorf("validation failed: %d invalid component(s)", summary.Invalid)
	}
	return nil
}

// ANSI color codes
const (
	colorReset = "\033[0m"
	colorGreen = "\033[32m"
	colorRed   = "\033[31m"
)

func outputText(summary ValidationSummary) error {
	// Check if stdout is a terminal for color support
	useColor := term.IsTerminal(int(os.Stdout.Fd()))

	fmt.Println("Validation Results:")

	for _, r := range summary.Results {
		if r.Valid {
			if useColor {
				fmt.Printf("  %s\u2714%s %s (%s)\n", colorGreen, colorReset, r.Name, r.Kind)
			} else {
				fmt.Printf("  \u2714 %s (%s)\n", r.Name, r.Kind)
			}
		} else {
			if useColor {
				fmt.Printf("  %s\u2718%s %s (%s)\n", colorRed, colorReset, r.Name, r.Kind)
			} else {
				fmt.Printf("  \u2718 %s (%s)\n", r.Name, r.Kind)
			}
			for _, e := range r.Errors {
				fmt.Printf("      - %s\n", e)
			}
		}
	}

	fmt.Printf("\nSummary: %d valid, %d invalid\n", summary.Valid, summary.Invalid)

	if summary.Invalid > 0 {
		return fmt.Errorf("validation failed: %d invalid component(s)", summary.Invalid)
	}
	return nil
}
