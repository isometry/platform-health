//go:build ui

package ui

import (
	"encoding/json"
	"fmt"
	"time"

	"google.golang.org/grpc/connectivity"
)

const (
	eventSnapshot   = "snapshot"
	eventScan       = "scan"
	eventScanning   = "scanning"
	eventScanError  = "scan-error"
	eventConnection = "connection"
)

// snapshotEvent embeds the protojson tree as raw bytes, so a snapshot is
// marshalled exactly once, on the scanner goroutine.
type snapshotEvent struct {
	Snapshot    json.RawMessage `json:"snapshot"`
	ScanID      string          `json:"scanID"`
	Seq         uint64          `json:"seq"`
	ObservedAt  time.Time       `json:"observedAt"`
	Transitions []Transition    `json:"transitions"`
}

// scanEvent is the liveness signal emitted after every completed scan, changed
// or not. It is what makes suppressing unchanged snapshots safe.
type scanEvent struct {
	ScanID     string    `json:"scanID"`
	Seq        uint64    `json:"seq"`
	Reason     string    `json:"reason"`
	ObservedAt time.Time `json:"observedAt"`
	DurationMs int64     `json:"durationMs"`
	Changed    bool      `json:"changed"`
}

type scanningEvent struct {
	ScanID         string    `json:"scanID"`
	Seq            uint64    `json:"seq"`
	Reason         string    `json:"reason"`
	StartedAt      time.Time `json:"startedAt"`
	QueuedFollowUp bool      `json:"queuedFollowUp"`
}

type scanErrorEvent struct {
	ScanID string    `json:"scanID"`
	Seq    uint64    `json:"seq"`
	Code   string    `json:"code"`
	Error  string    `json:"error"`
	At     time.Time `json:"at"`
}

type connectionEvent struct {
	State    string `json:"state"`
	Severity string `json:"severity"`
	Target   string `json:"target"`
	// RefreshMs is the configured auto-refresh interval, 0 when auto-refresh is
	// disabled. The dashboard says "sampled every N", and a client that had to
	// infer N from the gap between two scans could not tell a disabled refresh
	// from one whose second scan has not landed yet.
	RefreshMs int64 `json:"refreshMs"`
}

// newConnectionEvent maps a channel state for the UI. Target is the dialled
// address from our own flags, never a value taken from a response.
func newConnectionEvent(state connectivity.State, target string, refresh time.Duration) connectionEvent {
	return connectionEvent{
		State:     state.String(),
		Severity:  connectionSeverity(state),
		Target:    target,
		RefreshMs: refresh.Milliseconds(),
	}
}

func connectionSeverity(state connectivity.State) string {
	switch state {
	case connectivity.Ready:
		return "ok"
	case connectivity.Idle:
		// Benign, not an error: with manual refresh the channel idles after 30
		// minutes and would otherwise raise a false alarm on a healthy setup.
		return "benign"
	case connectivity.Connecting:
		return "transient"
	case connectivity.TransientFailure:
		return "error"
	case connectivity.Shutdown:
		return "terminal"
	default:
		return "unknown"
	}
}

// encodeEvent renders one event payload. Callers must surface a failure rather
// than shipping an empty envelope, which a browser renders as a blank dashboard
// with no explanation.
func encodeEvent(event string, v any) (frame, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return frame{}, fmt.Errorf("marshal %s event: %w", event, err)
	}
	return frame{event, data}, nil
}
