package ui_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/durationpb"

	ph "github.com/isometry/platform-health/pkg/platform_health"
	"github.com/isometry/platform-health/pkg/platform_health/details"
	"github.com/isometry/platform-health/pkg/ui"
)

// marshalOpts mirrors the options the scanner uses to build the SSE snapshot
// payload, so these tests exercise the same marshalling behaviour it relies on.
var marshalOpts = protojson.MarshalOptions{Multiline: false, EmitDefaultValues: true}

// unknownTypeURL names a detail type this binary has never registered, the
// same shape a rolling upgrade of a satellite server can hand us.
const unknownTypeURL = "type.googleapis.com/platform_health.detail.v1.Detail_FromTheFuture"

func mustUnknownAny(t *testing.T) *anypb.Any {
	t.Helper()
	return &anypb.Any{TypeUrl: unknownTypeURL, Value: []byte{0x0a, 0x02, 0x68, 0x69}}
}

func node(name, typ string, status ph.Status, children ...*ph.HealthCheckResponse) *ph.HealthCheckResponse {
	return &ph.HealthCheckResponse{Name: name, Type: typ, Status: status, Components: children}
}

func mustAny(t *testing.T, msg proto.Message) *anypb.Any {
	t.Helper()
	a, err := anypb.New(msg)
	require.NoError(t, err)
	return a
}

func findByName(list []*ph.HealthCheckResponse, name string) *ph.HealthCheckResponse {
	for _, c := range list {
		if c.GetName() == name {
			return c
		}
	}
	return nil
}

func TestHashIgnoresChildOrder(t *testing.T) {
	// Component order is nondeterministic: GetInstances ranges a map and
	// results arrive in goroutine completion order.
	a := node("", "", ph.Status_HEALTHY,
		node("alpha", "tcp", ph.Status_HEALTHY),
		node("beta", "http", ph.Status_UNHEALTHY),
	)
	b := node("", "", ph.Status_HEALTHY,
		node("beta", "http", ph.Status_UNHEALTHY),
		node("alpha", "tcp", ph.Status_HEALTHY),
	)
	assert.Equal(t, ui.Hash(ui.Canonicalise(a)), ui.Hash(ui.Canonicalise(b)))
}

func TestHashTieDeterministic(t *testing.T) {
	// Same (name, type) siblings, different status, arriving in each order.
	// A comparator that ties on (name, type) alone is order dependent here.
	orderA := node("", "", ph.Status_HEALTHY,
		node("db", "tcp", ph.Status_HEALTHY),
		node("db", "tcp", ph.Status_UNHEALTHY),
	)
	orderB := node("", "", ph.Status_HEALTHY,
		node("db", "tcp", ph.Status_UNHEALTHY),
		node("db", "tcp", ph.Status_HEALTHY),
	)
	assert.Equal(t, ui.Hash(ui.Canonicalise(orderA)), ui.Hash(ui.Canonicalise(orderB)))
}

func TestCanonicaliseSortsRecursively(t *testing.T) {
	// Reorder grandchildren, not just top-level children.
	a := node("", "", ph.Status_HEALTHY,
		node("mid", "svc", ph.Status_HEALTHY,
			node("z", "tcp", ph.Status_HEALTHY),
			node("a", "tcp", ph.Status_UNHEALTHY),
		),
	)
	b := node("", "", ph.Status_HEALTHY,
		node("mid", "svc", ph.Status_HEALTHY,
			node("a", "tcp", ph.Status_UNHEALTHY),
			node("z", "tcp", ph.Status_HEALTHY),
		),
	)
	assert.Equal(t, ui.Hash(ui.Canonicalise(a)), ui.Hash(ui.Canonicalise(b)))
}

func TestHashIgnoresDuration(t *testing.T) {
	a := node("x", "tcp", ph.Status_HEALTHY)
	a.Duration = durationpb.New(89000000)
	b := node("x", "tcp", ph.Status_HEALTHY)
	b.Duration = durationpb.New(91000000)
	assert.Equal(t, ui.Hash(ui.Canonicalise(a)), ui.Hash(ui.Canonicalise(b)))
}

func TestHashDetectsStatusChange(t *testing.T) {
	a := node("x", "tcp", ph.Status_HEALTHY)
	b := node("x", "tcp", ph.Status_UNHEALTHY)
	assert.NotEqual(t, ui.Hash(ui.Canonicalise(a)), ui.Hash(ui.Canonicalise(b)))
}

