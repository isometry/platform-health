package rest

import (
	"fmt"

	"github.com/google/cel-go/cel"
)

// ValidateCELExpression validates CEL expression syntax at configuration time
func ValidateCELExpression(expression string) error {
	env, err := cel.NewEnv(
		cel.Variable("response", cel.MapType(cel.StringType, cel.DynType)),
	)
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
