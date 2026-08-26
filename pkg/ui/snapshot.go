package ui

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"io"
	"slices"
	"strings"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"

	ph "github.com/isometry/platform-health/pkg/platform_health"
	"github.com/isometry/platform-health/pkg/platform_health/details"
)

// Transition records one component's status change between two scans.
type Transition struct {
	Path string `json:"path"`
	From string `json:"from"`
	To   string `json:"to"`
}

// RootPath is the Transitions key for the root, which has no name. Escaping
// below only replaces bytes, never removes them, so a real PathKey output is
// never empty and a "/" separator never appears next to an empty segment.
// A real path is therefore never exactly "/", which is why it is safe here.
const RootPath = "/"

// PathKey joins a parent path and a component name, escaping only % and /, in
// that order. The browser recomputes these keys with the identical two
// replacements, so the scheme is deliberately minimal: it covers the separator
// and its own escape character and nothing else. Do not widen it, and do not
// reach for url.PathEscape or encodeURIComponent, which escape different sets
// and would make server and client disagree for names like "ssh@localhost",
// with no error on either side.
func PathKey(parent, name string) string {
	escaped := strings.ReplaceAll(name, "%", "%25")
	escaped = strings.ReplaceAll(escaped, "/", "%2F")
	if parent == "" {
		return escaped
	}
	return parent + "/" + escaped
}

// Canonicalise returns a deep copy, unmutated, with children sorted by (name, type),
// tied on each child's own content digest so identical siblings don't depend on arrival order.
func Canonicalise(resp *ph.HealthCheckResponse) *ph.HealthCheckResponse {
	if resp == nil {
		return nil
	}
	out := proto.Clone(resp).(*ph.HealthCheckResponse)
	sortTree(out)
	return out
}

// SanitiseForMarshal returns a deep copy with every detail protojson cannot
// resolve replaced by a stand-in that marshals. protojson resolves Any
// through the global type registry and aborts marshalling the whole message
// on the first miss, so one child with an unregistered detail type would
// otherwise blank the entire snapshot, healthy siblings included.
//
// The input is never mutated. It is typically the canonicalised tree, which
// the scanner also uses for Hash and Transitions; those already tolerate an
// unresolvable detail (writeDetail falls back to the raw type URL and bytes,
// and Transitions never looks at details at all), so only the copy destined
// for the wire needs this pass.
func SanitiseForMarshal(resp *ph.HealthCheckResponse) *ph.HealthCheckResponse {
	if resp == nil {
		return nil
	}
	out := proto.Clone(resp).(*ph.HealthCheckResponse)
	sanitiseTree(out)
	return out
}

func sanitiseTree(n *ph.HealthCheckResponse) {
	for i, d := range n.GetDetails() {
		n.Details[i] = details.SanitiseAny(d)
	}
	for _, c := range n.GetComponents() {
		sanitiseTree(c)
	}
}

func sortTree(n *ph.HealthCheckResponse) {
	for _, c := range n.Components {
		sortTree(c)
	}
	type child struct {
		node   *ph.HealthCheckResponse
		digest []byte
	}
	children := make([]child, len(n.Components))
	for i, c := range n.Components {
		children[i] = child{c, nodeDigest(c)}
	}
	slices.SortFunc(children, func(a, b child) int {
		if c := strings.Compare(a.node.GetName(), b.node.GetName()); c != 0 {
			return c
		}
		if c := strings.Compare(a.node.GetType(), b.node.GetType()); c != 0 {
			return c
		}
		return bytes.Compare(a.digest, b.digest)
	})
	for i, c := range children {
		n.Components[i] = c.node
	}
}

// Hash digests a canonicalised tree, excluding duration (it jitters every scan)
// and including details (so a cert rotation or kstatus flip under an unchanged status is seen).
func Hash(resp *ph.HealthCheckResponse) string {
	return hex.EncodeToString(nodeDigest(resp))
}

