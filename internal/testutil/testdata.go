package testutil

import (
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestdataPath returns the absolute path to the testdata directory
// adjacent to the caller's source file.
func TestdataPath(t *testing.T) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(1)
	require.True(t, ok, "failed to get caller info")
	return filepath.Join(filepath.Dir(filename), "testdata")
}
