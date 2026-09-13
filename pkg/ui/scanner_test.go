//go:build ui

package ui_test

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ph "github.com/isometry/platform-health/pkg/platform_health"
	"github.com/isometry/platform-health/pkg/ui"
)

// sseFrame is one decoded server-sent event.
type sseFrame struct {
	event string
	data  map[string]any
}

// sseStream is one open GET /api/events connection.
type sseStream struct {
	t      *testing.T
	body   io.ReadCloser
	frames chan sseFrame
}

func (st *sseStream) close() { _ = st.body.Close() }

// next returns the next frame, failing the test if none arrives in time.
func (st *sseStream) next(timeout time.Duration) sseFrame {
	st.t.Helper()
	select {
	case f, ok := <-st.frames:
		require.True(st.t, ok, "stream closed")
		return f
	case <-time.After(timeout):
		st.t.Fatalf("no frame within %s", timeout)
		return sseFrame{}
	}
}

// expect reads frames until one with the given event name arrives.
func (st *sseStream) expect(event string) sseFrame {
	st.t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		f := st.next(time.Until(deadline))
		if f.event == event {
			return f
		}
	}
	st.t.Fatalf("no %s frame", event)
	return sseFrame{}
}

// expectAll reads frames until every named event has arrived, in any order:
// a pending snapshot is hoisted ahead of the ring, so relative order between a
// snapshot and the scan frame that produced it is not part of the contract.
func (st *sseStream) expectAll(events ...string) {
	st.t.Helper()
	want := map[string]bool{}
	for _, e := range events {
		want[e] = true
	}
	deadline := time.Now().Add(5 * time.Second)
	for len(want) > 0 {
		f := st.next(time.Until(deadline))
		delete(want, f.event)
	}
}

// replayed is the first frames of a stream with the connection frame removed.
type replayed struct {
	events []string
	seqs   []float64
}

// replay reads n frames and returns the non-connection ones in order. The
// subscriber hoists a pending snapshot ahead of its ring, so the connection
// frame's position is not part of the contract.
func (st *sseStream) replay(n int) replayed {
	st.t.Helper()
	var out replayed
	for i := 0; i < n; i++ {
		f := st.next(time.Second)
		if f.event == "connection" {
			continue
		}
		out.events = append(out.events, f.event)
		out.seqs = append(out.seqs, st.seq(f))
	}
	return out
}

func (st *sseStream) seq(f sseFrame) float64 {
	st.t.Helper()
	n, ok := f.data["seq"].(float64)
	require.True(st.t, ok, "frame %s carries no seq", f.event)
	return n
}

// testServer serves a scanner over HTTP and runs its loop until the test ends.
type testServer struct {
	t       *testing.T
	scanner *ui.Scanner
	srv     *httptest.Server
	cancel  context.CancelFunc
}

func newTestServer(t *testing.T, cfg ui.ScannerConfig, check func(context.Context) (*ph.HealthCheckResponse, error)) *testServer {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	s := ui.NewTestScanner(ctx, cfg, check)
	go s.Run()

	srv := httptest.NewServer(s.Mux(tcpAddr(t, "127.0.0.1:8090"), ui.Assets()))
	ts := &testServer{t: t, scanner: s, srv: srv, cancel: cancel}
	t.Cleanup(func() {
		cancel()
		s.Release()
		srv.Close()
		select {
		case <-s.Done():
		case <-time.After(5 * time.Second):
			t.Error("scanner loop did not exit")
		}
	})
	return ts
}

// stream opens an SSE connection and decodes its frames in the background.
func (ts *testServer) stream() *sseStream {
	ts.t.Helper()
	req, err := http.NewRequest(http.MethodGet, ts.srv.URL+"/api/events", nil)
	require.NoError(ts.t, err)
	req.Host = "127.0.0.1:8090"
	resp, err := ts.srv.Client().Do(req)
	require.NoError(ts.t, err)
	require.Equal(ts.t, http.StatusOK, resp.StatusCode)

	st := &sseStream{t: ts.t, body: resp.Body, frames: make(chan sseFrame, 64)}
	ts.t.Cleanup(st.close)
	go func() {
		defer close(st.frames)
		scanner := bufio.NewScanner(resp.Body)
		scanner.Buffer(make([]byte, 1<<20), 1<<20)
		var event string
		var data strings.Builder
		for scanner.Scan() {
			line := scanner.Text()
			switch {
			case strings.HasPrefix(line, "event:"):
				event = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
			case strings.HasPrefix(line, "data:"):
				data.WriteString(strings.TrimPrefix(strings.TrimPrefix(line, "data:"), " "))
			case line == "" && event != "":
				var payload map[string]any
				_ = json.Unmarshal([]byte(data.String()), &payload)
				st.frames <- sseFrame{event: event, data: payload}
				event, data = "", strings.Builder{}
			}
		}
	}()
	return st
}

func (ts *testServer) triggerScan() {
	ts.t.Helper()
	req, err := http.NewRequest(http.MethodPost, ts.srv.URL+"/api/scan", nil)
	require.NoError(ts.t, err)
	req.Host = "127.0.0.1:8090"
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	resp, err := ts.srv.Client().Do(req)
	require.NoError(ts.t, err)
	_ = resp.Body.Close()
	require.Equal(ts.t, http.StatusAccepted, resp.StatusCode)
}

// parkedCheck is a scan that blocks until released.
type parkedCheck struct {
	started chan struct{}
	release chan struct{}
}

func newParkedCheck() *parkedCheck {
	return &parkedCheck{started: make(chan struct{}, 8), release: make(chan struct{})}
}

