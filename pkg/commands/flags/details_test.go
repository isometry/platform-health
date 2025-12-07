package flags

import (
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/isometry/platform-health/pkg/platform_health/details"
)

func TestRenderTLSDetail(t *testing.T) {
	validUntil := time.Now().Add(30 * 24 * time.Hour) // 30 days from now
	tlsDetail := &details.Detail_TLS{
		CommonName:         "example.com",
		SubjectAltNames:    []string{"example.com", "www.example.com"},
		Chain:              []string{"example.com", "Intermediate CA", "Root CA"},
		ValidUntil:         timestamppb.New(validUntil),
		SignatureAlgorithm: "SHA256-RSA",
		PublicKeyAlgorithm: "RSA",
		Version:            "TLS 1.3",
		CipherSuite:        "TLS_AES_256_GCM_SHA384",
		Protocol:           "h2",
	}

	anyDetail, err := anypb.New(tlsDetail)
	if err != nil {
		t.Fatalf("failed to create any: %v", err)
	}

	output := RenderDetail(anyDetail, "")

	// Verify key fields are present
	checks := []string{
		"TLS Certificate:",
		"Common Name: example.com",
		"Subject Alt Names: example.com, www.example.com",
		"Valid Until:",
		"remaining",
		"TLS Version: TLS 1.3",
		"Cipher Suite: TLS_AES_256_GCM_SHA384",
		"Protocol: h2",
		"Certificate Chain:",
		"[0] example.com",
		"[1] Intermediate CA",
		"[2] Root CA",
	}

	for _, check := range checks {
		if !strings.Contains(output, check) {
			t.Errorf("expected output to contain %q, got:\n%s", check, output)
		}
	}
}

func TestRenderTLSDetail_Expired(t *testing.T) {
	validUntil := time.Now().Add(-24 * time.Hour) // Expired yesterday
	tlsDetail := &details.Detail_TLS{
		CommonName: "expired.example.com",
		ValidUntil: timestamppb.New(validUntil),
	}

	anyDetail, err := anypb.New(tlsDetail)
	if err != nil {
		t.Fatalf("failed to create any: %v", err)
	}

	output := RenderDetail(anyDetail, "")

	if !strings.Contains(output, "expired") {
		t.Errorf("expected output to contain 'expired', got:\n%s", output)
	}
}

func TestRenderKStatusDetail(t *testing.T) {
	kstatusDetail := &details.Detail_KStatus{
		Status:  "InProgress",
		Message: "Deployment is progressing",
		Conditions: []*details.Condition{
			{
				Type:    "Available",
				Status:  "False",
				Reason:  "MinimumReplicasUnavailable",
				Message: "Deployment does not have minimum availability",
			},
			{
				Type:   "Progressing",
				Status: "True",
				Reason: "ReplicaSetUpdated",
			},
		},
	}

	anyDetail, err := anypb.New(kstatusDetail)
	if err != nil {
		t.Fatalf("failed to create any: %v", err)
	}

	output := RenderDetail(anyDetail, "")

	checks := []string{
		"Kubernetes Status:",
		"Status: InProgress",
		"Message: Deployment is progressing",
		"Conditions:",
		"Available=False (MinimumReplicasUnavailable): Deployment does not have minimum availability",
		"Progressing=True (ReplicaSetUpdated)",
	}

	for _, check := range checks {
		if !strings.Contains(output, check) {
			t.Errorf("expected output to contain %q, got:\n%s", check, output)
		}
	}
}

func TestRenderLoopDetail(t *testing.T) {
	loopDetail := &details.Detail_Loop{
		ServerIds: []string{"server-a", "server-b", "server-a"},
	}

	anyDetail, err := anypb.New(loopDetail)
	if err != nil {
		t.Fatalf("failed to create any: %v", err)
	}

	output := RenderDetail(anyDetail, "")

	checks := []string{
		"Loop Detection:",
		"Server Chain: server-a -> server-b -> server-a",
	}

	for _, check := range checks {
		if !strings.Contains(output, check) {
			t.Errorf("expected output to contain %q, got:\n%s", check, output)
		}
	}
}

func TestRenderDetails_Multiple(t *testing.T) {
	tlsDetail := &details.Detail_TLS{CommonName: "example.com"}
	kstatusDetail := &details.Detail_KStatus{Status: "Ready"}

	anyTLS, _ := anypb.New(tlsDetail)
	anyKStatus, _ := anypb.New(kstatusDetail)

	output := RenderDetails([]*anypb.Any{anyTLS, anyKStatus}, "")

	if !strings.Contains(output, "TLS Certificate:") {
		t.Error("expected TLS detail in output")
	}
	if !strings.Contains(output, "Kubernetes Status:") {
		t.Error("expected KStatus detail in output")
	}
}

func TestRenderDetails_Empty(t *testing.T) {
	output := RenderDetails(nil, "")
	if output != "" {
		t.Errorf("expected empty output for nil details, got: %q", output)
	}

	output = RenderDetails([]*anypb.Any{}, "")
	if output != "" {
		t.Errorf("expected empty output for empty details, got: %q", output)
	}
}

func TestRenderDetail_WithIndent(t *testing.T) {
	tlsDetail := &details.Detail_TLS{CommonName: "example.com"}
	anyDetail, _ := anypb.New(tlsDetail)

	output := RenderDetail(anyDetail, "  ")

	if !strings.HasPrefix(output, "  TLS Certificate:") {
		t.Errorf("expected indented output, got:\n%s", output)
	}
	if !strings.Contains(output, "    Common Name:") {
		t.Errorf("expected double-indented fields, got:\n%s", output)
	}
}

func TestRenderDetail_Nil(t *testing.T) {
	output := RenderDetail(nil, "")
	if output != "" {
		t.Errorf("expected empty output for nil detail, got: %q", output)
	}
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		duration time.Duration
		contains string
	}{
		{-time.Hour, "expired"},
		{30 * time.Second, "30s remaining"},
		{5 * time.Minute, "5m remaining"},
		{2*time.Hour + 30*time.Minute, "2h 30m remaining"},
		{2 * 24 * time.Hour, "2d 0h remaining"},
		{45 * 24 * time.Hour, "45d remaining"},
	}

	for _, tt := range tests {
		t.Run(tt.contains, func(t *testing.T) {
			result := formatDuration(tt.duration)
			if !strings.Contains(result, tt.contains) {
				t.Errorf("formatDuration(%v) = %q, want to contain %q", tt.duration, result, tt.contains)
			}
		})
	}
}
