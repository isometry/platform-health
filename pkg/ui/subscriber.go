//go:build ui

package ui

import "sync"

// subscriberRingSize bounds the per-subscriber transient event backlog.
const subscriberRingSize = 64

// frame is one encoded SSE event awaiting a subscriber's writer.
type frame struct {
	event string
	data  []byte
}

// Subscriber is one connected SSE client. The hub sets state here and rings the
// doorbell; it never writes bytes. The handler goroutine is the sole writer for
// its subscriber, since http.ResponseWriter is not safe for concurrent use.
//
// Lock order is scanner then subscriber, never the reverse. Only the hub closes
// a subscriber, on unregister or scanner exit; the handler must never close the
// doorbell, and must read it two-valued and return once it is closed.
type Subscriber struct {
	doorbell chan struct{}

	mu sync.Mutex
	// pending is REPLACED, not queued: snapshots are full state, so dropping an
	// intermediate is correct.
	pending    frame
	hasPending bool
	// events is a bounded drop-oldest ring of transient events.
	events []frame
	closed bool
}

func newSubscriber() *Subscriber {
	return &Subscriber{doorbell: make(chan struct{}, 1)}
}

// deliver queues a frame and rings the doorbell without blocking.
func (sub *Subscriber) deliver(f frame) {
	sub.mu.Lock()
	defer sub.mu.Unlock()

	if sub.closed {
		return
	}
	if f.event == eventSnapshot {
		sub.pending, sub.hasPending = f, true
	} else {
		if len(sub.events) == subscriberRingSize {
			copy(sub.events, sub.events[1:])
			sub.events = sub.events[:subscriberRingSize-1]
		}
		sub.events = append(sub.events, f)
	}

	// The doorbell send stays under the lock. A non-blocking send on a cap-1
	// channel cannot block, and unlocking first would let a concurrent close
	// turn this into a send on a closed channel.
	select {
	case sub.doorbell <- struct{}{}:
	default:
	}
}

// take removes and returns the queued frames: the latest snapshot first, then
// transient events in arrival order.
func (sub *Subscriber) take() []frame {
	sub.mu.Lock()
	defer sub.mu.Unlock()

	out := make([]frame, 0, len(sub.events)+1)
	if sub.hasPending {
		out = append(out, sub.pending)
		sub.pending, sub.hasPending = frame{}, false
	}
	out = append(out, sub.events...)
	sub.events = sub.events[:0]
	return out
}

// close is idempotent and reserved to the hub.
func (sub *Subscriber) close() {
	sub.mu.Lock()
	defer sub.mu.Unlock()

	if sub.closed {
		return
	}
	sub.closed = true
	sub.pending, sub.hasPending, sub.events = frame{}, false, nil
	close(sub.doorbell)
}