func TestHashDetectsFailFastAndMessages(t *testing.T) {
	base := node("x", "tcp", ph.Status_UNHEALTHY)

	failFast := node("x", "tcp", ph.Status_UNHEALTHY)
	failFast.FailFastTriggered = true
	assert.NotEqual(t, ui.Hash(ui.Canonicalise(base)), ui.Hash(ui.Canonicalise(failFast)),
		"failFastTriggered means the tree is incomplete and must count as a change")

	withMsg := node("x", "tcp", ph.Status_UNHEALTHY)
	withMsg.Messages = []string{"connection refused"}
	assert.NotEqual(t, ui.Hash(ui.Canonicalise(base)), ui.Hash(ui.Canonicalise(withMsg)))
}

func TestHashMessageOrderSignificant(t *testing.T) {
	// CEL check failures append in expression order; reordering is a real change.
	a := node("x", "tcp", ph.Status_UNHEALTHY)
	a.Messages = []string{"first", "second"}
	b := node("x", "tcp", ph.Status_UNHEALTHY)
	b.Messages = []string{"second", "first"}
	assert.NotEqual(t, ui.Hash(ui.Canonicalise(a)), ui.Hash(ui.Canonicalise(b)))
}

func TestHashMessageConcatenationUnambiguous(t *testing.T) {
	// Guards against naive delimiter-free concatenation: ["ab","c"] must not
	// hash the same as ["a","bc"].
	a := node("x", "tcp", ph.Status_HEALTHY)
	a.Messages = []string{"ab", "c"}
	b := node("x", "tcp", ph.Status_HEALTHY)
	b.Messages = []string{"a", "bc"}
	assert.NotEqual(t, ui.Hash(ui.Canonicalise(a)), ui.Hash(ui.Canonicalise(b)))
}

func TestHashDetailsIncluded(t *testing.T) {
	a := node("x", "tls", ph.Status_HEALTHY)
	a.Details = []*anypb.Any{mustAny(t, &details.Detail_TLS{CommonName: "old.example.com"})}
	b := node("x", "tls", ph.Status_HEALTHY)
	b.Details = []*anypb.Any{mustAny(t, &details.Detail_TLS{CommonName: "new.example.com"})}
	assert.NotEqual(t, ui.Hash(ui.Canonicalise(a)), ui.Hash(ui.Canonicalise(b)))
}

func TestHashDetailsIgnoresDNSTTL(t *testing.T) {
	// TTL counts down between polls behind a caching resolver; it must not
	// register as a change.
	a := node("x", "dns", ph.Status_HEALTHY)
	a.Details = []*anypb.Any{mustAny(t, &details.Detail_DNS{
		Host:    "example.com",
		Records: []*details.DNSRecord{{Type: "A", Value: "1.2.3.4", Ttl: 60}},
	})}
	b := node("x", "dns", ph.Status_HEALTHY)
	b.Details = []*anypb.Any{mustAny(t, &details.Detail_DNS{
		Host:    "example.com",
		Records: []*details.DNSRecord{{Type: "A", Value: "1.2.3.4", Ttl: 30}},
	})}
	assert.Equal(t, ui.Hash(ui.Canonicalise(a)), ui.Hash(ui.Canonicalise(b)))
}

func TestHashServerIdPresence(t *testing.T) {
	// ServerId is optional string; nil and "" must not collide.
	unset := node("x", "tcp", ph.Status_HEALTHY)
	empty := node("x", "tcp", ph.Status_HEALTHY)
	empty.ServerId = proto.String("")
	assert.NotEqual(t, ui.Hash(ui.Canonicalise(unset)), ui.Hash(ui.Canonicalise(empty)))
}

