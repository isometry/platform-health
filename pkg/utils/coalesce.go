package utils

import "reflect"

func CoalesceZero[R any](values ...any) R {
	if values == nil || len(values) == 0 {
		return any(nil).(R)
	}
	for _, value := range values {
		if value != nil && reflect.Zero(reflect.TypeOf(value)).Interface() != value {
			return value.(R)
		}
	}
	return any(nil).(R)
}
