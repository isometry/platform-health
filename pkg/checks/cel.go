// Package checks provides shared CEL (Common Expression Language) evaluation
// capabilities for health check providers.
package checks

import (
	"fmt"
	"reflect"
	"sync"

	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/ext"
	"google.golang.org/protobuf/types/known/structpb"

	"github.com/isometry/platform-health/pkg/checks/functions"
)

// standardFunctions provides custom CEL functions available to all expressions.
var standardFunctions = functions.All()

// CEL holds configuration for CEL expression evaluation including
// variable declarations and a cache for compiled ASTs.
type CEL struct {
	cache     sync.Map // map[string]*cel.Ast
	variables []cel.EnvOption
	envOnce   sync.Once
	env       *cel.Env
}

// NewCEL creates a CEL configuration with the given variable declarations.
// Standard functions (like `time.Now()`) are automatically included.
// Each provider should create a package-level instance.
func NewCEL(variables ...cel.EnvOption) *CEL {
	return &CEL{
		variables: append(standardFunctions, variables...),
	}
}

// getEnv returns the cached CEL environment, creating it if necessary.
func (c *CEL) getEnv() (*cel.Env, error) {
	var initErr error
	c.envOnce.Do(func() {
		c.env, initErr = cel.NewEnv(append(c.variables,
			ext.Bindings(), // cel.bind()
			ext.Strings(),  // .lowerAscii(), .upperAscii(), .trim(), .replace(), etc.
			ext.Lists(),    // .slice(), .flatten(), etc.
			ext.Encoders(), // base64.encode/decode, etc.
			ext.Math(),     // math.greatest(), math.least(), etc.
			ext.Sets(),     // sets.contains(), sets.intersects(), etc.
		)...)
	})
	if initErr != nil {
		return nil, fmt.Errorf("failed to create CEL environment: %w", initErr)
	}
	return c.env, nil
}

// Mode represents a check execution mode for filtering.
type Mode int

const (
	ModeDefault Mode = iota // checks with no mode or empty mode
	ModeEach                // checks with mode: "each" (per-item)
)

// Check represents a single compiled CEL check.
type Check struct {
	env  *cel.Env
	ast  *cel.Ast
	expr Expression
}

// Evaluate runs this check against the context.
// Returns ("", nil) on success, (message, nil) on check failure, ("", err) on evaluation error.
func (c *Check) Evaluate(ctx map[string]any) (string, error) {
	return c.EvaluateWithBindings(ctx)
}

// EvaluateWithBindings runs this check with optional runtime program options.
// This allows injecting runtime function bindings (e.g., kubernetes.Get with client).
// Returns ("", nil) on success, (message, nil) on check failure, ("", err) on evaluation error.
func (c *Check) EvaluateWithBindings(ctx map[string]any, opts ...cel.ProgramOption) (string, error) {
	program, err := c.env.Program(c.ast, opts...)
	if err != nil {
		return "", fmt.Errorf("CEL program creation failed: %w", err)
	}

	result, _, err := program.Eval(ctx)
	if err != nil {
		return "", fmt.Errorf("CEL evaluation failed: %w", err)
	}

	value, err := result.ConvertToNative(reflect.TypeOf(false))
	if err != nil {
		return "", fmt.Errorf("CEL result conversion failed: %w", err)
	}

	if boolResult, ok := value.(bool); !ok || !boolResult {
		if c.expr.Message != "" {
			return c.expr.Message, nil
		}
		return fmt.Sprintf("CEL check failed: %s", c.expr.Expression), nil
	}

	return "", nil
}

// Compile compiles a single expression into a Check.
func (c *CEL) Compile(expr Expression) (*Check, error) {
	env, err := c.getEnv()
	if err != nil {
		return nil, err
	}

	ast, err := c.getOrCompileAST(expr.Expression, env)
	if err != nil {
		return nil, err
	}

	return &Check{env: env, ast: ast, expr: expr}, nil
}

