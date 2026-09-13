//go:build ui

package ui_test

import (
	"os/exec"
	"testing"
)

// TestAppJS runs the node unit tests for app.js's pure functions.
func TestAppJS(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not found on PATH")
	}
	cmd := exec.Command(node, "--test", "testdata/app_test.mjs")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("node --test failed: %v\n%s", err, out)
	}
}
