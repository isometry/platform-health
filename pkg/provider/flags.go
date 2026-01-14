package provider

import (
	"cmp"
	"errors"
	"fmt"
	"log/slog"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/pflag"
)

// FlagValue represents a single flag definition with metadata.
type FlagValue struct {
	Shorthand    string
	Kind         string
	DefaultValue any
	NoOptDefault string
	Usage        string
}

// FlagValues is a map of flag names to their definitions.
type FlagValues map[string]FlagValue

// Register adds all flags in the set to the given pflag.FlagSet.
func (f FlagValues) Register(flagSet *pflag.FlagSet, sort bool) {
	for flagName, flag := range f {
		flag.BuildFlag(flagSet, flagName)
	}
	flagSet.SortFlags = sort
}

// BuildFlag creates a pflag from the FlagValue definition.
func (f *FlagValue) BuildFlag(flagSet *pflag.FlagSet, flagName string) {
	switch f.Kind {
	case "bool":
		defaultVal := false
		if f.DefaultValue != nil {
			defaultVal = f.DefaultValue.(bool)
		}
		flagSet.BoolP(flagName, f.Shorthand, defaultVal, f.Usage)
	case "count":
		flagSet.CountP(flagName, f.Shorthand, f.Usage)
	case "int":
		defaultVal := 0
		if f.DefaultValue != nil {
			defaultVal = f.DefaultValue.(int)
		}
		flagSet.IntP(flagName, f.Shorthand, defaultVal, f.Usage)
	case "string":
		defaultVal := ""
		if f.DefaultValue != nil {
			defaultVal = f.DefaultValue.(string)
		}
		flagSet.StringP(flagName, f.Shorthand, defaultVal, f.Usage)
	case "stringSlice":
		var defaultVal []string
		if f.DefaultValue != nil {
			defaultVal = f.DefaultValue.([]string)
		}
		flagSet.StringSliceP(flagName, f.Shorthand, defaultVal, f.Usage)
	case "intSlice":
		var defaultVal []int
		if f.DefaultValue != nil {
			defaultVal = f.DefaultValue.([]int)
		}
		flagSet.IntSliceP(flagName, f.Shorthand, defaultVal, f.Usage)
	case "duration":
		var defaultVal time.Duration
		if f.DefaultValue != nil {
			switch v := f.DefaultValue.(type) {
			case time.Duration:
				defaultVal = v
			case string:
				var err error
				defaultVal, err = time.ParseDuration(v)
				if err != nil {
					slog.Warn("invalid duration default value", "flag", flagName, "value", v, "error", err)
				}
			}
		}
		flagSet.DurationP(flagName, f.Shorthand, defaultVal, f.Usage)
	}

	if f.NoOptDefault != "" {
		flag := flagSet.Lookup(flagName)
		flag.NoOptDefVal = f.NoOptDefault
	}
}

// ProviderFlags derives flag definitions from a provider instance using reflection.
// It reads struct tags to determine flag names, types, defaults, and descriptions:
//   - mapstructure: flag name (skip if "-" or empty)
//   - default: default value
//   - description: custom usage text (auto-generated if not provided)
//   - flag:"[name],[option]": name override (optional), option is "inline" or "nested"
//   - flag:",inline": for struct fields, flatten without prefix (kind instead of resource.kind)
//   - flag:",nested": for struct fields, flatten with prefix (resource.kind)
func ProviderFlags(instance Instance) FlagValues {
	result := make(FlagValues)
	providerType := instance.GetType()

	val := reflect.ValueOf(instance)
	if val.Kind() == reflect.Pointer {
		val = val.Elem()
	}
	if val.Kind() != reflect.Struct {
		return result
	}

	deriveFlags(val, "", providerType, result)
	return result
}

