package config

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// getTestdataPath returns the absolute path to the testdata directory
func getTestdataPath() string {
	_, filename, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(filename), "testdata")
}

func TestDeepMerge(t *testing.T) {
	tests := []struct {
		name     string
		dst      map[string]any
		src      map[string]any
		expected map[string]any
	}{
		{
			name:     "Empty maps",
			dst:      map[string]any{},
			src:      map[string]any{},
			expected: map[string]any{},
		},
		{
			name:     "Merge into empty",
			dst:      map[string]any{},
			src:      map[string]any{"key": "value"},
			expected: map[string]any{"key": "value"},
		},
		{
			name:     "Scalar replacement",
			dst:      map[string]any{"key": "old"},
			src:      map[string]any{"key": "new"},
			expected: map[string]any{"key": "new"},
		},
		{
			name:     "No overlap",
			dst:      map[string]any{"a": 1},
			src:      map[string]any{"b": 2},
			expected: map[string]any{"a": 1, "b": 2},
		},
		{
			name: "Nested map merge",
			dst: map[string]any{
				"nested": map[string]any{
					"a": 1,
					"b": 2,
				},
			},
			src: map[string]any{
				"nested": map[string]any{
					"b": 3,
					"c": 4,
				},
			},
			expected: map[string]any{
				"nested": map[string]any{
					"a": 1,
					"b": 3,
					"c": 4,
				},
			},
		},
		{
			name: "List concatenation",
			dst: map[string]any{
				"list": []any{"a", "b"},
			},
			src: map[string]any{
				"list": []any{"c", "d"},
			},
			expected: map[string]any{
				"list": []any{"a", "b", "c", "d"},
			},
		},
		{
			name: "Mixed types - src wins",
			dst: map[string]any{
				"key": "string",
			},
			src: map[string]any{
				"key": map[string]any{"nested": true},
			},
			expected: map[string]any{
				"key": map[string]any{"nested": true},
			},
		},
		{
			name: "Deep nested merge",
			dst: map[string]any{
				"level1": map[string]any{
					"level2": map[string]any{
						"a": 1,
					},
				},
			},
			src: map[string]any{
				"level1": map[string]any{
					"level2": map[string]any{
						"b": 2,
					},
				},
			},
			expected: map[string]any{
				"level1": map[string]any{
					"level2": map[string]any{
						"a": 1,
						"b": 2,
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := deepMerge(tt.dst, tt.src)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestCopyWithoutKey(t *testing.T) {
	original := map[string]any{
		"keep1":  "value1",
		"remove": "value2",
		"keep2":  "value3",
	}

	result := copyWithoutKey(original, "remove")

	assert.Equal(t, 2, len(result))
	assert.Equal(t, "value1", result["keep1"])
	assert.Equal(t, "value3", result["keep2"])
	assert.NotContains(t, result, "remove")

	// Original should be unchanged
	assert.Equal(t, 3, len(original))
}

func TestIncludeStack(t *testing.T) {
	t.Run("ContainsHash", func(t *testing.T) {
		stack := IncludeStack{
			{Path: "a.yaml", Hash: "hash1"},
			{Path: "b.yaml", Hash: "hash2"},
		}

		assert.True(t, stack.ContainsHash("hash1"))
		assert.True(t, stack.ContainsHash("hash2"))
		assert.False(t, stack.ContainsHash("hash3"))
	})

	t.Run("Push", func(t *testing.T) {
		stack := IncludeStack{}
		stack = stack.Push("a.yaml", "hash1")
		stack = stack.Push("b.yaml", "hash2")

		assert.Equal(t, 2, len(stack))
		assert.Equal(t, "a.yaml", stack[0].Path)
		assert.Equal(t, "hash1", stack[0].Hash)
	})

	t.Run("CycleString", func(t *testing.T) {
		stack := IncludeStack{
			{Path: "a.yaml", Hash: "hash1"},
			{Path: "b.yaml", Hash: "hash2"},
		}

		result := stack.CycleString("c.yaml")
		assert.Equal(t, "a.yaml -> b.yaml -> c.yaml (duplicate content)", result)
	})
}

func TestComputeHash(t *testing.T) {
	t.Run("Same content same hash", func(t *testing.T) {
		content := []byte("test content")
		hash1 := computeHash(content)
		hash2 := computeHash(content)
		assert.Equal(t, hash1, hash2)
	})

	t.Run("Different content different hash", func(t *testing.T) {
		hash1 := computeHash([]byte("content1"))
		hash2 := computeHash([]byte("content2"))
		assert.NotEqual(t, hash1, hash2)
	})

	t.Run("Hash length", func(t *testing.T) {
		hash := computeHash([]byte("test"))
		assert.Equal(t, 16, len(hash)) // 8 bytes = 16 hex chars
	})
}

func TestProcessIncludes(t *testing.T) {
	t.Run("No includes", func(t *testing.T) {
		config := map[string]any{
			"type": "mock",
			"name": "test",
		}

		result, err := ProcessIncludes(config, ".", nil)
		require.NoError(t, err)
		assert.Equal(t, config, result)
	})

	t.Run("Invalid includes type", func(t *testing.T) {
		config := map[string]any{
			"includes": "not-a-list",
		}

		_, err := ProcessIncludes(config, ".", nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "includes must be a list")
	})

	t.Run("Invalid include path type", func(t *testing.T) {
		config := map[string]any{
			"includes": []any{123}, // not a string
		}

		_, err := ProcessIncludes(config, ".", nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "include path must be a string")
	})
}

func TestProcessIncludesWithFiles(t *testing.T) {
	// Create temp directory for test files
	tmpDir := t.TempDir()

	t.Run("Basic include", func(t *testing.T) {
		// Create include file
		includeContent := `
type: mock
host: included.example.com
`
		err := os.WriteFile(filepath.Join(tmpDir, "base.yaml"), []byte(includeContent), 0644)
		require.NoError(t, err)

		config := map[string]any{
			"includes": []any{"base"},
			"name":     "test",
		}

		result, err := ProcessIncludes(config, tmpDir, nil)
		require.NoError(t, err)

		assert.Equal(t, "mock", result["type"])
		assert.Equal(t, "included.example.com", result["host"])
		assert.Equal(t, "test", result["name"]) // local override
		assert.NotContains(t, result, "includes")
	})

	t.Run("Multiple includes - later overrides earlier", func(t *testing.T) {
		// Create first include
		err := os.WriteFile(filepath.Join(tmpDir, "first.yaml"), []byte(`
type: mock
value: first
shared: from-first
`), 0644)
		require.NoError(t, err)

		// Create second include
		err = os.WriteFile(filepath.Join(tmpDir, "second.yaml"), []byte(`
value: second
extra: from-second
`), 0644)
		require.NoError(t, err)

		config := map[string]any{
			"includes": []any{"first", "second"},
		}

		result, err := ProcessIncludes(config, tmpDir, nil)
		require.NoError(t, err)

		assert.Equal(t, "mock", result["type"])    // from first
		assert.Equal(t, "second", result["value"]) // second overrides first
		assert.Equal(t, "from-first", result["shared"])
		assert.Equal(t, "from-second", result["extra"])
	})

	t.Run("Local config has highest priority", func(t *testing.T) {
		err := os.WriteFile(filepath.Join(tmpDir, "included.yaml"), []byte(`
type: mock
value: from-include
`), 0644)
		require.NoError(t, err)

		config := map[string]any{
			"includes": []any{"included"},
			"value":    "local-value",
		}

		result, err := ProcessIncludes(config, tmpDir, nil)
		require.NoError(t, err)

		assert.Equal(t, "mock", result["type"])
		assert.Equal(t, "local-value", result["value"]) // local wins
	})

	t.Run("List concatenation", func(t *testing.T) {
		err := os.WriteFile(filepath.Join(tmpDir, "base-checks.yaml"), []byte(`
checks:
  - check: 'check1'
    message: "Check 1"
`), 0644)
		require.NoError(t, err)

		config := map[string]any{
			"includes": []any{"base-checks"},
			"checks": []any{
				map[string]any{"check": "check2", "message": "Check 2"},
			},
		}

		result, err := ProcessIncludes(config, tmpDir, nil)
		require.NoError(t, err)

		checks := result["checks"].([]any)
		assert.Equal(t, 2, len(checks))
	})

	t.Run("Nested includes in components", func(t *testing.T) {
		err := os.WriteFile(filepath.Join(tmpDir, "nested-include.yaml"), []byte(`
type: http
url: https://example.com
`), 0644)
		require.NoError(t, err)

		config := map[string]any{
			"components": map[string]any{
				"api": map[string]any{
					"includes": []any{"nested-include"},
					"timeout":  "5s",
				},
			},
		}

		result, err := ProcessIncludes(config, tmpDir, nil)
		require.NoError(t, err)

		components := result["components"].(map[string]any)
		api := components["api"].(map[string]any)
		assert.Equal(t, "http", api["type"])
		assert.Equal(t, "https://example.com", api["url"])
		assert.Equal(t, "5s", api["timeout"])
	})

	t.Run("Nested includes in include files", func(t *testing.T) {
		// Create nested structure
		includesDir := filepath.Join(tmpDir, "includes")
		require.NoError(t, os.MkdirAll(includesDir, 0755))

		err := os.WriteFile(filepath.Join(includesDir, "leaf.yaml"), []byte(`
leaf: true
`), 0644)
		require.NoError(t, err)

		err = os.WriteFile(filepath.Join(includesDir, "parent.yaml"), []byte(`
includes:
  - leaf
parent: true
`), 0644)
		require.NoError(t, err)

		config := map[string]any{
			"includes": []any{"includes/parent"},
		}

		result, err := ProcessIncludes(config, tmpDir, nil)
		require.NoError(t, err)

		assert.Equal(t, true, result["leaf"])
		assert.Equal(t, true, result["parent"])
	})

	t.Run("Missing include file", func(t *testing.T) {
		config := map[string]any{
			"includes": []any{"nonexistent"},
		}

		_, err := ProcessIncludes(config, tmpDir, nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to load include")
	})
}

func TestProcessIncludesLoopDetection(t *testing.T) {
	tmpDir := t.TempDir()

	t.Run("Direct self-reference", func(t *testing.T) {
		// File that includes itself
		err := os.WriteFile(filepath.Join(tmpDir, "self.yaml"), []byte(`
includes:
  - self
type: mock
`), 0644)
		require.NoError(t, err)

		config := map[string]any{
			"includes": []any{"self"},
		}

		_, err = ProcessIncludes(config, tmpDir, nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "include loop detected")
	})

	t.Run("Indirect loop A->B->A", func(t *testing.T) {
		err := os.WriteFile(filepath.Join(tmpDir, "a.yaml"), []byte(`
includes:
  - b
from: a
`), 0644)
		require.NoError(t, err)

		err = os.WriteFile(filepath.Join(tmpDir, "b.yaml"), []byte(`
includes:
  - a
from: b
`), 0644)
		require.NoError(t, err)

		config := map[string]any{
			"includes": []any{"a"},
		}

		_, err = ProcessIncludes(config, tmpDir, nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "include loop detected")
	})

	t.Run("Duplicate content creates loop via recursion", func(t *testing.T) {
		// Two files with identical content that includes "parent"
		// This creates a loop because the content recursively includes an ancestor
		loopContent := `
includes:
  - parent
type: mock
`
		err := os.WriteFile(filepath.Join(tmpDir, "child1.yaml"), []byte(loopContent), 0644)
		require.NoError(t, err)

		err = os.WriteFile(filepath.Join(tmpDir, "child2.yaml"), []byte(loopContent), 0644)
		require.NoError(t, err)

		// Parent includes child1 which has same content as child2
		err = os.WriteFile(filepath.Join(tmpDir, "parent.yaml"), []byte(`
includes:
  - child1
parent: true
`), 0644)
		require.NoError(t, err)

		config := map[string]any{
			"includes": []any{"parent"},
		}

		// This creates a loop: parent -> child1 -> parent (via identical content)
		_, err = ProcessIncludes(config, tmpDir, nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "include loop detected")
	})

	t.Run("Sibling includes with identical content allowed", func(t *testing.T) {
		// Two files with identical content but no includes - not a loop, just redundant
		content := `type: mock
value: same
`
		err := os.WriteFile(filepath.Join(tmpDir, "twin1.yaml"), []byte(content), 0644)
		require.NoError(t, err)

		err = os.WriteFile(filepath.Join(tmpDir, "twin2.yaml"), []byte(content), 0644)
		require.NoError(t, err)

		config := map[string]any{
			"includes": []any{"twin1", "twin2"},
		}

		// Sibling includes with identical content are allowed (just redundant)
		result, err := ProcessIncludes(config, tmpDir, nil)
		require.NoError(t, err)
		assert.Equal(t, "mock", result["type"])
	})

	t.Run("Diamond pattern allowed", func(t *testing.T) {
		// Diamond: main includes both A and B, both include shared
		// This is allowed because each path to "shared" is independent
		sharedContent := `shared: true
`
		err := os.WriteFile(filepath.Join(tmpDir, "shared.yaml"), []byte(sharedContent), 0644)
		require.NoError(t, err)

		err = os.WriteFile(filepath.Join(tmpDir, "branch-a.yaml"), []byte(`
includes:
  - shared
branch: a
`), 0644)
		require.NoError(t, err)

		err = os.WriteFile(filepath.Join(tmpDir, "branch-b.yaml"), []byte(`
includes:
  - shared
branch: b
`), 0644)
		require.NoError(t, err)

		// Main config includes both branches (diamond pattern)
		config := map[string]any{
			"components": map[string]any{
				"comp-a": map[string]any{
					"includes": []any{"branch-a"},
				},
				"comp-b": map[string]any{
					"includes": []any{"branch-b"},
				},
			},
		}

		// Diamond pattern should work - each component's include chain is independent
		result, err := ProcessIncludes(config, tmpDir, nil)
		require.NoError(t, err)

		components := result["components"].(map[string]any)
		compA := components["comp-a"].(map[string]any)
		compB := components["comp-b"].(map[string]any)

		assert.Equal(t, true, compA["shared"])
		assert.Equal(t, "a", compA["branch"])
		assert.Equal(t, true, compB["shared"])
		assert.Equal(t, "b", compB["branch"])
	})
}

func TestProcessIncludesRelativePaths(t *testing.T) {
	tmpDir := t.TempDir()

	// Create directory structure:
	// tmpDir/
	//   main.yaml
	//   includes/
	//     base.yaml
	//     shared/
	//       common.yaml

	includesDir := filepath.Join(tmpDir, "includes")
	sharedDir := filepath.Join(includesDir, "shared")
	require.NoError(t, os.MkdirAll(sharedDir, 0755))

	// common.yaml in shared/
	err := os.WriteFile(filepath.Join(sharedDir, "common.yaml"), []byte(`
common: true
`), 0644)
	require.NoError(t, err)

	// base.yaml includes ../shared/common (relative to includes/)
	err = os.WriteFile(filepath.Join(includesDir, "base.yaml"), []byte(`
includes:
  - shared/common
base: true
`), 0644)
	require.NoError(t, err)

	t.Run("Relative path resolution from include file", func(t *testing.T) {
		// Main config includes includes/base
		config := map[string]any{
			"includes": []any{"includes/base"},
		}

		result, err := ProcessIncludes(config, tmpDir, nil)
		require.NoError(t, err)

		assert.Equal(t, true, result["common"])
		assert.Equal(t, true, result["base"])
	})
}

// TestProcessIncludesFixtures uses the testdata fixtures for integration testing
func TestProcessIncludesFixtures(t *testing.T) {
	testdataPath := getTestdataPath()

	t.Run("FluxCD system include", func(t *testing.T) {
		config := map[string]any{
			"type":     "system",
			"includes": []any{"includes/fluxcd"},
			"components": map[string]any{
				"helm-controller": map[string]any{
					"type":   "mock",
					"health": 0,
				},
			},
		}

		result, err := ProcessIncludes(config, testdataPath, nil)
		require.NoError(t, err)

		// Should have merged the system type
		assert.Equal(t, "system", result["type"])

		// Should have all components merged
		components := result["components"].(map[string]any)
		assert.Contains(t, components, "source-controller")
		assert.Contains(t, components, "kustomize-controller")
		assert.Contains(t, components, "helm-controller")
	})

	t.Run("Base checks list concatenation", func(t *testing.T) {
		config := map[string]any{
			"type":     "tls",
			"host":     "example.com",
			"includes": []any{"includes/base-checks"},
			"checks": []any{
				map[string]any{
					"check":   "tls.protocol == \"h2\"",
					"message": "HTTP/2 required",
				},
			},
		}

		result, err := ProcessIncludes(config, testdataPath, nil)
		require.NoError(t, err)

		// Should have both checks concatenated
		checks := result["checks"].([]any)
		assert.Equal(t, 2, len(checks))

		// First check should be from include
		check1 := checks[0].(map[string]any)
		assert.Contains(t, check1["check"], "TLS 1.3")

		// Second check should be from local
		check2 := checks[1].(map[string]any)
		assert.Contains(t, check2["check"], "h2")
	})
}
