package output

import (
	"bytes"
	"testing"

	"github.com/goccy/go-yaml"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"

	ph "github.com/isometry/platform-health/pkg/platform_health"
	"github.com/isometry/platform-health/pkg/platform_health/details"
)

// unknownDetailTypeURL names a detail type this binary has never registered,
// the shape a rolling upgrade of a satellite server can hand it over gRPC.
const unknownDetailTypeURL = "type.googleapis.com/platform_health.detail.v1.Detail_FromTheFuture"

func mustUnknownDetailAny(t *testing.T) *anypb.Any {
	t.Helper()
	return &anypb.Any{TypeUrl: unknownDetailTypeURL, Value: []byte{0x0a, 0x02, 0x68, 0x69}}
}

func mustDetailAny(t *testing.T, msg proto.Message) *anypb.Any {
	t.Helper()
	a, err := anypb.New(msg)
	require.NoError(t, err)
	return a
}

// knownDetailsTree builds a tree with only registered detail types, used to
// prove sanitisation does not change existing output at all.
func knownDetailsTree(t *testing.T) *ph.HealthCheckResponse {
	return &ph.HealthCheckResponse{
		Name:   "root",
		Type:   "system",
		Status: ph.Status_HEALTHY,
		Components: []*ph.HealthCheckResponse{
			{
				Name:    "x",
				Type:    "tls",
				Status:  ph.Status_HEALTHY,
				Details: []*anypb.Any{mustDetailAny(t, &details.Detail_TLS{CommonName: "example.com"})},
			},
			{Name: "clean", Type: "tcp", Status: ph.Status_HEALTHY},
		},
	}
}

func TestFormatters_SurviveUnresolvableDetail(t *testing.T) {
	tests := []struct {
		name      string
		formatter string
	}{
		{"json", "json"},
		{"yaml_plain", "yaml"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := &ph.HealthCheckResponse{
				Name:   "root",
				Type:   "system",
				Status: ph.Status_UNHEALTHY,
				Components: []*ph.HealthCheckResponse{
					{Name: "healthy-sibling", Type: "tcp", Status: ph.Status_HEALTHY},
					{Name: "future", Type: "satellite", Status: ph.Status_UNHEALTHY,
						Details: []*anypb.Any{mustUnknownDetailAny(t)}},
				},
			}

			formatter, ok := GetFormatter(tt.formatter)
			require.True(t, ok)

			out, err := formatter.Format(resp, Config{})
			require.NoError(t, err)
			assert.NotEmpty(t, out)
			assert.Contains(t, string(out), "healthy-sibling")
			assert.Contains(t, string(out), "future")
		})
	}
}

func TestFormatters_UnresolvableDetailKeepsTypeURLVisible(t *testing.T) {
	tests := []struct {
		name      string
		formatter string
	}{
		{"json", "json"},
		{"yaml_plain", "yaml"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := &ph.HealthCheckResponse{
				Name:    "future",
				Type:    "satellite",
				Status:  ph.Status_UNHEALTHY,
				Details: []*anypb.Any{mustUnknownDetailAny(t)},
			}

			formatter, ok := GetFormatter(tt.formatter)
			require.True(t, ok)

			out, err := formatter.Format(resp, Config{})
			require.NoError(t, err)
			assert.Contains(t, string(out), unknownDetailTypeURL)
		})
	}
}

// TestJSONFormatter_KnownDetailsByteIdentical is the regression that matters
// most: sanitisation must not change a single byte of existing output.
func TestJSONFormatter_KnownDetailsByteIdentical(t *testing.T) {
	resp := knownDetailsTree(t)

	want, err := (protojson.MarshalOptions{Multiline: true, Indent: "  "}).Marshal(resp)
	require.NoError(t, err)

	formatter, ok := GetFormatter("json")
	require.True(t, ok)
	got, err := formatter.Format(resp, Config{})
	require.NoError(t, err)

	assert.Equal(t, want, got)
}

// TestYAMLFormatter_KnownDetailsByteIdentical mirrors the plain (non-colorized)
// branch's own marshalling path to prove sanitisation is a no-op passthrough
// for a tree with only registered detail types.
func TestYAMLFormatter_KnownDetailsByteIdentical(t *testing.T) {
	resp := knownDetailsTree(t)

	jsonBytes, err := protojson.Marshal(resp)
	require.NoError(t, err)
	var data any
	require.NoError(t, yaml.UnmarshalWithOptions(jsonBytes, &data, yaml.UseOrderedMap()))
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf, yaml.Indent(2), yaml.IndentSequence(true))
	require.NoError(t, enc.Encode(data))
	want := bytes.TrimRight(buf.Bytes(), "\n")

	formatter, ok := GetFormatter("yaml")
	require.True(t, ok)
	got, err := formatter.Format(resp, Config{})
	require.NoError(t, err)

	assert.Equal(t, want, got)
}

// TestJUnitFormatter_UnresolvableDetailUnaffected confirms the junit
// formatter needs no fix: it never marshals an Any tree in one shot, so
// details.RenderAny's existing per-detail "[unmarshal error]" fallback
// already isolates one bad detail without failing the whole document.
func TestJUnitFormatter_UnresolvableDetailUnaffected(t *testing.T) {
	resp := &ph.HealthCheckResponse{
		Name:    "future",
		Type:    "satellite",
		Status:  ph.Status_UNHEALTHY,
		Details: []*anypb.Any{mustUnknownDetailAny(t)},
	}

	formatter, ok := GetFormatter("junit")
	require.True(t, ok)
	out, err := formatter.Format(resp, Config{})
	require.NoError(t, err)
	assert.Contains(t, string(out), "[unmarshal error]")
}
