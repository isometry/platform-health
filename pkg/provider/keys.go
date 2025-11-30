package provider

// KnownComponentKeys defines valid keys at the component configuration level.
// The boolean value indicates whether the key is required (true) or optional (false).
// Key existence alone indicates validity; unknown keys will generate warnings.
var KnownComponentKeys = map[string]bool{
	"kind":       true,  // required: provider type
	"spec":       false, // optional: provider-specific configuration
	"checks":     false, // optional: CEL expressions
	"components": false, // optional: nested children (Container providers)
	"timeout":    false, // optional: per-instance timeout override
	"includes":   false, // optional: include other configuration files
}