// nodeDigest hashes a node's content plus its children's digests, length-prefixing
// every field so concatenation can't collide. Also used by sortTree as a sibling tiebreaker.
func nodeDigest(n *ph.HealthCheckResponse) []byte {
	h := sha256.New()
	if n == nil {
		return h.Sum(nil)
	}
	writeString(h, n.GetName())
	writeString(h, n.GetType())
	writeString(h, n.GetStatus().String())
	writeBool(h, n.GetFailFastTriggered())
	writeBool(h, n.ServerId != nil)
	writeString(h, n.GetServerId())

	// Message order is meaningful: CEL failures append in expression order.
	writeUint64(h, uint64(len(n.GetMessages())))
	for _, m := range n.GetMessages() {
		writeString(h, m)
	}

	detailList := n.GetDetails()
	writeUint64(h, uint64(len(detailList)))
	for _, d := range detailList {
		writeDetail(h, d)
	}

	children := n.GetComponents()
	writeUint64(h, uint64(len(children)))
	for _, c := range children {
		h.Write(nodeDigest(c))
	}
	return h.Sum(nil)
}

// writeDetail hashes a sanitised, deterministic re-marshalling, not raw wire bytes,
// since raw bytes carry volatile fields like DNS TTL. Unregistered types fall back to the raw envelope.
func writeDetail(w io.Writer, a *anypb.Any) {
	msg, err := a.UnmarshalNew()
	if err != nil {
		writeString(w, a.GetTypeUrl())
		writeBytes(w, a.GetValue())
		return
	}
	sanitiseDetail(msg)
	b, err := proto.MarshalOptions{Deterministic: true}.Marshal(msg)
	if err != nil {
		writeString(w, a.GetTypeUrl())
		writeBytes(w, a.GetValue())
		return
	}
	writeString(w, a.GetTypeUrl())
	writeBytes(w, b)
}

// sanitiseDetail zeroes fields known to change between polls without a real
// status change. Add a case here when a new detail type gains a volatile field.
func sanitiseDetail(msg proto.Message) {
	switch d := msg.(type) {
	case *details.Detail_DNS:
		for _, r := range d.GetRecords() {
			r.Ttl = 0
		}
		// Resolver RR-set order is not guaranteed stable across polls; sort so
		// order alone does not register as a change.
		slices.SortFunc(d.Records, func(a, b *details.DNSRecord) int {
			if c := strings.Compare(a.GetType(), b.GetType()); c != 0 {
				return c
			}
			return strings.Compare(a.GetValue(), b.GetValue())
		})
	}
}

func writeUint64(w io.Writer, v uint64) {
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], v)
	w.Write(buf[:])
}

func writeBool(w io.Writer, v bool) {
	if v {
		w.Write([]byte{1})
	} else {
		w.Write([]byte{0})
	}
}

func writeBytes(w io.Writer, b []byte) {
	writeUint64(w, uint64(len(b)))
	w.Write(b)
}

func writeString(w io.Writer, s string) {
	writeBytes(w, []byte(s))
}

// Transitions reports per-path status changes between two canonicalised trees.
// A path in only one tree gets an empty From or To; the root is reported under RootPath.
func Transitions(prev, next *ph.HealthCheckResponse) []Transition {
	before, after := map[string]string{}, map[string]string{}
	collectStatuses(before, "", prev)
	collectStatuses(after, "", next)

	var out []Transition
	for path, to := range after {
		if from, ok := before[path]; !ok {
			out = append(out, Transition{Path: path, From: "", To: to})
		} else if from != to {
			out = append(out, Transition{Path: path, From: from, To: to})
		}
	}
	for path, from := range before {
		if _, ok := after[path]; !ok {
			out = append(out, Transition{Path: path, From: from, To: ""})
		}
	}
	slices.SortFunc(out, func(a, b Transition) int { return strings.Compare(a.Path, b.Path) })
	return out
}

func collectStatuses(m map[string]string, parent string, n *ph.HealthCheckResponse) {
	if n == nil {
		return
	}
	path := parent
	switch {
	case n.GetName() != "":
		path = PathKey(parent, n.GetName())
		m[path] = n.GetStatus().String()
	case parent == "":
		m[RootPath] = n.GetStatus().String()
	}
	for _, c := range n.GetComponents() {
		collectStatuses(m, path, c)
	}
}
