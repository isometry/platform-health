package provider

import (
	"context"
	"fmt"

	"github.com/google/cel-go/cel"
	"github.com/isometry/platform-health/pkg/checks"
	"github.com/isometry/platform-health/pkg/phctx"
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

	// SetChecks sets and compiles CEL expressions.
	// Returns an error if any expression is invalid.
	SetChecks([]checks.Expression) error
}

// BaseWithChecks provides reusable CEL handling that can be embedded by providers.
// It manages CEL expression storage, compilation, and evaluation.
type BaseWithChecks struct {
	checks   []checks.Expression
	compiled []*checks.Check
}

// SetChecksAndCompile is a helper for providers implementing SetChecks.
// It stores expressions and compiles them using the provided CEL config.
func (b *BaseWithChecks) SetChecksAndCompile(exprs []checks.Expression, config *checks.CEL) error {
	b.checks = exprs
	b.compiled = nil
	if len(exprs) == 0 {
		return nil
	}
	compiled, err := config.CompileAll(exprs)
	if err != nil {
		return fmt.Errorf("invalid CEL expression: %w", err)
	}
	b.compiled = compiled
	return nil
}

// EvaluateChecks runs all checks against the provided CEL context with optional runtime program options.
// This is used to inject runtime function bindings (e.g., kubernetes.Get with K8s client).
// Call this from the provider's GetHealth() method.
// Returns nil if all expressions pass, or a slice of failure messages.
// If fail-fast mode is enabled in the Go context, returns immediately after first failure.
func (b *BaseWithChecks) EvaluateChecks(ctx context.Context, celCtx map[string]any, opts ...cel.ProgramOption) []string {
	if len(b.compiled) == 0 {
		return nil
	}

	failFast := phctx.FailFastFromContext(ctx)
	var msgs []string

	for _, check := range b.compiled {
		msg, err := check.Evaluate(celCtx, opts...)
		if err != nil {
			msgs = append(msgs, err.Error())
			if failFast {
				return msgs
			}
			continue
		}
		if msg != "" {
			msgs = append(msgs, msg)
			if failFast {
				return msgs
			}
		}
	}

	return msgs
}

// GetChecks returns configured CEL expressions.
func (b *BaseWithChecks) GetChecks() []checks.Expression {
	return b.checks
}

// Checks returns compiled checks, optionally filtered by mode.
// Checks() returns all checks.
// Checks(checks.ModeEach) returns only per-item checks.
// Checks(checks.ModeDefault) returns only default checks.
func (b *BaseWithChecks) Checks(modes ...checks.Mode) []*checks.Check {
	if len(modes) == 0 {
		return b.compiled // all checks
	}

	var result []*checks.Check
	for i, expr := range b.checks {
		for _, mode := range modes {
			match := (mode == checks.ModeEach && expr.Each()) ||
				(mode == checks.ModeDefault && !expr.Each())
			if match {
				result = append(result, b.compiled[i])
				break
			}
		}
	}
	return result
}

// HasChecks returns true if any CEL expressions are configured.
func (b *BaseWithChecks) HasChecks() bool {
	return len(b.checks) > 0
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
