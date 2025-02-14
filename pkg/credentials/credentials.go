package credentials

import (
	"fmt"
	"reflect"
	"strings"
)

type Secret = any

type Option = func(Credentials)

type Credentials interface {
	Name() string
	GetSecret(...Option) ([]Secret, error)
	ExposeSecret(Secret, ...Option) error
	SetSecretBus(SecretBus, ...Option)
}

func WithSecretBus(bus SecretBus) Option {
	return func(m Credentials) {
		m.SetSecretBus(bus)
	}
}

func applyOpts(m Credentials, opts []Option) {
	for _, opt := range opts {
		opt(m)
	}
}

func mapToStructFields(creds Credentials, fields map[string]any) error {
	rT := reflect.ValueOf(creds)
	if rT.Kind() != reflect.Ptr {
		return fmt.Errorf("expected a pointer to a struct, got %v", rT.Kind())
	}
	fC := rT.Elem().NumField()
	for i := 0; i < fC; i++ {
		field := rT.Elem().Field(i)
		fieldName := rT.Elem().Type().Field(i).Name
		if val, ok := fields[fieldName]; ok {
			if !field.CanSet() {
				return fmt.Errorf("field '%v' cannot be set", field.String())
			}
			field.Set(reflect.ValueOf(val))
		} else {
			if val, ok = fields[strings.ToLower(fieldName)]; ok {
				if !field.CanSet() {
					return fmt.Errorf("field '%v' cannot be set", field.String())
				}
				field.Set(reflect.ValueOf(val))
			}
		}
	}
	return nil
}