func TestCanonicaliseDoesNotMutateInput(t *testing.T) {
	zulu := node("zulu", "tcp", ph.Status_HEALTHY)
	zulu.Messages = []string{"original"}
	zulu.Details = []*anypb.Any{mustAny(t, &details.Detail_TLS{CommonName: "original"})}
	in := node("", "", ph.Status_HEALTHY, zulu, node("alpha", "tcp", ph.Status_HEALTHY))

	out := ui.Canonicalise(in)
	outZulu := findByName(out.Components, "zulu")
	require.NotNil(t, outZulu)

	// Mutate the clone as deeply as possible: a shallow copy sharing slices
	// or byte backing arrays with the input would leak these writes back.
	outZulu.Name = "mutated"
	outZulu.Messages[0] = "mutated"
	outZulu.Details[0].Value[0] ^= 0xFF

	assert.Equal(t, "zulu", zulu.Name)
	assert.Equal(t, "original", zulu.Messages[0])
	want := mustAny(t, &details.Detail_TLS{CommonName: "original"})
	assert.Equal(t, want.Value, zulu.Details[0].Value)
}

func TestNilInputs(t *testing.T) {
	assert.Nil(t, ui.Canonicalise(nil))
	assert.NotPanics(t, func() { ui.Hash(nil) })
	assert.Equal(t, ui.Hash(nil), ui.Hash(nil))
	assert.NotPanics(t, func() { ui.Transitions(nil, nil) })
	assert.Empty(t, ui.Transitions(nil, nil))
}

func TestPathKey(t *testing.T) {
	tests := []struct {
		name          string
		parent, child string
		expected      string
	}{
		{"root child has no prefix", "", "google", "google"},
		{"nested", "fluxcd", "source-controller", "fluxcd/source-controller"},
		{"slash in name is escaped", "", "a/b", "a%2Fb"},
		{"escaped name cannot collide with nesting", "a", "b", "a/b"},
		{"percent is escaped", "", "100%", "100%25"},
		// These pass through unescaped on purpose: the client computes the same
		// key with encodeURIComponent, which escapes @ : and &, so agreement
		// here is the actual contract this scheme exists to guarantee.
		{"at sign passes through", "", "ssh@localhost", "ssh@localhost"},
		{"colon passes through", "", "svc:8080", "svc:8080"},
		{"ampersand passes through", "", "x&y", "x&y"},
		// url.PathEscape agrees with the minimal scheme on @ : & but not on
		// these, so they also guard against a reversion to it specifically.
		{"comma passes through", "", "a,b", "a,b"},
		{"space passes through", "", "a b", "a b"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, ui.PathKey(tt.parent, tt.child))
		})
	}
}

func TestRootPathCannotCollide(t *testing.T) {
	// Spread of adversarial names, including the old NUL based sentinel this
	// scheme replaced, none of which may ever produce the root sentinel itself.
	names := []string{"/", "\x00root", "", "%", "//", "a/", "/a"}
	for _, n := range names {
		assert.NotEqual(t, ui.RootPath, ui.PathKey("", n), "name %q must not produce the root sentinel", n)
		assert.NotEqual(t, ui.RootPath, ui.PathKey("parent", n), "name %q under a parent must not produce the root sentinel", n)
	}
}

func TestTransitions(t *testing.T) {
	prev := node("", "", ph.Status_HEALTHY, node("db", "tcp", ph.Status_HEALTHY))
	next := node("", "", ph.Status_UNHEALTHY, node("db", "tcp", ph.Status_UNHEALTHY))

	got := ui.Transitions(ui.Canonicalise(prev), ui.Canonicalise(next))

	assert.Contains(t, got, ui.Transition{Path: "db", From: "HEALTHY", To: "UNHEALTHY"})
}

func TestTransitionsIncludesRoot(t *testing.T) {
	prev := node("", "", ph.Status_HEALTHY, node("db", "tcp", ph.Status_HEALTHY))
	next := node("", "", ph.Status_UNHEALTHY, node("db", "tcp", ph.Status_UNHEALTHY))

	got := ui.Transitions(ui.Canonicalise(prev), ui.Canonicalise(next))

	assert.Contains(t, got, ui.Transition{Path: ui.RootPath, From: "HEALTHY", To: "UNHEALTHY"})
}

func TestTransitionsReportsDisappearance(t *testing.T) {
	prev := node("", "", ph.Status_HEALTHY, node("db", "tcp", ph.Status_HEALTHY))
	next := node("", "", ph.Status_HEALTHY)

	got := ui.Transitions(ui.Canonicalise(prev), ui.Canonicalise(next))

	assert.Contains(t, got, ui.Transition{Path: "db", From: "HEALTHY", To: ""})
}