// deriveFlags recursively extracts flag definitions from struct fields.
func deriveFlags(val reflect.Value, prefix string, providerType string, result FlagValues) {
	typ := val.Type()
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)

		// Skip unexported fields
		if !field.IsExported() {
			continue
		}

		// Get flag name from mapstructure tag
		flagName := field.Tag.Get("mapstructure")
		if flagName == "" || flagName == "-" || flagName == ",squash" {
			continue
		}

		// Parse flag tag: "name,option" where name overrides flag name, option is "inline" or "nested"
		flagTag := field.Tag.Get("flag")
		var flagOption string
		if parts := strings.Split(flagTag, ","); len(parts) >= 1 {
			flagName = cmp.Or(parts[0], flagName) // Use flag name override if provided
			if len(parts) >= 2 {
				flagOption = parts[1]
			}
		}

		// Check if this is a struct that should be inlined or nested
		if field.Type.Kind() == reflect.Struct && (flagOption == "inline" || flagOption == "nested") {
			var newPrefix string
			if flagOption == "nested" {
				if prefix != "" {
					newPrefix = prefix + "." + flagName
				} else {
					newPrefix = flagName
				}
			} else {
				newPrefix = prefix
			}
			deriveFlags(val.Field(i), newPrefix, providerType, result)
			continue
		}

		// Add prefix for nested fields
		if prefix != "" {
			flagName = prefix + "." + flagName
		}

		// Determine flag kind from Go type
		kind, ok := goTypeToFlagKind(field.Type)
		if !ok {
			continue // Skip unsupported types
		}

		// Get default value
		defaultValue := parseDefaultValue(field.Tag.Get("default"), field.Type)

		// Get description (auto-generate if not provided)
		description := field.Tag.Get("description")
		if description == "" {
			description = fmt.Sprintf("set %s %s", providerType, flagName)
		}

		result[flagName] = FlagValue{
			Kind:         kind,
			DefaultValue: defaultValue,
			Usage:        description,
		}
	}
}

// ConfigureFromFlags applies flag values from a pflag.FlagSet to a provider instance.
// It uses reflection to map flags to struct fields based on mapstructure tags.
func ConfigureFromFlags(instance Instance, fs *pflag.FlagSet) error {
	val := reflect.ValueOf(instance)
	if val.Kind() == reflect.Pointer {
		val = val.Elem()
	}
	if val.Kind() != reflect.Struct {
		return fmt.Errorf("instance must be a struct pointer")
	}

	var errs []error
	configureFields(val, "", fs, &errs)

	if len(errs) > 0 {
		return fmt.Errorf("flag errors: %w", errors.Join(errs...))
	}
	return nil
}

// configureFields recursively configures struct fields from flags.
func configureFields(val reflect.Value, prefix string, fs *pflag.FlagSet, errs *[]error) {
	typ := val.Type()

	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)

		// Skip unexported fields
		if !field.IsExported() {
			continue
		}

		// Get flag name from mapstructure tag
		flagName := field.Tag.Get("mapstructure")
		if flagName == "" || flagName == "-" || flagName == ",squash" {
			continue
		}

		// Get the field value
		fieldVal := val.Field(i)
		if !fieldVal.CanSet() {
			continue
		}

		// Parse flag tag: "name,option" where name overrides flag name, option is "inline" or "nested"
		flagTag := field.Tag.Get("flag")
		var flagOption string
		if parts := strings.Split(flagTag, ","); len(parts) >= 1 {
			flagName = cmp.Or(parts[0], flagName) // Use flag name override if provided
			if len(parts) >= 2 {
				flagOption = parts[1]
			}
		}

		// Check if this is a struct that should be inlined or nested
		if field.Type.Kind() == reflect.Struct && (flagOption == "inline" || flagOption == "nested") {
			var newPrefix string
			if flagOption == "nested" {
				if prefix != "" {
					newPrefix = prefix + "." + flagName
				} else {
					newPrefix = flagName
				}
			} else {
				newPrefix = prefix
			}
			configureFields(fieldVal, newPrefix, fs, errs)
			continue
		}

		// Add prefix for nested fields
		if prefix != "" {
			flagName = prefix + "." + flagName
		}

		// Read from pflag and set field
		if err := setFieldFromFlag(fieldVal, field.Type, fs, flagName); err != nil {
			*errs = append(*errs, err)
		}
	}
}

// goTypeToFlagKind maps Go types to pflag kind strings.
func goTypeToFlagKind(t reflect.Type) (string, bool) {
	switch t.Kind() {
	case reflect.Pointer:
		// Unwrap pointer to get underlying type's flag kind
		return goTypeToFlagKind(t.Elem())
	case reflect.String:
		return "string", true
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if t == reflect.TypeFor[time.Duration]() {
			return "duration", true
		}
		// Check if this is a protobuf enum (int32 with String() method)
		if isProtobufEnum(t) {
			return "string", true
		}
		return "int", true
	case reflect.Bool:
		return "bool", true
	case reflect.Slice:
		switch t.Elem().Kind() {
		case reflect.String:
			return "stringSlice", true
		case reflect.Int:
			return "intSlice", true
		}
	}
	return "", false
}

