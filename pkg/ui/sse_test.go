//go:build ui

package ui_test

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/isometry/platform-health/pkg/ui"
)

func TestWriteEventSingleLine(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, ui.WriteEvent(&buf, "scan", []byte(`{"seq":1}`)))

	assert.Equal(t, "event: scan\ndata: {\"seq\":1}\n\n", buf.String())
}

func TestWriteEventPrefixesEveryLine(t *testing.T) {
	// SSE framing is line-oriented. A payload containing a newline must have
	// EVERY line prefixed, or the browser silently discards the remainder and
	// reports no error at all.
	var buf bytes.Buffer
	require.NoError(t, ui.WriteEvent(&buf, "snapshot", []byte("{\n  \"a\": 1\n}")))

	assert.Equal(t, "event: snapshot\ndata: {\ndata:   \"a\": 1\ndata: }\n\n", buf.String())
}

func TestWriteEventEmptyPayload(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, ui.WriteEvent(&buf, "heartbeat", []byte{}))

	assert.Equal(t, "event: heartbeat\ndata: \n\n", buf.String())
}