func TestTransitionsSortedByPath(t *testing.T) {
	// Six names, so map iteration landing on sorted order by chance is
	// negligible (1/720): a real ordering bug reproduces reliably.
	names := []string{"zulu", "yankee", "xray", "whiskey", "victor", "uniform"}
	var prevKids, nextKids []*ph.HealthCheckResponse
	for _, name := range names {
		prevKids = append(prevKids, node(name, "tcp", ph.Status_HEALTHY))
		nextKids = append(nextKids, node(name, "tcp", ph.Status_UNHEALTHY))
	}
	prev := node("", "", ph.Status_HEALTHY, prevKids...)
	next := node("", "", ph.Status_HEALTHY, nextKids...)

	got := ui.Transitions(ui.Canonicalise(prev), ui.Canonicalise(next))
	paths := make([]string, len(got))
	for i, tr := range got {
		paths[i] = tr.Path
	}
	assert.Equal(t, []string{"uniform", "victor", "whiskey", "xray", "yankee", "zulu"}, paths)
}

func TestSanitiseForMarshalSurvivesUnknownDetail(t *testing.T) {
	// protojson resolves Any through the global registry and aborts marshalling
	// the whole message on the first miss. A remote satellite on a newer build
	// can hand us a detail type we have never registered, so one bad child must
	// not blank the healthy sibling or the root.
	tree := node("", "", ph.Status_UNHEALTHY,
		node("healthy-sibling", "tcp", ph.Status_HEALTHY),
		node("future", "satellite", ph.Status_UNHEALTHY),
	)
	tree.Components[1].Details = []*anypb.Any{mustUnknownAny(t)}

	canon := ui.Canonicalise(tree)

	_, errBefore := marshalOpts.Marshal(canon)
	require.Error(t, errBefore, "an unresolvable Any must still fail a direct marshal, or this test proves nothing")

	out, err := marshalOpts.Marshal(ui.SanitiseForMarshal(canon))
	require.NoError(t, err)
	assert.NotEmpty(t, out)
	assert.Contains(t, string(out), "healthy-sibling")
	assert.Contains(t, string(out), "future")
}

func TestSanitiseForMarshalKeepsTypeURLVisible(t *testing.T) {
	tree := node("future", "satellite", ph.Status_UNHEALTHY)
	tree.Details = []*anypb.Any{mustUnknownAny(t)}

	out, err := marshalOpts.Marshal(ui.SanitiseForMarshal(ui.Canonicalise(tree)))
	require.NoError(t, err)
	assert.Contains(t, string(out), unknownTypeURL)
}

func TestSanitiseForMarshalKnownDetailsByteIdentical(t *testing.T) {
	// Known types must pass through untouched: SanitiseForMarshal must not
	// change a single byte of what the scanner produces today.
	a := node("x", "tls", ph.Status_HEALTHY)
	a.Details = []*anypb.Any{mustAny(t, &details.Detail_TLS{CommonName: "example.com"})}
	b := node("", "", ph.Status_HEALTHY, a, node("clean", "tcp", ph.Status_HEALTHY))

	canon := ui.Canonicalise(b)

	before, err := marshalOpts.Marshal(canon)
	require.NoError(t, err)
	after, err := marshalOpts.Marshal(ui.SanitiseForMarshal(canon))
	require.NoError(t, err)

	assert.Equal(t, before, after)
}

func TestSanitiseForMarshalUnknownDetailDeterministic(t *testing.T) {
	// Two scans carrying the same unknown detail must produce identical bytes,
	// or the hash flaps and the UI reports change on every poll.
	build := func() *ph.HealthCheckResponse {
		tree := node("future", "satellite", ph.Status_UNHEALTHY)
		tree.Details = []*anypb.Any{mustUnknownAny(t)}
		return ui.Canonicalise(tree)
	}
	first, second := build(), build()

	outFirst, err := marshalOpts.Marshal(ui.SanitiseForMarshal(first))
	require.NoError(t, err)
	outSecond, err := marshalOpts.Marshal(ui.SanitiseForMarshal(second))
	require.NoError(t, err)

	assert.Equal(t, outFirst, outSecond)
	assert.Equal(t, ui.Hash(first), ui.Hash(second))

	// SanitiseForMarshal must not mutate the tree Hash and Transitions rely on.
	_, stillUnresolved := first.Details[0].UnmarshalNew()
	assert.Error(t, stillUnresolved)
}