// isProtobufEnum checks if a type is a protobuf enum (int32-based with String method)
func isProtobufEnum(t reflect.Type) bool {
	// Must be int32-based
	if t.Kind() != reflect.Int32 {
		return false
	}
	// Must have String() method (implements fmt.Stringer)
	_, hasString := t.MethodByName("String")
	return hasString
}

// enumFromString converts a string to a protobuf enum int32 value.
// It iterates through possible enum values and matches by String() representation.
func enumFromString(t reflect.Type, s string) (int32, error) {
	s = strings.ToUpper(s)

	// Try values 0-100 (more than enough for any protobuf enum)
	for i := range int32(100) {
		// Create a value of the enum type with this int32
		v := reflect.New(t).Elem()
		v.SetInt(int64(i))

		// Call String() method
		stringMethod := v.MethodByName("String")
		if !stringMethod.IsValid() {
			break
		}

		result := stringMethod.Call(nil)
		if len(result) != 1 {
			break
		}

		name := result[0].String()
		if strings.ToUpper(name) == s {
			return i, nil
		}
	}

	return 0, fmt.Errorf("unknown enum value: %s", s)
}

// parseDefaultValue parses a default tag value into the appropriate Go type.
func parseDefaultValue(defaultStr string, t reflect.Type) any {
	if defaultStr == "" {
		return nil
	}

	switch t.Kind() {
	case reflect.Pointer:
		// Parse as underlying type (the flag system handles pointers transparently)
		return parseDefaultValue(defaultStr, t.Elem())
	case reflect.String:
		return defaultStr
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if t == reflect.TypeFor[time.Duration]() {
			return defaultStr // Duration flags accept string defaults
		}
		// Protobuf enums use string defaults (e.g., "HEALTHY")
		if isProtobufEnum(t) {
			return defaultStr
		}
		if v, err := strconv.Atoi(defaultStr); err == nil {
			return v
		}
	case reflect.Bool:
		return defaultStr == "true"
	case reflect.Slice:
		// Handle slice defaults like "[200]" or "[val1,val2]"
		trimmed := strings.Trim(defaultStr, "[]")
		if trimmed == "" {
			return nil
		}
		switch t.Elem().Kind() {
		case reflect.String:
			return strings.Split(trimmed, ",")
		case reflect.Int:
			parts := strings.Split(trimmed, ",")
			result := make([]int, 0, len(parts))
			for _, p := range parts {
				if v, err := strconv.Atoi(strings.TrimSpace(p)); err == nil {
					result = append(result, v)
				}
			}
			return result
		}
	}
	return nil
}

// setFieldFromFlag reads a flag value and sets it on the struct field.
func setFieldFromFlag(fieldVal reflect.Value, fieldType reflect.Type, fs *pflag.FlagSet, flagName string) error {
	switch fieldType.Kind() {
	case reflect.Pointer:
		// Create new value of the underlying type
		elemType := fieldType.Elem()
		newVal := reflect.New(elemType)

		// Recursively set the underlying value from the flag
		if err := setFieldFromFlag(newVal.Elem(), elemType, fs, flagName); err != nil {
			return err
		}

		// Set the pointer field to point to the new value
		fieldVal.Set(newVal)
	case reflect.String:
		v, err := fs.GetString(flagName)
		if err != nil {
			return err
		}
		fieldVal.SetString(v)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if fieldType == reflect.TypeFor[time.Duration]() {
			v, err := fs.GetDuration(flagName)
			if err != nil {
				return err
			}
			fieldVal.SetInt(int64(v))
		} else if isProtobufEnum(fieldType) {
			// Handle protobuf enum: get string and convert to int32
			v, err := fs.GetString(flagName)
			if err != nil {
				return err
			}
			enumVal, err := enumFromString(fieldType, v)
			if err != nil {
				return err
			}
			fieldVal.SetInt(int64(enumVal))
		} else {
			v, err := fs.GetInt(flagName)
			if err != nil {
				return err
			}
			fieldVal.SetInt(int64(v))
		}
	case reflect.Bool:
		v, err := fs.GetBool(flagName)
		if err != nil {
			return err
		}
		fieldVal.SetBool(v)
	case reflect.Slice:
		switch fieldType.Elem().Kind() {
		case reflect.String:
			v, err := fs.GetStringSlice(flagName)
			if err != nil {
				return err
			}
			fieldVal.Set(reflect.ValueOf(v))
		case reflect.Int:
			v, err := fs.GetIntSlice(flagName)
			if err != nil {
				return err
			}
			fieldVal.Set(reflect.ValueOf(v))
		}
	}
	return nil
}
