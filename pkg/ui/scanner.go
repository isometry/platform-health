// Package ui serves a live platform-health dashboard: a gRPC client that polls
// the server's unary Check RPC and fans results out to browsers over SSE.
//
// One scanner goroutine owns the gRPC connection, the refresh timer and the
// subscriber registry, serialising them in a single select. That makes
// "subscriber count reached zero" a serialised event rather than a race between
// HTTP handlers.
//
// Scanner.Trigger takes no context.Context, deliberately and permanently. If it
// accepted one, the obvious implementation in an HTTP handler passes
// r.Context(), and then the first-subscriber scan is cancelled every time a
// browser's EventSource reconnects, which it does every few seconds while a page
// loads. Scan contexts derive from rootCtx alone.
package ui

import (
	"context"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/backoff"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/isometry/platform-health/pkg/client"
	ph "github.com/isometry/platform-health/pkg/platform_health"

	// protojson resolves Any details through the global registry and errors if
	// a type is unregistered, and that error kills marshalling of the entire
	// response, not just the detail. Without this every scan fails the moment
	// any provider sets detail: true.
	_ "github.com/isometry/platform-health/pkg/platform_health/details"
)

// TriggerState reports what a Trigger call did, so the UI can say "scanning,
// refresh queued" rather than pretending a press started a fresh scan.
type TriggerState string

const (
	TriggerStarted   TriggerState = "started"
	TriggerQueued    TriggerState = "queued"
	TriggerCoalesced TriggerState = "coalesced"
)

// ScanState is the scanner's current activity, replayed to new subscribers.
type ScanState string

const (
	ScanIdle    ScanState = "idle"
	ScanRunning ScanState = "scanning"
)

const (
	// minScanInterval floors auto-triggered scans. EventSource reconnects every
	// few seconds, so without it a down server scans once per reconnect: every
	// scan fails, so "no snapshot" is re-satisfied every time.
	minScanInterval = 10 * time.Second

	// DefaultTimeout matches the --timeout flag default.
	DefaultTimeout = 30 * time.Second
)

// ScannerConfig configures a Scanner.
type ScannerConfig struct {
	Dial    client.DialConfig
	Timeout time.Duration // per-scan deadline
	Refresh time.Duration // auto-refresh interval; 0 disables auto-refresh
}

// store holds the latest observation as immutable bytes, because
// internal/output mutates its argument and Flatten aliases its slices, so a
// live message must never escape. Replay frames are kept encoded.
type store struct {
	payload     []byte
	canon       *ph.HealthCheckResponse
	hash        string
	scanID      string
	seq         uint64
	observedAt  time.Time
	transitions []Transition
	lastError   string
	scanState   ScanState

	snapshotFrame *frame
	errorFrame    *frame
	scanningFrame *frame
	connFrame     *frame
}

// StoreState is a consistent read of the store for HTTP handlers.
type StoreState struct {
	Snapshot    []byte
	Hash        string
	ScanID      string
	Seq         uint64
	ObservedAt  time.Time
	Transitions []Transition
	LastError   string
	ScanState   ScanState
}

// Scanner owns the gRPC connection, the refresh timer and the subscriber
// registry, and runs the poll loop.
type Scanner struct {
	cfg     ScannerConfig
	rootCtx context.Context

	// target labels the source in connection events; check performs one scan.
	// Fixture mode swaps check and leaves conn nil.
	target string
	check  func(context.Context) (*ph.HealthCheckResponse, error)
	conn   *grpc.ClientConn

	trigger    chan string
	register   chan *Subscriber
	unregister chan *Subscriber
	connState  chan connectivity.State

	// shutdown is closed by Release to let SSE handlers return; done is closed
	// by Run when the loop exits.
	releaseOnce sync.Once
	shutdown    chan struct{}
	done        chan struct{}

	scanning atomic.Bool
	seq      atomic.Uint64

	// subs, lastAttempt and the connection dedupe belong to the scanner
	// goroutine alone.
	subs        map[*Subscriber]struct{}
	lastAttempt time.Time
	connSeeded  bool
	connLast    connectivity.State

	mu    sync.RWMutex
	store store
}

