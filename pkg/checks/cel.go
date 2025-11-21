// Package checks provides shared CEL (Common Expression Language) evaluation
// capabilities for health check providers.
package checks

import (
	"fmt"
	"reflect"
	"sync"

	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/ext"
)

// CEL holds configuration for CEL expression evaluation including
// variable declarations and a cache for compiled programs.
type CEL struct {
	cache     sync.Map // map[string]cel.Program
	variables []cel.EnvOption
}

// NewCEL creates a CEL configuration with the given variable declarations.
// Each provider should create a package-level instance.
func NewCEL(variables ...cel.EnvOption) *CEL {
	return &CEL{
		variables: variables,
	}
}

// NewEvaluator creates a CEL evaluator for the given expressions.
func (c *CEL) NewEvaluator(exprs []Expression) (*Evaluator, error) {
	if len(exprs) == 0 {
		return nil, nil
	}

	// Create CEL environment with provided variables and extensions
	env, err := cel.NewEnv(append(c.variables, ext.Lists())...)
	if err != nil {
		return nil, fmt.Errorf("failed to create CEL environment: %w", err)
	}

	// Pre-compile all expressions
	programs := make([]cel.Program, len(exprs))
	for idx, expr := range exprs {
		program, err := c.getOrCompile(expr.Expression, env)
		if err != nil {
			return nil, fmt.Errorf("CEL expression [%d] compilation error: %w", idx, err)
		}
		programs[idx] = program
	}

	return &Evaluator{
		programs: programs,
		exprs:    exprs,
	}, nil
}

// getOrCompile returns a cached program or compiles and caches a new one.
func (c *CEL) getOrCompile(expr string, env *cel.Env) (cel.Program, error) {
	// Check cache first
	if cached, ok := c.cache.Load(expr); ok {
		return cached.(cel.Program), nil
	}

	// Compile expression
	ast, issues := env.Compile(expr)
	if issues != nil && issues.Err() != nil {
		return nil, issues.Err()
	}
	if ast.OutputType() != cel.BoolType {
		return nil, fmt.Errorf("expression must return boolean")
	}
	program, err := env.Program(ast)
	if err != nil {
		return nil, err
	}

	// Cache and return
	c.cache.Store(expr, program)
	return program, nil
}

// Expression represents a CEL expression validation rule
type Expression struct {
	Expression   string `mapstructure:"expression"`
	ErrorMessage string `mapstructure:"errorMessage"`
}

// Evaluator holds compiled CEL programs for evaluation
type Evaluator struct {
	programs []cel.Program
	exprs    []Expression
}

// ValidateExpression validates CEL expression syntax at configuration time.
// Variables should be declared using cel.Variable() options.
func ValidateExpression(expression string, variables ...cel.EnvOption) error {
	env, err := cel.NewEnv(append(variables, ext.Lists())...)
	if err != nil {
		return fmt.Errorf("failed to create CEL environment: %w", err)
	}

	ast, issues := env.Compile(expression)
	if issues != nil && issues.Err() != nil {
		return fmt.Errorf("invalid CEL expression: %w", issues.Err())
	}

	if ast.OutputType() != cel.BoolType {
		return fmt.Errorf("CEL expression must return boolean, got %v", ast.OutputType())
	}

	return nil
}

// Evaluate runs all compiled CEL programs against the provided context.
// Returns nil if all expressions evaluate to true, or an error describing
// the first failing expression.
func (e *Evaluator) Evaluate(ctx map[string]any) error {
	if e == nil || len(e.programs) == 0 {
		return nil
	}

	for idx, program := range e.programs {
		result, _, err := program.Eval(ctx)
		if err != nil {
			return fmt.Errorf("CEL expression [%d] failed to evaluate: %w", idx, err)
		}

		// Convert result to native boolean
		value, err := result.ConvertToNative(reflect.TypeOf(false))
		if err != nil {
			return fmt.Errorf("CEL expression [%d] result conversion failed: %w", idx, err)
		}

		// Check if result is boolean true
		if boolResult, ok := value.(bool); !ok || !boolResult {
			if e.exprs[idx].ErrorMessage != "" {
				return fmt.Errorf("%s", e.exprs[idx].ErrorMessage)
			}
			return fmt.Errorf("CEL expression failed: %s", e.exprs[idx].Expression)
		}
	}

	return nil
}
