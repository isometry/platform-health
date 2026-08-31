package details

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
)

// unknownTypeURL names a detail type this binary has never registered, the
// shape a rolling upgrade of a satellite server can hand it.
const unknownTypeURL = "type.googleapis.com/platform_health.detail.v1.Detail_FromTheFuture"

func mustUnknownAny(t *testing.T) *anypb.Any {
	t.Helper()
	return &anypb.Any{TypeUrl: unknownTypeURL, Value: []byte{0x0a, 0x02, 0x68, 0x69}}
}

func TestSanitiseAny(t *testing.T) {
	tests := []struct {
		name      string
		input     *anypb.Any
		wantSame  bool
		checkFunc func(t *testing.T, out *anypb.Any)
	}{
		{
			name:     "nil passes through",
			input:    nil,
			wantSame: true,
		},
		{
			name:     "known type passes through unchanged",
			input:    mustAny(t, &Detail_TLS{CommonName: "example.com"}),
			wantSame: true,
		},
		{
			name:  "unknown type becomes a resolvable stand-in",
			input: mustUnknownAny(t),
			checkFunc: func(t *testing.T, out *anypb.Any) {
				_, err := out.UnmarshalNew()
				require.NoError(t, err, "stand-in must itself be resolvable")

				b, err := protojson.Marshal(out)
				require.NoError(t, err)
				assert.Contains(t, string(b), unknownTypeURL, "original type URL must survive into the output")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := SanitiseAny(tt.input)
			if tt.wantSame {
				assert.Same(t, tt.input, out)
				return
			}
			tt.checkFunc(t, out)
		})
	}
}

func TestSanitiseAnyDeterministic(t *testing.T) {
	first := SanitiseAny(mustUnknownAny(t))
	second := SanitiseAny(mustUnknownAny(t))

	b1, err := protojson.Marshal(first)
	require.NoError(t, err)
	b2, err := protojson.Marshal(second)
	require.NoError(t, err)

	assert.Equal(t, b1, b2)
}

func TestSanitiseAnyDoesNotMutateInput(t *testing.T) {
	in := mustUnknownAny(t)
	originalTypeURL := in.GetTypeUrl()
	originalValue := append([]byte(nil), in.GetValue()...)

	SanitiseAny(in)

	assert.Equal(t, originalTypeURL, in.GetTypeUrl())
	assert.Equal(t, originalValue, in.GetValue())
}

func mustAny(t *testing.T, msg proto.Message) *anypb.Any {
	t.Helper()
	a, err := anypb.New(msg)
	require.NoError(t, err)
	return a
}