func (p *parkedCheck) check(ctx context.Context) (*ph.HealthCheckResponse, error) {
	p.started <- struct{}{}
	select {
	case <-p.release:
		return node("", "", ph.Status_HEALTHY, node("db", "tcp", ph.Status_HEALTHY)), nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (p *parkedCheck) waitStarted(t *testing.T) {
	t.Helper()
	select {
	case <-p.started:
	case <-time.After(5 * time.Second):
		t.Fatal("scan did not start")
	}
}

func healthyCheck(context.Context) (*ph.HealthCheckResponse, error) {
	return node("", "", ph.Status_HEALTHY, node("db", "tcp", ph.Status_HEALTHY)), nil
}

func TestSubscribeDoesNotBlockDuringScan(t *testing.T) {
	parked := newParkedCheck()
	ts := newTestServer(t, ui.ScannerConfig{Timeout: 10 * time.Second}, parked.check)

	first := ts.stream()
	first.expect("connection")
	ts.triggerScan()
	first.expect("scanning")
	parked.waitStarted(t)

	subscribed := make(chan *ui.Subscriber, 1)
	go func() { subscribed <- ts.scanner.Subscribe() }()
	select {
	case sub := <-subscribed:
		ts.scanner.Unsubscribe(sub)
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Subscribe blocked while a scan was running")
	}

	second := ts.stream()
	second.expect("connection")
	second.expect("scanning")

	close(parked.release)
	first.expectAll("scan", "snapshot")
	second.expectAll("scan", "snapshot")
}

func TestUnsubscribeDoesNotBlockDuringScan(t *testing.T) {
	parked := newParkedCheck()
	ts := newTestServer(t, ui.ScannerConfig{Timeout: 10 * time.Second}, parked.check)

	sub := ts.scanner.Subscribe()
	ts.scanner.Trigger("manual")
	parked.waitStarted(t)

	done := make(chan struct{})
	go func() { ts.scanner.Unsubscribe(sub); close(done) }()
	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Unsubscribe blocked while a scan was running")
	}
	close(parked.release)
}

func TestTriggerWhileScanningQueuesOneFollowUp(t *testing.T) {
	parked := newParkedCheck()
	ts := newTestServer(t, ui.ScannerConfig{Timeout: 10 * time.Second}, parked.check)

	st := ts.stream()
	st.expect("scanning") // first-subscriber scan
	parked.waitStarted(t)
	assert.Equal(t, ui.TriggerQueued, ts.scanner.Trigger("manual"))
	assert.Equal(t, ui.TriggerCoalesced, ts.scanner.Trigger("manual"))

	close(parked.release)
	assert.Equal(t, float64(1), st.seq(st.expect("scan")))
	assert.Equal(t, float64(2), st.seq(st.expect("scan")))

	// Nothing else is pending: a further trigger starts a fresh scan.
	assert.Equal(t, ui.TriggerStarted, ts.scanner.Trigger("manual"))
	assert.Equal(t, float64(3), st.seq(st.expect("scan")))
}

func TestTimerDoesNotFireDuringScan(t *testing.T) {
	parked := newParkedCheck()
	ts := newTestServer(t, ui.ScannerConfig{Timeout: 10 * time.Second, Refresh: 50 * time.Millisecond}, parked.check)

	st := ts.stream()
	st.expect("connection")
	st.expect("scanning") // first-subscriber scan
	parked.waitStarted(t)

	time.Sleep(300 * time.Millisecond)
	select {
	case <-parked.started:
		t.Fatal("a second scan started while the first was running")
	default:
	}
	close(parked.release)
	assert.Equal(t, float64(1), st.seq(st.expect("scan")))
}

func TestReplayIncludesScanFrame(t *testing.T) {
	ts := newTestServer(t, ui.ScannerConfig{Timeout: time.Second}, healthyCheck)

	first := ts.stream()
	first.expect("connection")
	first.expect("scan")

	second := ts.stream()
	replay := second.replay(3)
	require.Equal(t, []string{"snapshot", "scan"}, replay.events)
	assert.Equal(t, replay.seqs[0], replay.seqs[1])
}

func TestReplayAfterFailureKeepsOlderScanFrame(t *testing.T) {
	calls := 0
	check := func(ctx context.Context) (*ph.HealthCheckResponse, error) {
		calls++
		if calls > 1 {
			return nil, errors.New("boom")
		}
		return healthyCheck(ctx)
	}
	ts := newTestServer(t, ui.ScannerConfig{Timeout: time.Second}, check)

	first := ts.stream()
	first.expect("scan")
	ts.triggerScan()
	first.expect("scan-error")

	second := ts.stream()
	replay := second.replay(4)
	assert.Equal(t, []string{"snapshot", "scan", "scan-error"}, replay.events)
	assert.Equal(t, []float64{1, 1, 2}, replay.seqs)
}

func TestRunReturnsWithInflightScanOnCancel(t *testing.T) {
	parked := newParkedCheck()
	ts := newTestServer(t, ui.ScannerConfig{Timeout: 10 * time.Second}, parked.check)

	ts.scanner.Trigger("manual")
	parked.waitStarted(t)

	ts.cancel()
	select {
	case <-ts.scanner.Done():
	case <-time.After(time.Second):
		t.Fatal("Run did not return while a scan was in flight")
	}
}

func TestFirstSubscriberDuringScanDoesNotQueue(t *testing.T) {
	parked := newParkedCheck()
	ts := newTestServer(t, ui.ScannerConfig{Timeout: 10 * time.Second}, parked.check)

	first := ts.stream()
	first.expect("scanning")
	parked.waitStarted(t)

	second := ts.stream()
	second.expect("scanning")

	close(parked.release)
	assert.Equal(t, float64(1), second.seq(second.expect("scan")))
	assert.Equal(t, ui.TriggerStarted, ts.scanner.Trigger("manual"), "no follow-up scan was queued by the second subscriber")
}
