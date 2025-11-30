// Package functions provides custom CEL functions for health check expressions.
package functions

import "github.com/google/cel-go/cel"

// All returns all custom CEL functions.
func All() []cel.EnvOption {
	return Time()
}
