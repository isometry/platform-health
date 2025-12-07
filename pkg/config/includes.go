package config

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/spf13/viper"
)

// IncludeEntry tracks both path (for error messages) and content hash (for loop detection)
type IncludeEntry struct {
	Path string
	Hash string
}

// IncludeStack tracks the chain of includes for loop detection
type IncludeStack []IncludeEntry

// computeHash returns a fast hash of the file contents
func computeHash(content []byte) string {
	h := sha256.Sum256(content)
	return hex.EncodeToString(h[:8]) // First 8 bytes (16 hex chars) is sufficient
}

// ContainsHash checks if a content hash is already in the stack
func (s IncludeStack) ContainsHash(hash string) bool {
	return slices.ContainsFunc(s, func(e IncludeEntry) bool {
		return e.Hash == hash
	})
}

// Push adds a new entry to the stack
func (s IncludeStack) Push(path, hash string) IncludeStack {
	return append(s, IncludeEntry{Path: path, Hash: hash})
}

// CycleString formats the include chain for error messages
func (s IncludeStack) CycleString(cyclePath string) string {
	parts := make([]string, len(s)+1)
	for i, entry := range s {
		parts[i] = entry.Path
	}
	parts[len(s)] = cyclePath + " (duplicate content)"
	return strings.Join(parts, " -> ")
}

// ProcessIncludes recursively processes includes in a config map.
// basePath is the directory containing the current config file.
// stack is used for loop detection via content hashing.
func ProcessIncludes(configMap map[string]any, basePath string, stack IncludeStack) (map[string]any, error) {
	// Extract includes list
	includesRaw, hasIncludes := configMap["includes"]
	if !hasIncludes {
		// Still need to process nested maps for includes
		return processNestedIncludes(configMap, basePath, stack)
	}

	includes, ok := includesRaw.([]any)
	if !ok {
		return nil, fmt.Errorf("includes must be a list")
	}

	// Start with empty result
	result := make(map[string]any)

	// Process each include in order
	for _, inc := range includes {
		includePath, ok := inc.(string)
		if !ok {
			return nil, fmt.Errorf("include path must be a string")
		}

		// Resolve relative path
		fullPath := filepath.Join(basePath, includePath)

		// Load include file
		includeConfig, newBasePath, hash, err := loadIncludeFile(fullPath)
		if err != nil {
			return nil, fmt.Errorf("failed to load include %s: %w", includePath, err)
		}

		// Check for loops using content hash
		if stack.ContainsHash(hash) {
			return nil, fmt.Errorf("include loop detected: %s", stack.CycleString(includePath))
		}

		// Recursively process includes in the loaded file
		processed, err := ProcessIncludes(includeConfig, newBasePath, stack.Push(includePath, hash))
		if err != nil {
			return nil, err
		}

		// Merge into result
		result = deepMerge(result, processed)
	}

	// Remove includes key from local config before merging
	localConfig := copyWithoutKey(configMap, "includes")

	// Process nested includes in local config
	localConfig, err := processNestedIncludes(localConfig, basePath, stack)
	if err != nil {
		return nil, err
	}

	// Merge local config last (highest priority)
	result = deepMerge(result, localConfig)

	return result, nil
}

// loadIncludeFile loads a config file using Viper's suffix detection
// Returns the config map, the directory containing the file, and a content hash
func loadIncludeFile(pathWithoutExt string) (map[string]any, string, string, error) {
	// Use :: key delimiter to allow dots in component names (e.g., google.com)
	v := viper.NewWithOptions(viper.KeyDelimiter("::"))
	dir := filepath.Dir(pathWithoutExt)
	name := filepath.Base(pathWithoutExt)

	v.AddConfigPath(dir)
	v.SetConfigName(name)

	if err := v.ReadInConfig(); err != nil {
		return nil, "", "", err
	}

	// Read raw file for hashing
	configFile := v.ConfigFileUsed()
	content, err := os.ReadFile(configFile)
	if err != nil {
		return nil, "", "", err
	}
	hash := computeHash(content)

	var result map[string]any
	if err := v.Unmarshal(&result); err != nil {
		return nil, "", "", err
	}

	return result, dir, hash, nil
}

// deepMerge recursively merges src into dst
// - Maps are recursively merged
// - Lists are concatenated
// - Scalars are replaced
func deepMerge(dst, src map[string]any) map[string]any {
	result := make(map[string]any)

	// Copy dst
	maps.Copy(result, dst)

	// Merge src
	for k, v := range src {
		existing := result[k]

		switch srcVal := v.(type) {
		case map[string]any:
			// Maps: recursive merge
			if dstMap, ok := existing.(map[string]any); ok {
				result[k] = deepMerge(dstMap, srcVal)
			} else {
				result[k] = srcVal
			}
		case []any:
			// Lists: concatenate
			if dstList, ok := existing.([]any); ok {
				result[k] = append(dstList, srcVal...)
			} else {
				result[k] = srcVal
			}
		default:
			// Scalars: replace
			result[k] = v
		}
	}

	return result
}

// processNestedIncludes processes includes in nested component maps
func processNestedIncludes(configMap map[string]any, basePath string, stack IncludeStack) (map[string]any, error) {
	result := make(map[string]any)

	for k, v := range configMap {
		if nestedMap, isMap := v.(map[string]any); isMap {
			processed, err := ProcessIncludes(nestedMap, basePath, stack)
			if err != nil {
				return nil, fmt.Errorf("in %s: %w", k, err)
			}
			result[k] = processed
		} else {
			result[k] = v
		}
	}

	return result, nil
}

// copyWithoutKey creates a shallow copy of the map without the specified key
func copyWithoutKey(m map[string]any, key string) map[string]any {
	result := maps.Clone(m)
	delete(result, key)
	return result
}