// NewScanner dials the target and returns a scanner ready for Run. The
// connection is lazy: grpc.NewClient performs no I/O.
func NewScanner(rootCtx context.Context, cfg ScannerConfig) (*Scanner, error) {
	conn, err := client.Dial(cfg.Dial,
		// The default backoff ceiling is 120s, so an unattended dashboard stays
		// dark for two minutes after the server recovers. ResetConnectBackoff is
		// experimental and its own doc says not to use it, so fix it here.
		grpc.WithConnectParams(grpc.ConnectParams{Backoff: backoff.Config{
			BaseDelay: time.Second, Multiplier: 1.6, Jitter: 0.2, MaxDelay: 15 * time.Second,
		}}),
		// The client sends no keepalive pings by default, so an idle socket
		// across a NAT is silently reaped. 5m is the server's EnforcementPolicy
		// minimum; lower earns a GOAWAY ENHANCE_YOUR_CALM.
		grpc.WithKeepaliveParams(keepalive.ClientParameters{
			Time: 5 * time.Minute, Timeout: 20 * time.Second,
		}),
		// The default is 4MB; a large estate with detail:true exceeds it, and
		// the failure mode is ResourceExhausted on every scan forever.
		grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(32<<20)),
	)
	if err != nil {
		return nil, fmt.Errorf("dial %s: %w", cfg.Dial.Address(), err)
	}

	s := newScanner(rootCtx, cfg, cfg.Dial.Address())
	s.conn = conn
	health := ph.NewHealthClient(conn)
	s.check = func(ctx context.Context) (*ph.HealthCheckResponse, error) {
		return health.Check(ctx, &ph.HealthCheckRequest{})
	}
	return s, nil
}

// NewFixtureScanner returns a scanner that reads a protojson snapshot from disk
// instead of dialling, so the dashboard can be developed without a server. The
// file is re-read on every scan, so an edit shows up on the next refresh.
func NewFixtureScanner(rootCtx context.Context, cfg ScannerConfig, path string) (*Scanner, error) {
	// Fail at startup rather than as a scan error the browser has to surface.
	if _, err := loadFixture(path); err != nil {
		return nil, err
	}

	s := newScanner(rootCtx, cfg, "fixture:"+path)
	s.check = func(context.Context) (*ph.HealthCheckResponse, error) {
		return loadFixture(path)
	}
	return s, nil
}

func loadFixture(path string) (*ph.HealthCheckResponse, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read fixture: %w", err)
	}
	resp := &ph.HealthCheckResponse{}
	if err := (protojson.UnmarshalOptions{DiscardUnknown: true}).Unmarshal(data, resp); err != nil {
		return nil, fmt.Errorf("parse fixture %s: %w", path, err)
	}
	return resp, nil
}

func newScanner(rootCtx context.Context, cfg ScannerConfig, target string) *Scanner {
	if cfg.Timeout <= 0 {
		cfg.Timeout = DefaultTimeout
	}

	return &Scanner{
		cfg:        cfg,
		rootCtx:    rootCtx,
		target:     target,
		trigger:    make(chan string, 1),
		register:   make(chan *Subscriber),
		unregister: make(chan *Subscriber),
		connState:  make(chan connectivity.State, 1),
		shutdown:   make(chan struct{}),
		done:       make(chan struct{}),
		subs:       make(map[*Subscriber]struct{}),
		store:      store{scanState: ScanIdle},
	}
}

// Close releases the gRPC connection. Call it only after Run has returned:
// closing first makes an in-flight Check fail with a confusing Unavailable.
func (s *Scanner) Close() error {
	if s.conn == nil {
		return nil
	}
	return s.conn.Close()
}

