package provider

import (
	"fmt"
)

// InstanceError wraps an error with instance context (kind and name).
type InstanceError struct {
	Kind string
	Name string
	Err  error
}

func (e InstanceError) Error() string {
	return fmt.Sprintf("%s/%s: %v", e.Kind, e.Name, e.Err)
}

func (e InstanceError) Unwrap() error {
	return e.Err
}

// NewInstanceError creates a new InstanceError.
func NewInstanceError(kind, name string, err error) InstanceError {
	return InstanceError{Kind: kind, Name: name, Err: err}
}

// UnusedKeysWarning indicates unknown fields were found in spec configuration.
// This is not necessarily a hard error - the caller decides based on strict mode.
type UnusedKeysWarning struct {
	Keys []string
}

func (e *UnusedKeysWarning) Error() string {
	return fmt.Sprintf("unknown spec key(s): %v", e.Keys)
}

// ValidationResult represents the validation outcome for a single instance.
type ValidationResult struct {
	Kind   string   `json:"kind"`
	Name   string   `json:"name"`
	Valid  bool     `json:"valid"`
	Errors []string `json:"errors,omitempty"`
}

// NewValidationResult creates a ValidationResult from an Instance and optional error.
func NewValidationResult(kind, name string, err error) ValidationResult {
	result := ValidationResult{
		Kind:  kind,
		Name:  name,
		Valid: err == nil,
	}
	if err != nil {
		result.Errors = collectErrorStrings(err)
	}
	return result
}

// collectErrorStrings extracts error messages from an error, handling joined errors.
func collectErrorStrings(err error) []string {
	if err == nil {
		return nil
	}
	// Handle errors.Join result (implements Unwrap() []error)
	if unwrapped, ok := err.(interface{ Unwrap() []error }); ok {
		errs := unwrapped.Unwrap()
		result := make([]string, 0, len(errs))
		for _, e := range errs {
			result = append(result, e.Error())
		}
		return result
	}
	return []string{err.Error()}
}
