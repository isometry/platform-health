package provider

import (
	"fmt"
)

// InstanceError wraps an error with instance context (type and name).
type InstanceError struct {
	Type string
	Name string
	Err  error
}

func (e InstanceError) Error() string {
	return fmt.Sprintf("%s/%s: %v", e.Type, e.Name, e.Err)
}

func (e InstanceError) Unwrap() error {
	return e.Err
}

// NewInstanceError creates a new InstanceError.
func NewInstanceError(providerType, name string, err error) InstanceError {
	return InstanceError{Type: providerType, Name: name, Err: err}
}

// UnusedKeysWarning indicates unknown fields were found in spec configuration.
// This is not necessarily a hard error - the caller decides based on strict mode.
type UnusedKeysWarning struct {
	Keys []string
}

func (e *UnusedKeysWarning) Error() string {
	return fmt.Sprintf("unknown spec key(s): %v", e.Keys)
}