// Release tells SSE handlers to write their farewell and return. It must run
// before http.Server.Shutdown, which otherwise blocks forever on handlers that
// never return on their own. Idempotent. It does NOT stop Run: only cancelling
// rootCtx does that.
func (s *Scanner) Release() {
	s.releaseOnce.Do(func() { close(s.shutdown) })
}

// Released is closed by Release. A handler must select on it: the loop may
// still be finishing an in-flight scan, so waiting for a closed doorbell
// instead would stall shutdown for a whole scan timeout. Like the doorbell it
// is closed, not signalled: keep selecting on it after it fires and you spin.
func (s *Scanner) Released() <-chan struct{} {
	return s.shutdown
}

// Done is closed when Run returns, which happens only on rootCtx cancellation.
// Release alone will never close it, so waiting here without cancelling rootCtx
// hangs with no diagnostic.
func (s *Scanner) Done() <-chan struct{} {
	return s.done
}

// Trigger requests a scan. It takes no context.Context, deliberately and
// permanently: see the package doc.
//
// At most one scan runs and at most one is queued. A press during a running
// scan queues a fresh scan; a second press coalesces into the queued one.
func (s *Scanner) Trigger(reason string) TriggerState {
	select {
	case s.trigger <- reason:
		if s.scanning.Load() {
			return TriggerQueued
		}
		return TriggerStarted
	default:
		return TriggerCoalesced
	}
}

// Subscribe registers a new SSE subscriber and replays current state to it.
//
// It blocks until the loop reaches its select, which during a scan means up to
// one full scan timeout. Handlers must write and flush their SSE headers first,
// or a browser sees a hung request.
func (s *Scanner) Subscribe() *Subscriber {
	sub := newSubscriber()
	select {
	case s.register <- sub:
	case <-s.done:
		// Registration lost the race with shutdown, so the sweep over s.subs
		// cannot see it. Closing here is still the hub doing the closing.
		sub.close()
	}
	return sub
}

// Unsubscribe removes a subscriber. Safe to call more than once.
func (s *Scanner) Unsubscribe(sub *Subscriber) {
	select {
	case s.unregister <- sub:
	case <-s.done:
		// The loop has exited and already closed this subscriber.
	}
}

// Target returns the label under which this scanner reports connection
// events: a dialled address, or "fixture:<path>" in fixture mode.
func (s *Scanner) Target() string {
	return s.target
}

// State returns a consistent read of the store. The returned slices are
// immutable and must not be modified.
func (s *Scanner) State() StoreState {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return StoreState{
		Snapshot:    s.store.payload,
		Hash:        s.store.hash,
		ScanID:      s.store.scanID,
		Seq:         s.store.seq,
		ObservedAt:  s.store.observedAt,
		Transitions: s.store.transitions,
		LastError:   s.store.lastError,
		ScanState:   s.store.scanState,
	}
}

// Run is the scan loop. It returns when rootCtx is done.
func (s *Scanner) Run() {
	defer close(s.done)
	// Only the hub closes a subscriber, so shutdown is its job too. No deliver
	// can run once Run has returned.
	defer func() {
		for sub := range s.subs {
			sub.close()
		}
	}()

	// Seed the connection frame before any subscriber can register, so the
	// first replay is never missing it.
	if s.conn != nil {
		s.broadcastConnection(s.conn.GetState())
	} else {
		// Fixture mode has no channel to report on, but a dashboard with no
		// connection frame at all looks like a frame that failed to arrive.
		s.broadcastConnection(connectivity.Ready)
	}
	go s.watchConnection()

	timer := time.NewTimer(time.Hour)
	stopTimer(timer)
	defer timer.Stop()

	for {
		select {
		case <-s.rootCtx.Done():
			return

		case sub := <-s.register:
			s.subs[sub] = struct{}{}
			s.replayTo(sub)
			if len(s.subs) == 1 {
				s.armTimer(timer)
			}
			if !s.haveSnapshot() && time.Since(s.lastAttempt) > s.floor() {
				s.Trigger("first-subscriber")
			}

		case sub := <-s.unregister:
			delete(s.subs, sub)
			sub.close()
			if len(s.subs) == 0 {
				stopTimer(timer)
			}
			// An in-flight scan is NOT cancelled here. Cancelling would unwind
			// provider.Check mid-probe on the server, leaving half-open
			// connections against production endpoints on every closed tab.

		case reason := <-s.trigger:
			s.runScan(reason)
			// Reset from COMPLETION, never a fixed ticker: a ticker period
			// shorter than a slow scan leaves the next tick already pending.
			s.armTimer(timer)

		case <-timer.C:
			// Re-check: Stop does not drain an already-delivered tick. The
			// floor also bounds auto-triggered scans when --refresh is short.
			if len(s.subs) > 0 && time.Since(s.lastAttempt) >= s.floor() {
				s.runScan("refresh")
			}
			s.armTimer(timer)

		case state := <-s.connState:
			s.broadcastConnection(state)
		}
	}
}