// CompileAll compiles multiple expressions into Checks.
func (c *CEL) CompileAll(exprs []Expression) ([]*Check, error) {
	if len(exprs) == 0 {
		return nil, nil
	}

	env, err := c.getEnv()
	if err != nil {
		return nil, err
	}

	compiled := make([]*Check, len(exprs))
	for i, expr := range exprs {
		ast, err := c.getOrCompileAST(expr.Expression, env)
		if err != nil {
			return nil, fmt.Errorf("check[%d]: %w", i, err)
		}
		compiled[i] = &Check{env: env, ast: ast, expr: expr}
	}
	return compiled, nil
}

// getOrCompileAST returns a cached AST or compiles and caches a new one.
func (c *CEL) getOrCompileAST(expr string, env *cel.Env) (*cel.Ast, error) {
	// Check cache first
	if cached, ok := c.cache.Load(expr); ok {
		return cached.(*cel.Ast), nil
	}

	// Compile expression
	ast, issues := env.Compile(expr)
	if issues != nil && issues.Err() != nil {
		return nil, issues.Err()
	}
	if ast.OutputType() != cel.BoolType {
		return nil, fmt.Errorf("expression must return boolean")
	}

	// Cache and return
	c.cache.Store(expr, ast)
	return ast, nil
}

// Expression represents a CEL expression validation rule
type Expression struct {
	Expression string `mapstructure:"check"`
	Message    string `mapstructure:"message"`
	Mode       string `mapstructure:"mode"` // "each" for per-item evaluation, empty for default
}

// Each returns true if this expression should be evaluated per-item
func (e Expression) Each() bool {
	return e.Mode == "each"
}

// ParseConfig converts raw YAML config to []Expression.
// Accepts either:
//   - A slice of strings (simple expressions)
//   - A slice of maps with "check" and optional "message" keys
func ParseConfig(raw any) ([]Expression, error) {
	rawSlice, ok := raw.([]any)
	if !ok {
		return nil, fmt.Errorf("checks must be an array")
	}

	exprs := make([]Expression, 0, len(rawSlice))
	for i, item := range rawSlice {
		switch v := item.(type) {
		case string:
			exprs = append(exprs, Expression{Expression: v})
		case map[string]any:
			expr := Expression{}
			if e, ok := v["check"].(string); ok {
				expr.Expression = e
			} else {
				return nil, fmt.Errorf("check[%d]: missing 'check' field", i)
			}
			if m, ok := v["message"].(string); ok {
				expr.Message = m
			}
			if m, ok := v["mode"].(string); ok {
				if m != "" && m != "each" {
					return nil, fmt.Errorf("check[%d]: invalid mode %q (must be 'each' or empty)", i, m)
				}
				expr.Mode = m
			}
			exprs = append(exprs, expr)
		default:
			return nil, fmt.Errorf("check[%d]: invalid type %T", i, item)
		}
	}
	return exprs, nil
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

// EvaluateAny compiles and evaluates a CEL expression returning its result.
// Unlike Evaluate(), this does not require boolean output - any type is allowed.
// The result is converted to native Go types for serialization.
func (c *CEL) EvaluateAny(expr string, ctx map[string]any) (any, error) {
	env, err := cel.NewEnv(append(c.variables, ext.Lists())...)
	if err != nil {
		return nil, fmt.Errorf("failed to create CEL environment: %w", err)
	}

	ast, issues := env.Compile(expr)
	if issues != nil && issues.Err() != nil {
		return nil, issues.Err()
	}

	program, err := env.Program(ast)
	if err != nil {
		return nil, err
	}

	result, _, err := program.Eval(ctx)
	if err != nil {
		return nil, err
	}

	// Convert to structpb.Value, then to native Go types for serialization
	native, err := result.ConvertToNative(reflect.TypeOf(&structpb.Value{}))
	if err != nil {
		return nil, err
	}
	return native.(*structpb.Value).AsInterface(), nil
}
