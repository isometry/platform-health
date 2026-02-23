package phctx

import (
	"context"
	"log/slog"
	"runtime"

	"github.com/spf13/viper"
	slogctx "github.com/veqryn/slog-context"
)

// Context key type - struct to avoid collisions with other packages
type contextKey struct{ name string }

var (
	viperKey          = contextKey{"viper"}
	failFastKey       = contextKey{"failFast"}
	parallelismKey    = contextKey{"parallelism"}
	hopsKey           = contextKey{"hops"}
	componentPathsKey = contextKey{"componentPaths"}
)

// Logger returns a logger from context with additional attributes
func Logger(ctx context.Context, args ...any) *slog.Logger {
	return slogctx.FromCtx(ctx).With(args...)
}

// Viper context helpers

// NewViper creates an owned viper instance with :: delimiter.
// The :: delimiter allows dots in component names (e.g., google.com).
func NewViper() *viper.Viper {
	return viper.NewWithOptions(viper.KeyDelimiter("::"))
}

// ContextWithViper returns a context with viper instance stored
func ContextWithViper(ctx context.Context, v *viper.Viper) context.Context {
	return context.WithValue(ctx, viperKey, v)
}

// Viper returns the viper instance from context.
// Panics if viper was not set - this is a programming error.
func Viper(ctx context.Context) *viper.Viper {
	v, ok := ctx.Value(viperKey).(*viper.Viper)
	if !ok {
		panic("viper not found in context - must call ContextWithViper first")
	}
	return v
}

// Fail-fast context helpers

// ContextWithFailFast returns a context with fail-fast behavior enabled/disabled
func ContextWithFailFast(ctx context.Context, failFast bool) context.Context {
	return context.WithValue(ctx, failFastKey, failFast)
}

// FailFastFromContext returns the fail-fast setting from context
func FailFastFromContext(ctx context.Context) bool {
	if v, ok := ctx.Value(failFastKey).(bool); ok {
		return v
	}
	return false
}

// Parallelism context helpers

// ContextWithParallelism returns a context with parallelism limit set
func ContextWithParallelism(ctx context.Context, limit int) context.Context {
	return context.WithValue(ctx, parallelismKey, limit)
}

// ParallelismFromContext returns the parallelism limit from context (default 0 = GOMAXPROCS)
func ParallelismFromContext(ctx context.Context) int {
	if v, ok := ctx.Value(parallelismKey).(int); ok {
		return v
	}
	return 0
}

// ParallelismLimit converts the parallelism setting to an actual limit value.
// Returns -1 for unlimited (don't call SetLimit), or a positive number.
func ParallelismLimit(limit int) int {
	if limit == 0 {
		return runtime.GOMAXPROCS(0)
	}
	return limit
}

// Hops context helpers for loop detection

// Hops represents IDs of the platform-health servers that have been visited
type Hops []string

// ContextWithHops returns a context with hops for loop detection
func ContextWithHops(ctx context.Context, hops Hops) context.Context {
	return context.WithValue(ctx, hopsKey, hops)
}

// HopsFromContext returns the hops from context
func HopsFromContext(ctx context.Context) Hops {
	if hops, ok := ctx.Value(hopsKey).(Hops); ok {
		return hops
	}
	return Hops{}
}

// ComponentPaths context helpers for hierarchical filtering

// ComponentPaths represents hierarchical component paths for filtering health checks
type ComponentPaths [][]string // Each path like ["system", "subcomponent"]

// ContextWithComponentPaths returns a context with component paths for filtering
func ContextWithComponentPaths(ctx context.Context, paths ComponentPaths) context.Context {
	return context.WithValue(ctx, componentPathsKey, paths)
}

// ComponentPathsFromContext returns the component paths from context
func ComponentPathsFromContext(ctx context.Context) ComponentPaths {
	if paths, ok := ctx.Value(componentPathsKey).(ComponentPaths); ok {
		return paths
	}
	return nil
}
