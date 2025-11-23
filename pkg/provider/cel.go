package provider

import (
	"context"
	"fmt"
	"reflect"

	"github.com/isometry/platform-health/pkg/checks"
)

// CELCapable indicates a provider supports CEL expressions for health checks.
// Providers implementing this interface can participate in dynamic CLI commands
// and context inspection via `ph context`.
// Note: Providers implementing CELCapable should also implement Instance.
type CELCapable interface {
	// GetCELConfig returns the provider's CEL variable declarations.
	// This defines what variables are available in CEL expressions.
	GetCELConfig() *checks.CEL

	// GetCELContext fetches data and builds the evaluation context.
	// Used for both check evaluation AND context display (ph context).
	// Returns the context map with all variables populated.
	GetCELContext(ctx context.Context) (map[string]any, error)

	// GetChecks returns configured CEL expressions.
	GetChecks() []checks.Expression

	// SetChecks sets CEL expressions (for dynamic/CLI configuration).
	SetChecks([]checks.Expression)
}

// BaseCELProvider provides reusable CEL handling that can be embedded by providers.
// It manages CEL expression storage, compilation, and evaluation.
type BaseCELProvider struct {
	Checks    []checks.Expression `mapstructure:"checks"`
	evaluator *checks.Evaluator
}

// SetupCEL compiles CEL expressions using the provided configuration.
// Call this from the provider's Setup() method.
func (b *BaseCELProvider) SetupCEL(celConfig *checks.CEL) error {
	if len(b.Checks) == 0 {
		return nil
	}
	evaluator, err := celConfig.NewEvaluator(b.Checks)
	if err != nil {
		return fmt.Errorf("invalid CEL expression: %w", err)
	}
	b.evaluator = evaluator
	return nil
}

// EvaluateCEL runs all checks against the provided context.
// Call this from the provider's GetHealth() method.
// Returns nil if all expressions pass, or an error describing the first failure.
func (b *BaseCELProvider) EvaluateCEL(ctx map[string]any) error {
	if b.evaluator == nil {
		return nil
	}
	return b.evaluator.Evaluate(ctx)
}

// GetChecks returns configured CEL expressions.
func (b *BaseCELProvider) GetChecks() []checks.Expression {
	return b.Checks
}

// SetChecks sets CEL expressions for dynamic configuration.
// This clears the compiled evaluator, requiring SetupCEL to be called again.
func (b *BaseCELProvider) SetChecks(exprs []checks.Expression) {
	b.Checks = exprs
	b.evaluator = nil // Force recompilation on next SetupCEL
}

// HasChecks returns true if any CEL expressions are configured.
func (b *BaseCELProvider) HasChecks() bool {
	return len(b.Checks) > 0
}

// GetCELCapableProviders returns a list of provider types that implement CELCapable.
func GetCELCapableProviders() []string {
	mu.RLock()
	defer mu.RUnlock()

	var capable []string
	for name, providerType := range Providers {
		// Create a new instance to check interface
		instance := reflect.New(providerType.Elem()).Interface()
		if _, ok := instance.(CELCapable); ok {
			capable = append(capable, name)
		}
	}
	return capable
}

// NewInstance creates a new instance of the specified provider type.
// Returns nil if the provider type is not registered.
func NewInstance(providerType string) Instance {
	mu.RLock()
	defer mu.RUnlock()

	registeredType, ok := Providers[providerType]
	if !ok {
		return nil
	}

	return reflect.New(registeredType.Elem()).Interface().(Instance)
}

// IsCELCapable checks if a provider instance implements CELCapable.
func IsCELCapable(instance Instance) bool {
	_, ok := instance.(CELCapable)
	return ok
}

// AsCELCapable returns the instance as CELCapable if it implements the interface.
// Returns nil if the instance does not implement CELCapable.
func AsCELCapable(instance Instance) CELCapable {
	if cel, ok := instance.(CELCapable); ok {
		return cel
	}
	return nil
}
