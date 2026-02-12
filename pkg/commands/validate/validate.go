package validate

import (
	"cmp"
	"encoding/json"
	"fmt"
	"os"
	"slices"

	"github.com/mgutz/ansi"
	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/isometry/platform-health/internal/cliflags"
	"github.com/isometry/platform-health/pkg/config"
	"github.com/isometry/platform-health/pkg/phctx"
	"github.com/isometry/platform-health/pkg/provider"
)

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
	cliflags.BindFlags(cmd, v)
	return nil
}

func run(cmd *cobra.Command, _ []string) error {
	v := phctx.Viper(cmd.Context())
	paths, name := cliflags.ConfigPaths(v)
	outputFormat := v.GetString("output")

	// Always use strict mode for validation
	result, err := config.Load(cmd.Context(), paths, name, true)
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	summary := collectResults(result)

	switch outputFormat {
	case "json":
		return outputJSON(summary)
	default:
		return outputText(summary)
	}
}

// ValidationResult represents the validation outcome for a single instance.
type ValidationResult struct {
	Type   string   `json:"type"`
	Name   string   `json:"name"`
	Valid  bool     `json:"valid"`
	Errors []string `json:"errors,omitempty"`
}

// ValidationSummary holds the overall validation results
type ValidationSummary struct {
	Results []ValidationResult `json:"results"`
	Valid   int                `json:"valid"`
	Invalid int                `json:"invalid"`
}

func collectResults(result *config.LoadResult) ValidationSummary {
	var results []ValidationResult

	// Add errors from config loading (instances that failed to load)
	for _, err := range result.ValidationErrors() {
		// Try to extract instance info from InstanceError
		if instErr, ok := err.(provider.InstanceError); ok {
			results = append(results, ValidationResult{
				Type:   instErr.Type,
				Name:   instErr.Name,
				Valid:  false,
				Errors: []string{instErr.Err.Error()},
			})
		} else {
			results = append(results, ValidationResult{
				Type:   "unknown",
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

	// Aggregate results by (Type, Name), merging errors for the same component
	results = aggregateResults(results)

	// Sort results by name for deterministic output
	slices.SortFunc(results, func(a, b ValidationResult) int {
		return cmp.Compare(a.Name, b.Name)
	})

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

func collectComponentResults(inst provider.Instance, pathPrefix string, results *[]ValidationResult) {
	name := inst.GetName()
	if pathPrefix != "" {
		name = pathPrefix + "/" + name
	}

	// Add this instance as valid (it loaded successfully)
	*results = append(*results, ValidationResult{
		Type:  inst.GetType(),
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
			*results = append(*results, ValidationResult{
				Type:   instErr.Type,
				Name:   errName,
				Valid:  false,
				Errors: []string{instErr.Err.Error()},
			})
		}
	}

	for _, child := range container.GetComponents() {
		collectComponentResults(child, name, results)
	}
}

// aggregateResults merges validation results by (Type, Name), combining errors.
// If a component has any errors, it is marked invalid (errors take precedence over valid).
func aggregateResults(results []ValidationResult) []ValidationResult {
	grouped := make(map[string]*ValidationResult)

	for _, r := range results {
		key := r.Type + ":" + r.Name
		if existing, ok := grouped[key]; ok {
			existing.Errors = append(existing.Errors, r.Errors...)
			if !r.Valid {
				existing.Valid = false
			}
		} else {
			copy := r
			grouped[key] = &copy
		}
	}

	merged := make([]ValidationResult, 0, len(grouped))
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

func outputText(summary ValidationSummary) error {
	// Resolve color codes only if stdout is a terminal
	green, red, reset := "", "", ""
	if term.IsTerminal(int(os.Stdout.Fd())) {
		green = ansi.ColorCode("green")
		red = ansi.ColorCode("red")
		reset = ansi.ColorCode("reset")
	}

	fmt.Println("Validation Results:")

	for _, r := range summary.Results {
		if r.Valid {
			fmt.Printf("  %s\u2714%s %s (%s)\n", green, reset, r.Name, r.Type)
		} else {
			fmt.Printf("  %s\u2718%s %s (%s)\n", red, reset, r.Name, r.Type)
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
