package client

// GetString safely extracts a string from a map.
// Returns empty string if key doesn't exist or value is not a string.
func GetString(m map[string]any, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}
