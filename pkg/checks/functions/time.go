package functions

import (
	"time"

	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/common/types"
	"github.com/google/cel-go/common/types/ref"
)

// Time returns CEL functions for time operations.
func Time() []cel.EnvOption {
	return []cel.EnvOption{
		// time.Now() - returns current timestamp
		cel.Function("time.Now",
			cel.Overload("time_now",
				[]*cel.Type{},
				cel.TimestampType,
				cel.FunctionBinding(func(args ...ref.Val) ref.Val {
					return types.Timestamp{Time: time.Now()}
				}),
			),
		),
		// time.Since(timestamp) - returns duration since the given timestamp
		cel.Function("time.Since",
			cel.Overload("time_since_timestamp",
				[]*cel.Type{cel.TimestampType},
				cel.DurationType,
				cel.UnaryBinding(func(arg ref.Val) ref.Val {
					ts := arg.(types.Timestamp)
					return types.Duration{Duration: time.Since(ts.Time)}
				}),
			),
		),
		// time.Until(timestamp) - returns duration until the given timestamp
		cel.Function("time.Until",
			cel.Overload("time_until_timestamp",
				[]*cel.Type{cel.TimestampType},
				cel.DurationType,
				cel.UnaryBinding(func(arg ref.Val) ref.Val {
					ts := arg.(types.Timestamp)
					return types.Duration{Duration: time.Until(ts.Time)}
				}),
			),
		),
	}
}
