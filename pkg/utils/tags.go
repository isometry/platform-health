package utils

import (
	"fmt"
	"reflect"
	"strings"
)

// GetRefTagValueIfZero returns a map of field names where the specified tag value is not empty and the field value is zero.
func GetRefTagValueIfZero(tag string, s any) map[string]string {
	reflected := reflect.TypeOf(s)
	if reflected.Kind() == reflect.Ptr {
		reflected = reflected.Elem()
	}
	fieldCount := reflected.NumField()
	out := make(map[string]string, fieldCount)
	for i := 0; i < fieldCount; i++ {
		field := reflected.Field(i)
		fieldValue := reflect.ValueOf(s).Field(i)
		tagValue := strings.TrimSpace(field.Tag.Get(tag))
		if tagValue != "" && fieldValue.IsZero() {
			out[field.Name] = tagValue
		}
	}
	return out
}

// SetField sets the value of a field on a struct.
func SetField(s any, name string, value any) error {
	reflected := reflect.ValueOf(s)
	if reflected.Kind() == reflect.Ptr {
		reflected = reflected.Elem()
	} else {
		return fmt.Errorf("cannot set field on non-pointer type")
	}
	field := reflected.FieldByName(name)
	if !field.CanSet() || !field.IsValid() {
		return fmt.Errorf("field %s is not settable or invalid", name)
	}
	field.Set(reflect.ValueOf(value))
	return nil
}