// runScan performs one scan and broadcasts its outcome.
func (s *Scanner) runScan(reason string) {
	var (
		scanID string
		seq    uint64
	)
	// Registered first so it covers every statement below, uuid included: one
	// bad response must not kill polling for every viewer.
	defer func() {
		if r := recover(); r != nil {
			s.failScan(scanID, seq, fmt.Errorf("panic during %s scan: %v", reason, r))
		}
	}()

	seq = s.seq.Add(1)
	scanID = uuid.Must(uuid.NewV7()).String()
	started := time.Now()

	s.scanning.Store(true)
	defer s.scanning.Store(false)

	scanningFrame, err := encodeEvent(eventScanning, scanningEvent{
		ScanID:         scanID,
		Seq:            seq,
		Reason:         reason,
		StartedAt:      started,
		QueuedFollowUp: len(s.trigger) > 0,
	})
	if err != nil {
		s.failScan(scanID, seq, err)
		return
	}
	s.beginScan(scanningFrame)
	defer s.endScan()
	s.broadcast(scanningFrame)

	ctx, cancel := context.WithTimeout(s.rootCtx, s.cfg.Timeout)
	defer cancel()

	s.lastAttempt = started
	resp, err := s.check(ctx)
	if err != nil {
		s.failScan(scanID, seq, fmt.Errorf("%s scan: %w", reason, err))
		return
	}

	canon := Canonicalise(resp)
	hash := Hash(canon)
	transitions := Transitions(s.store.canon, canon)

	// Marshal ONCE, here, and store only bytes. Multiline would corrupt the
	// line-oriented SSE framing; EmitDefaultValues keeps a status key on
	// UNKNOWN nodes. Sanitise a copy first: protojson aborts the whole
	// message on one unresolvable detail type, which a remote satellite on a
	// newer build can hand us, and canon itself must stay untouched for Hash
	// and Transitions.
	payload, err := protojson.MarshalOptions{
		Multiline:         false,
		EmitDefaultValues: true,
	}.Marshal(SanitiseForMarshal(canon))
	if err != nil {
		s.failScan(scanID, seq, fmt.Errorf("marshal snapshot: %w", err))
		return
	}

	observedAt := time.Now()
	changed := hash != s.store.hash

	snapshotFrame, err := encodeEvent(eventSnapshot, snapshotEvent{
		Snapshot:    payload,
		ScanID:      scanID,
		Seq:         seq,
		ObservedAt:  observedAt,
		Transitions: transitions,
	})
	if err != nil {
		s.failScan(scanID, seq, err)
		return
	}

	scanFrame, err := encodeEvent(eventScan, scanEvent{
		ScanID:     scanID,
		Seq:        seq,
		Reason:     reason,
		ObservedAt: observedAt,
		DurationMs: observedAt.Sub(started).Milliseconds(),
		Changed:    changed,
	})
	if err != nil {
		s.failScan(scanID, seq, err)
		return
	}

	s.mu.Lock()
	s.store.payload = payload
	s.store.canon = canon
	s.store.hash = hash
	s.store.scanID = scanID
	s.store.seq = seq
	s.store.observedAt = observedAt
	s.store.transitions = transitions
	s.store.lastError = ""
	s.store.errorFrame = nil
	s.store.snapshotFrame = &snapshotFrame
	s.mu.Unlock()

	// Liveness on EVERY scan, changed or not: this is what makes suppressing
	// unchanged snapshots safe.
	s.broadcast(scanFrame)
	if changed {
		s.broadcast(snapshotFrame)
	}
}

