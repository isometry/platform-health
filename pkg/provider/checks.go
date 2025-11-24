package provider

import (
	"context"
	"fmt"

	"github.com/isometry/platform-health/pkg/checks"
)

// InstanceWithChecks indicates a provider supports CEL expressions for health checks.
// Providers implementing this interface can participate in dynamic CLI commands
// and context inspection via `ph context`.
// Note: Providers implementing InstanceWithChecks should also implement Instance.
type InstanceWithChecks interface {
	// GetCheckConfig returns the provider's CEL variable declarations.
	// This defines what variables are available in CEL expressions.
	GetCheckConfig() *checks.CEL

	// GetCheckContext fetches data and builds the evaluation context.
	// Used for both check evaluation AND context display (ph context).
	// Returns the context map with all variables populated.
	GetCheckContext(ctx context.Context) (map[string]any, error)

	// GetChecks returns configured CEL expressions.
	GetChecks() []checks.Expression

	// SetChecks sets CEL expressions (for dynamic/CLI configuration).
	SetChecks([]checks.Expression)
}

// BaseInstanceWithChecks provides reusable CEL handling that can be embedded by providers.
// It manages CEL expression storage, compilation, and evaluation.
type BaseInstanceWithChecks struct {
	Checks    []checks.Expression `mapstructure:"checks"`
	evaluator *checks.Evaluator
}

// SetupChecks compiles CEL expressions using the provided configuration.
// Call this from the provider's Setup() method.
func (b *BaseInstanceWithChecks) SetupChecks(checkConfig *checks.CEL) error {
	if len(b.Checks) == 0 {
		return nil
	}
	evaluator, err := checkConfig.NewEvaluator(b.Checks)
	if err != nil {
		return fmt.Errorf("invalid CEL expression: %w", err)
	}
	b.evaluator = evaluator
	return nil
}

// EvaluateChecks runs all checks against the provided context.
// Call this from the provider's GetHealth() method.
// Returns nil if all expressions pass, or an error describing the first failure.
func (b *BaseInstanceWithChecks) EvaluateChecks(ctx map[string]any) error {
	if b.evaluator == nil {
		return nil
	}
	return b.evaluator.Evaluate(ctx)
}

// GetChecks returns configured CEL expressions.
func (b *BaseInstanceWithChecks) GetChecks() []checks.Expression {
	return b.Checks
}

// SetChecks sets CEL expressions for dynamic configuration.
// This clears the compiled evaluator, requiring SetupChecks to be called again.
func (b *BaseInstanceWithChecks) SetChecks(exprs []checks.Expression) {
	b.Checks = exprs
	b.evaluator = nil // Force recompilation on next SetupChecks
}

// HasChecks returns true if any CEL expressions are configured.
func (b *BaseInstanceWithChecks) HasChecks() bool {
	return len(b.Checks) > 0
}

// SupportsChecks checks if a provider instance implements InstanceWithChecks.
func SupportsChecks(instance Instance) bool {
	_, ok := instance.(InstanceWithChecks)
	return ok
}

// AsInstanceWithChecks returns the instance as InstanceWithChecks if it implements the interface.
// Returns nil if the instance does not implement InstanceWithChecks.
func AsInstanceWithChecks(instance Instance) InstanceWithChecks {
	if cp, ok := instance.(InstanceWithChecks); ok {
		return cp
	}
	return nil
}
