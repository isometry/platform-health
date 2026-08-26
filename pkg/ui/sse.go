package ui

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"time"
)

// heartbeatInterval keeps intermediaries and idle-timeout proxies from closing
// a quiet connection between events.
const heartbeatInterval = 20 * time.Second

// writeDeadline bounds every write. Without one, a backgrounded tab that stops
// reading fills the send buffer and parks the handler in write(2) forever: the
// request context is NOT cancelled while the connection stays open, so the
// goroutine leaks and blocks shutdown.
const writeDeadline = 10 * time.Second

// WriteEvent writes one SSE frame. Every line of data is prefixed, because SSE
// framing is line-oriented: an unprefixed line is treated as an unknown field
// and silently dropped by the browser's parser, with no error anywhere.
func WriteEvent(w io.Writer, event string, data []byte) error {
	if _, err := fmt.Fprintf(w, "event: %s\n", event); err != nil {
		return err
	}
	for _, line := range bytes.Split(data, []byte("\n")) {
		if _, err := fmt.Fprintf(w, "data: %s\n", line); err != nil {
			return err
		}
	}
	_, err := fmt.Fprint(w, "\n")
	return err
}

// sseHandler streams scan results to one browser. The subscriber's own
// goroutine is the sole writer, since http.ResponseWriter is not safe for
// concurrent use.
func (s *Scanner) sseHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Type", "text/event-stream")
		h.Set("Cache-Control", "no-cache, no-transform") // no-transform stops a compressing proxy from buffering
		h.Set("Connection", "keep-alive")
		h.Set("X-Accel-Buffering", "no")
		h.Set("X-Content-Type-Options", "nosniff")

		rc := http.NewResponseController(w)

		// Lengthen the browser's reconnect interval from its 3s default. This
		// runs before Subscribe, so no Unsubscribe is registered yet: the
		// deadline here is the only backstop against a client that never reads,
		// and both the write and the flush must be checked or that backstop is
		// silently defeated.
		if err := rc.SetWriteDeadline(time.Now().Add(writeDeadline)); err != nil {
			return
		}
		if _, err := fmt.Fprint(w, "retry: 5000\n\n"); err != nil {
			return
		}
		if err := rc.Flush(); err != nil {
			return
		}

		// Headers and retry must be flushed before Subscribe: register is
		// unbuffered and runScan runs inline in the scanner's select, so
		// Subscribe can block for a full scan timeout (30s default). Without
		// this ordering, a browser opening a tab mid-scan sees a hung request.
		sub := s.Subscribe()
		defer s.Unsubscribe(sub)

		heartbeat := time.NewTicker(heartbeatInterval)
		defer heartbeat.Stop()

		for {
			select {
			case <-r.Context().Done():
				return

			case <-s.Released():
				writeFarewell(w, rc)
				return

			case <-s.Done():
				writeFarewell(w, rc)
				return

			case _, ok := <-sub.doorbell:
				// The hub closes subscribers at shutdown, and a closed channel
				// is permanently ready, so a bare receive here would busy-loop
				// and burn a core. Read two-valued and return once closed.
				if !ok {
					writeFarewell(w, rc)
					return
				}
				_ = rc.SetWriteDeadline(time.Now().Add(writeDeadline))
				for _, f := range sub.take() {
					if err := WriteEvent(w, f.event, f.data); err != nil {
						return
					}
				}
				if err := rc.Flush(); err != nil {
					return
				}

			case <-heartbeat.C:
				_ = rc.SetWriteDeadline(time.Now().Add(writeDeadline))
				if _, err := fmt.Fprint(w, ":heartbeat\n\n"); err != nil {
					return
				}
				if err := rc.Flush(); err != nil {
					return
				}
			}
		}
	}
}

// writeFarewell tells the browser this close was deliberate, so it does not
// reconnect every 5s against a dead port. Shutdown cancels rootCtx before
// calling Release, so Done and the closed doorbell are ready first and Released
// often never wins the select: every shutdown path has to offer the farewell.
// Errors are ignored: the caller returns either way.
func writeFarewell(w http.ResponseWriter, rc *http.ResponseController) {
	_ = rc.SetWriteDeadline(time.Now().Add(writeDeadline))
	_ = WriteEvent(w, "shutdown", []byte("{}"))
	_ = rc.Flush()
}