// failScan records an error and broadcasts it. The previous snapshot is
// retained: a timeout is not evidence of unhealth.
func (s *Scanner) failScan(scanID string, seq uint64, err error) {
	f, encErr := encodeEvent(eventScanError, scanErrorEvent{
		ScanID: scanID,
		Seq:    seq,
		Code:   status.Code(err).String(),
		Error:  err.Error(),
		At:     time.Now(),
	})
	if encErr != nil {
		f = frame{eventScanError, []byte(`{"error":"scan failed and the error could not be encoded"}`)}
	}

	s.mu.Lock()
	s.store.lastError = err.Error()
	s.store.errorFrame = &f
	s.mu.Unlock()

	s.broadcast(f)
}

func (s *Scanner) beginScan(f frame) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.store.scanningFrame = &f
	s.store.scanState = ScanRunning
}

func (s *Scanner) endScan() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.store.scanningFrame = nil
	s.store.scanState = ScanIdle
}

func (s *Scanner) haveSnapshot() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.store.payload != nil
}

// broadcast fans a frame out to every subscriber. Scanner goroutine only.
func (s *Scanner) broadcast(f frame) {
	for sub := range s.subs {
		sub.deliver(f)
	}
}

// replayTo sends a new subscriber the full current state: connection, snapshot,
// current error and scan-in-progress. A tab connecting after a failed first
// scan must not see an unexplained blank page.
func (s *Scanner) replayTo(sub *Subscriber) {
	s.mu.RLock()
	replay := []*frame{
		s.store.connFrame,
		s.store.snapshotFrame,
		s.store.errorFrame,
		s.store.scanningFrame,
	}
	s.mu.RUnlock()

	for _, f := range replay {
		if f != nil {
			sub.deliver(*f)
		}
	}
}

func (s *Scanner) broadcastConnection(state connectivity.State) {
	if s.connSeeded && s.connLast == state {
		return
	}
	f, err := encodeEvent(eventConnection, newConnectionEvent(state, s.target, s.cfg.Refresh))
	if err != nil {
		// Mark as seen only after encoding, or a retry is deduped away.
		return
	}
	s.connSeeded, s.connLast = true, state

	s.mu.Lock()
	s.store.connFrame = &f
	s.mu.Unlock()

	s.broadcast(f)
}

// watchConnection feeds channel state into the loop's select rather than
// broadcasting directly, so the subscriber registry stays single-owner.
// GetState and WaitForStateChange are not experimental.
func (s *Scanner) watchConnection() {
	if s.conn == nil {
		return
	}
	for {
		state := s.conn.GetState()
		select {
		case s.connState <- state:
		case <-s.rootCtx.Done():
			return
		}
		// After Close the channel never changes state again, so a watcher that
		// kept waiting here would leak.
		if state == connectivity.Shutdown {
			return
		}
		if !s.conn.WaitForStateChange(s.rootCtx, state) {
			return
		}
	}
}

// floor is the minimum interval between auto-triggered scans.
func (s *Scanner) floor() time.Duration {
	return max(minScanInterval, s.cfg.Refresh)
}

// armTimer schedules the next auto-refresh, measured from scan completion.
func (s *Scanner) armTimer(t *time.Timer) {
	stopTimer(t)
	if s.cfg.Refresh > 0 && len(s.subs) > 0 {
		t.Reset(s.cfg.Refresh)
	}
}

func stopTimer(t *time.Timer) {
	if !t.Stop() {
		select {
		case <-t.C:
		default:
		}
	}
}
