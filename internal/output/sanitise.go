package output

import (
	"google.golang.org/protobuf/proto"

	ph "github.com/isometry/platform-health/pkg/platform_health"
	"github.com/isometry/platform-health/pkg/platform_health/details"
)

// sanitiseForMarshal returns a copy of status with every detail protojson
// cannot resolve replaced by a resolvable stand-in. protojson resolves Any
// through the global type registry and aborts marshalling the whole message
// on the first miss, so a remote satellite on a newer build handing this
// binary an unregistered detail type would otherwise blank the whole
// response. status is never mutated.
func sanitiseForMarshal(status *ph.HealthCheckResponse) *ph.HealthCheckResponse {
	if status == nil {
		return nil
	}
	out := proto.Clone(status).(*ph.HealthCheckResponse)
	sanitiseDetailsTree(out)
	return out
}

func sanitiseDetailsTree(n *ph.HealthCheckResponse) {
	for i, d := range n.GetDetails() {
		n.Details[i] = details.SanitiseAny(d)
	}
	for _, c := range n.GetComponents() {
		sanitiseDetailsTree(c)
	}
}
