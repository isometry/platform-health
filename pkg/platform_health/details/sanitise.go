package details

import (
	"fmt"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/wrapperspb"

	ph "github.com/isometry/platform-health/pkg/platform_health"
)

// SanitiseAny returns a untouched if protojson can resolve its type, or a
// deterministic stand-in built from nothing but its type URL if not.
// protojson resolves Any through the global type registry and aborts
// marshalling the whole enclosing message on the first miss, so a satellite
// on a newer build handing this binary a detail type it has never registered
// would otherwise blank an entire response. Callers walk their own tree and
// substitute per detail with this function before marshalling.
//
// The stand-in wraps a well-known, always-registered type so protojson can
// marshal it, and carries the original type URL in the value, the only clue
// left for whoever debugs it. a is never mutated.
//
// Resolvability is probed with a full UnmarshalNew rather than a registry
// lookup so a registered type carrying a corrupt payload is replaced too.
func SanitiseAny(a *anypb.Any) *anypb.Any {
	if a == nil {
		return a
	}
	if _, err := a.UnmarshalNew(); err == nil {
		return a
	}
	standIn, err := anypb.New(&wrapperspb.StringValue{
		Value: fmt.Sprintf("unresolved detail type: %s", a.GetTypeUrl()),
	})
	if err != nil {
		// wrapperspb.StringValue always marshals; unreachable in practice.
		return &anypb.Any{TypeUrl: a.GetTypeUrl()}
	}
	return standIn
}

// SanitiseResponse returns a deep copy of resp with every detail in the tree
// passed through SanitiseAny, ready for protojson. resp is never mutated.
func SanitiseResponse(resp *ph.HealthCheckResponse) *ph.HealthCheckResponse {
	if resp == nil {
		return nil
	}
	out := proto.Clone(resp).(*ph.HealthCheckResponse)
	sanitiseTree(out)
	return out
}

func sanitiseTree(n *ph.HealthCheckResponse) {
	for i, d := range n.GetDetails() {
		n.Details[i] = SanitiseAny(d)
	}
	for _, c := range n.GetComponents() {
		sanitiseTree(c)
	}
}
