package output

import (
	"encoding/xml"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	ph "github.com/isometry/platform-health/pkg/platform_health"
	"github.com/isometry/platform-health/pkg/platform_health/details"
)

func TestJUnitFormatter_SingleHealthy(t *testing.T) {
	resp := &ph.HealthCheckResponse{
		Name:     "test",
		Type:     "http",
		Status:   ph.Status_HEALTHY,
		Duration: durationpb.New(100000000), // 0.1s
	}

	formatter, _ := GetFormatter("junit")
	output, err := formatter.Format(resp, Config{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Parse the XML to verify structure
	var suite junitTestSuite
	if err := xml.Unmarshal(output, &suite); err != nil {
		t.Fatalf("failed to parse XML: %v", err)
	}

	if suite.Name != "test" {
		t.Errorf("expected suite name 'test', got %q", suite.Name)
	}
	if suite.Tests != 1 {
		t.Errorf("expected 1 test, got %d", suite.Tests)
	}
	if suite.Failures != 0 {
		t.Errorf("expected 0 failures, got %d", suite.Failures)
	}
	if len(suite.Cases) != 1 {
		t.Errorf("expected 1 testcase, got %d", len(suite.Cases))
	}
	if len(suite.Cases[0].Failures) != 0 {
		t.Error("expected no failure element for healthy check")
	}
}

func TestJUnitFormatter_SingleUnhealthy(t *testing.T) {
	resp := &ph.HealthCheckResponse{
		Name:     "test",
		Type:     "tcp",
		Status:   ph.Status_UNHEALTHY,
		Messages: []string{"connection refused"},
		Duration: durationpb.New(50000000), // 0.05s
	}

	formatter, _ := GetFormatter("junit")
	output, err := formatter.Format(resp, Config{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var suite junitTestSuite
	if err := xml.Unmarshal(output, &suite); err != nil {
		t.Fatalf("failed to parse XML: %v", err)
	}

	if suite.Failures != 1 {
		t.Errorf("expected 1 failure, got %d", suite.Failures)
	}
	if len(suite.Cases[0].Failures) == 0 {
		t.Fatal("expected failure element for unhealthy check")
	}
	if suite.Cases[0].Failures[0].Message != "connection refused" {
		t.Errorf("expected failure message 'connection refused', got %q", suite.Cases[0].Failures[0].Message)
	}
}

func TestJUnitFormatter_SingleUnknown(t *testing.T) {
	resp := &ph.HealthCheckResponse{
		Name:   "test",
		Type:   "vault",
		Status: ph.Status_UNKNOWN,
	}

	formatter, _ := GetFormatter("junit")
	output, err := formatter.Format(resp, Config{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var suite junitTestSuite
	if err := xml.Unmarshal(output, &suite); err != nil {
		t.Fatalf("failed to parse XML: %v", err)
	}

	if suite.Skipped != 1 {
		t.Errorf("expected 1 skipped, got %d", suite.Skipped)
	}
	if suite.Cases[0].Skipped == nil {
		t.Fatal("expected skipped element for unknown status")
	}
}

func TestJUnitFormatter_Nested(t *testing.T) {
	resp := &ph.HealthCheckResponse{
		Name:   "root",
		Type:   "system",
		Status: ph.Status_HEALTHY,
		Components: []*ph.HealthCheckResponse{
			{
				Name:   "group1",
				Type:   "system",
				Status: ph.Status_HEALTHY,
				Components: []*ph.HealthCheckResponse{
					{
						Name:     "check1",
						Type:     "http",
						Status:   ph.Status_HEALTHY,
						Duration: durationpb.New(100000000),
					},
					{
						Name:     "check2",
						Type:     "tcp",
						Status:   ph.Status_UNHEALTHY,
						Messages: []string{"timeout"},
						Duration: durationpb.New(200000000),
					},
				},
			},
			{
				Name:     "standalone",
				Type:     "dns",
				Status:   ph.Status_HEALTHY,
				Duration: durationpb.New(50000000),
			},
		},
	}

	formatter, _ := GetFormatter("junit")
	output, err := formatter.Format(resp, Config{Flat: false})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var suite junitTestSuite
	if err := xml.Unmarshal(output, &suite); err != nil {
		t.Fatalf("failed to parse XML: %v", err)
	}

	// Should have 3 total tests (check1, check2, standalone)
	if suite.Tests != 3 {
		t.Errorf("expected 3 tests, got %d", suite.Tests)
	}
	// Should have 1 failure (check2)
	if suite.Failures != 1 {
		t.Errorf("expected 1 failure, got %d", suite.Failures)
	}
	// Should have 1 nested testsuite (group1) and 1 testcase (standalone)
	if len(suite.Suites) != 1 {
		t.Errorf("expected 1 nested suite, got %d", len(suite.Suites))
	}
	if len(suite.Cases) != 1 {
		t.Errorf("expected 1 direct testcase, got %d", len(suite.Cases))
	}
}

func TestJUnitFormatter_Flat(t *testing.T) {
	resp := &ph.HealthCheckResponse{
		Name:   "root",
		Type:   "system",
		Status: ph.Status_HEALTHY,
		Components: []*ph.HealthCheckResponse{
			{
				Name:   "group1",
				Type:   "system",
				Status: ph.Status_HEALTHY,
				Components: []*ph.HealthCheckResponse{
					{
						Name:     "check1",
						Type:     "http",
						Status:   ph.Status_HEALTHY,
						Duration: durationpb.New(100000000),
					},
				},
			},
		},
	}

	formatter, _ := GetFormatter("junit")
	output, err := formatter.Format(resp, Config{Flat: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var suite junitTestSuite
	if err := xml.Unmarshal(output, &suite); err != nil {
		t.Fatalf("failed to parse XML: %v", err)
	}

	// In flat mode, no nested suites
	if len(suite.Suites) != 0 {
		t.Errorf("expected 0 nested suites in flat mode, got %d", len(suite.Suites))
	}
	// All should be testcases with path names
	if len(suite.Cases) == 0 {
		t.Error("expected testcases in flat mode")
	}

	// Check that paths are used
	foundPath := false
	for _, tc := range suite.Cases {
		if strings.Contains(tc.Name, "/") {
			foundPath = true
			break
		}
	}
	if !foundPath {
		t.Error("expected path-based names in flat mode")
	}
}

func TestJUnitFormatter_Duration(t *testing.T) {
	resp := &ph.HealthCheckResponse{
		Name:     "test",
		Type:     "http",
		Status:   ph.Status_HEALTHY,
		Duration: durationpb.New(1500000000), // 1.5s
	}

	formatter, _ := GetFormatter("junit")
	output, err := formatter.Format(resp, Config{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var suite junitTestSuite
	if err := xml.Unmarshal(output, &suite); err != nil {
		t.Fatalf("failed to parse XML: %v", err)
	}

	// Time should be approximately 1.5
	if suite.Cases[0].Time < 1.4 || suite.Cases[0].Time > 1.6 {
		t.Errorf("expected time around 1.5, got %f", suite.Cases[0].Time)
	}
}

func TestJUnitFormatter_XMLEscaping(t *testing.T) {
	resp := &ph.HealthCheckResponse{
		Name:     "test<>",
		Type:     "http",
		Status:   ph.Status_UNHEALTHY,
		Messages: []string{`error: "bad" & <invalid>`},
	}

	formatter, _ := GetFormatter("junit")
	output, err := formatter.Format(resp, Config{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should be valid XML (no parse error)
	var suite junitTestSuite
	if err := xml.Unmarshal(output, &suite); err != nil {
		t.Fatalf("failed to parse XML with special characters: %v", err)
	}

	// Message should be preserved (after XML encoding/decoding)
	if suite.Cases[0].Failures[0].Message != `error: "bad" & <invalid>` {
		t.Errorf("message not preserved correctly: %s", suite.Cases[0].Failures[0].Message)
	}
}

// JUnit Details Integration Tests

func TestJUnitFormatter_HealthyWithTLSDetails(t *testing.T) {
	validUntil := time.Now().Add(30 * 24 * time.Hour)
	tlsDetail := &details.Detail_TLS{
		CommonName:  "example.com",
		ValidUntil:  timestamppb.New(validUntil),
		Version:     "TLS 1.3",
		CipherSuite: "TLS_AES_256_GCM_SHA384",
	}
	anyDetail, err := anypb.New(tlsDetail)
	if err != nil {
		t.Fatalf("failed to create any: %v", err)
	}

	resp := &ph.HealthCheckResponse{
		Name:     "tls-check",
		Type:     "tls",
		Status:   ph.Status_HEALTHY,
		Details:  []*anypb.Any{anyDetail},
		Duration: durationpb.New(100000000),
	}

	formatter, _ := GetFormatter("junit")
	output, err := formatter.Format(resp, Config{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	outputStr := string(output)

	// Verify it's valid XML
	var suite junitTestSuite
	if err := xml.Unmarshal(output, &suite); err != nil {
		t.Fatalf("failed to parse XML: %v", err)
	}

	// Healthy check should have system-out with details
	if !strings.Contains(outputStr, "<system-out>") {
		t.Error("expected system-out element for healthy check with details")
	}
	if !strings.Contains(outputStr, "example.com") {
		t.Error("expected TLS CommonName in system-out")
	}
	if !strings.Contains(outputStr, "TLS 1.3") {
		t.Error("expected TLS version in system-out")
	}

	// Should NOT have failure
	if suite.Failures != 0 {
		t.Errorf("expected 0 failures for healthy check, got %d", suite.Failures)
	}
}

func TestJUnitFormatter_UnhealthyWithKStatusDetails(t *testing.T) {
	kstatusDetail := &details.Detail_KStatus{
		Status:  "InProgress",
		Message: "Deployment is progressing",
		Conditions: []*details.Condition{
			{
				Type:    "Available",
				Status:  "False",
				Reason:  "MinimumReplicasUnavailable",
				Message: "Not enough replicas",
			},
		},
	}
	anyDetail, err := anypb.New(kstatusDetail)
	if err != nil {
		t.Fatalf("failed to create any: %v", err)
	}

	resp := &ph.HealthCheckResponse{
		Name:     "deployment-check",
		Type:     "kubernetes",
		Status:   ph.Status_UNHEALTHY,
		Messages: []string{"deployment not ready"},
		Details:  []*anypb.Any{anyDetail},
		Duration: durationpb.New(200000000),
	}

	formatter, _ := GetFormatter("junit")
	output, err := formatter.Format(resp, Config{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	outputStr := string(output)

	// Verify it's valid XML
	var suite junitTestSuite
	if err := xml.Unmarshal(output, &suite); err != nil {
		t.Fatalf("failed to parse XML: %v", err)
	}

	// Should have failure with details
	if suite.Failures != 1 {
		t.Errorf("expected 1 failure, got %d", suite.Failures)
	}

	// Failure content should include details
	if !strings.Contains(outputStr, "deployment not ready") {
		t.Error("expected message in failure")
	}
	if !strings.Contains(outputStr, "Details:") {
		t.Error("expected 'Details:' section in failure content")
	}
	if !strings.Contains(outputStr, "Kubernetes Status:") {
		t.Error("expected KStatus details in failure")
	}
	if !strings.Contains(outputStr, "Available=False") {
		t.Error("expected condition in failure")
	}

	// Should also have system-out
	if !strings.Contains(outputStr, "<system-out>") {
		t.Error("expected system-out element")
	}
}

func TestJUnitFormatter_LoopDetectedWithDetails(t *testing.T) {
	loopDetail := &details.Detail_Loop{
		ServerIds: []string{"server-a", "server-b", "server-a"},
	}
	anyDetail, err := anypb.New(loopDetail)
	if err != nil {
		t.Fatalf("failed to create any: %v", err)
	}

	resp := &ph.HealthCheckResponse{
		Name:     "satellite-check",
		Type:     "satellite",
		Status:   ph.Status_LOOP_DETECTED,
		Details:  []*anypb.Any{anyDetail},
		Duration: durationpb.New(500000000),
	}

	formatter, _ := GetFormatter("junit")
	output, err := formatter.Format(resp, Config{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	outputStr := string(output)

	// Verify it's valid XML
	var suite junitTestSuite
	if err := xml.Unmarshal(output, &suite); err != nil {
		t.Fatalf("failed to parse XML: %v", err)
	}

	// Should be skipped
	if suite.Skipped != 1 {
		t.Errorf("expected 1 skipped, got %d", suite.Skipped)
	}

	// Skipped message should include loop details
	if !strings.Contains(outputStr, "LOOP_DETECTED") {
		t.Error("expected LOOP_DETECTED in skipped message")
	}
	if !strings.Contains(outputStr, "server-a -> server-b -> server-a") {
		t.Error("expected server chain in output")
	}
}

func TestJUnitFormatter_NoDetailsWhenEmpty(t *testing.T) {
	resp := &ph.HealthCheckResponse{
		Name:     "simple-check",
		Type:     "http",
		Status:   ph.Status_HEALTHY,
		Duration: durationpb.New(100000000),
		// No Details
	}

	formatter, _ := GetFormatter("junit")
	output, err := formatter.Format(resp, Config{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	outputStr := string(output)

	// Should NOT have system-out when no details
	if strings.Contains(outputStr, "<system-out>") {
		t.Error("expected no system-out element when no details present")
	}
}
