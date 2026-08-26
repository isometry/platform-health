# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project

`platform-health` is a Go gRPC client/server for lightweight platform health monitoring. A server loads a YAML config of *components*, each backed by a compile-time-registered *provider* (tcp, http, kubernetes, helm, vault, …), probes them asynchronously, and returns an aggregated `HealthCheckResponse`. Module path: `github.com/isometry/platform-health`.

## Commands

```bash
go generate ./...                          # regenerate pkg/provider/kubernetes/common_generated.go
go test ./...                              # full suite (CI runs `go generate ./...` first)
go test ./pkg/provider/http/... -run TestHTTP -v   # single package / single test
go build ./cmd/ph && ./ph server -l -vv    # run a local server
make protoc                                # regenerate *.pb.go from proto/ (needs protoc + protoc-gen-go[-grpc])
make build                                 # goreleaser snapshot build (also runs protoc)
make ko-build                              # container image of ./cmd/phs via ko
```

Generated files (`*.pb.go`, `common_generated.go`) **are committed**. Regenerate and commit them when the corresponding `proto/*.proto` or `pkg/provider/kubernetes/common/generator.go` changes. There is no lint target; `.pre-commit-config.yaml` only runs kubeconform against the Helm chart.

## Architecture

### Four binaries, one command tree

`cmd/ph` is the unified CLI. `cmd/phc`, `cmd/phs` and `cmd/phui` splice `"client"` / `"server"` / `"ui"` into `os.Args` before calling `root.New()`, so each reaches exactly one subcommand: `ph ui` is unreachable from `phc` and `phs`. **Adding a provider means editing `cmd/ph` and `cmd/phs`** (`phc` and `phui` deliberately import only `details`).

Subcommands live in `pkg/commands/{root,client,server,check,context,migrate,validate,ui}`. `root.New()` creates the single owned viper instance and stores it in the command context; `check` and `context` additionally generate one subcommand *per registered provider* at startup via `pkg/commands/shared`, with flags derived by reflection from the provider struct (`provider.ProviderFlags`).

### Provider plugin system (`pkg/provider`)

- `registry.go`: `Register(type, new(Component))` in each provider's `init()` stores a `reflect.Type`.
- `factory.go`: `NewInstance(type, opts...)` reflects a fresh instance, decodes `spec` via mapstructure (or CLI flags via `WithFlags`), wires `components`/`checks`/`timeout`/`order`/`always`, then calls `Setup()`. `KnownComponentKeys` is the authoritative list of component-level YAML keys; unknown *spec* keys surface as a non-fatal `UnusedKeysWarning` (returned alongside a valid instance, so callers must use `errors.As`, not `err != nil`).
- `provider.go`: the `Instance` interface, the `Base` struct providers embed (name/timeout/order/always), and `Check()`, which groups instances by `order`, runs each group through an errgroup (parallelism from context), and implements fail-fast plus `always` instances that run on the *parent* context so they survive cancellation.
- Optional interfaces: `Container` (nested components; embed `BaseContainer`, call `ResolveComponents()` in `Setup()`) and `InstanceWithChecks` (CEL; embed `BaseWithChecks`, call `SetChecksAndCompile`). Detect them with `AsContainer` / `AsInstanceWithChecks`, never a bare type switch.
- `GetOrder()`/`GetAlways()` are *not* on the `Instance` interface. They are read via ad-hoc type assertions (`orderOf`/`alwaysOf`), so wrappers like `server.filteredInstance` silently lose them.

See `pkg/provider/README.md` for the full contract; `pkg/provider/tcp` is the smallest complete example.

### Request flow

`server.Check` → loop detection via `hops` (returns `LOOP_DETECTED` + `Detail_Loop`) → push hops/fail-fast/parallelism into context (`pkg/phctx`) → `filterInstances` for `-c name/sub/path` selection (sub-paths ride along in the context as `ComponentPaths`, consumed one level at a time by the `system` provider) → `provider.Check` → aggregate status (highest `Status` number wins). `ph check` reuses the same server code path in-process instead of dialing.

### Config (`pkg/config`)

Everything lives under a top-level `components:` key. `Load()` reads via viper, runs `ProcessIncludes` (recursive `includes:`, depth-capped, cycle-detected by content hash, maps merged / lists concatenated), then `harden()` turns raw maps into instances. In non-strict mode invalid instances are logged and skipped; `--strict` collects them into `ValidationErrors()`. The server watches the file with fsnotify and hot-reloads under a `sync.RWMutex`, rolling back to the previous config if the new one fails to load (or fails strict validation).

### Cross-cutting

- **`pkg/phctx`** owns every context key (viper, hops, fail-fast, parallelism, component paths) and the logger accessor. Use `phctx.Logger(ctx, attrs...)`, never `slog.Default()` directly in a provider.
- **viper uses `::` as its key delimiter** (`phctx.NewViper()`) so component names may contain dots (`google.com`). Never construct a plain `viper.New()`.
- **`pkg/checks`** wraps cel-go: one package-level `*CEL` env per provider declaring its variables, with a compiled-AST cache. `mode: "each"` iterates collections via `WithIterationKeys`. `ph context <component>` dumps the evaluation map, the debugging entry point for expressions.
- **`internal/output`** has a formatter registry (`json`, `yaml`, `junit`) populated by `init()`; `pkg/platform_health/details` renders `google.protobuf.Any` details by type-URL suffix, so a new detail type needs a `Render` method plus a case in `RenderAny`.
- **`pkg/ui`** backs `ph ui`: a gRPC client of a running server that serves a dashboard over SSE, built with no change to the server. It is the repo's first `go:embed` and first HTTP server, and `pkg/ui/assets` its first browser code (hence `javascript` in the CodeQL matrix).
- `pkg/provider/mock` and `pkg/provider/kubernetes/testutil` (fake dynamic client, REST mapper and resource builders) are the standard test seams. No cluster is required to run the suite.

## Conventions

- Providers name their struct `Component`, expose `const ProviderType`, embed `provider.Base`, implement `LogValue()` for structured logging, and set defaults in `Setup()` (via `go-defaults` struct tags plus an explicit `DefaultTimeout`).
- Provider docs are per-package `README.md` files (`pkg/provider/<name>/README.md`) listing spec fields and CEL variables. Update them alongside code, and link new providers from the root `README.md` list.
- Config keys are snake_case in YAML and bound with `mapstructure:"..."` tags.

## Working notes

Long-running work keeps a ledger at
`.superpowers/sdd/<plan-name>/progress.md`: every decision made on the user's
behalf, the measured baselines behind it, and what it costs if wrong. Read the
ledger for the current plan before starting, and append to it as you go. It is
git-ignored, so it survives compaction but not a fresh clone or `git clean -fdx`;
when it is missing, `git log` is the fallback.

The dashboard's ledger is `.superpowers/sdd/2026-08-26-ph-ui-dashboard/progress.md`.
It also carries findings parked for later, including the kubernetes provider
inheriting client-go's 5 QPS default with one shared token bucket per context.

### Rules that came out of that work

- **Verify by measuring, not by reading reports.** Several reports on the
  dashboard branch asserted things that measurement disproved, in both
  directions. Numbers, not impressions.
- **Never `git checkout`, `switch`, `stash` or `clean` in a shared tree.** A
  reviewer once left the tree on `main` with a whole package missing. Use a
  detached worktree and remove it.
- **Bind verification servers to loopback, with the kill in the same command
  chain.** A stray dashboard once ran for two hours on all interfaces, and
  another answered a verification run and produced a false pass.
- **Drive browser tests with real `page.mouse` and `page.keyboard`.** Synthetic
  `PointerEvent` dispatch has passed here while real input failed, and it leaves
  stale pointer-capture state that corrupts later real-input tests on the page.
- **`pkg/ui/testdata/fixture.json` is not representative.** It is the only source
  of detail payloads and `failFastTriggered`, but its names are plain. Real
  estates are larger and full of `@`. Check both.

### Dashboard UI decisions, settled

Names are never truncated; scrolling, collapsing and zooming are the answer.
Status colour outranks every other emphasis. On a graph edge the precedence is
status, then route, then duration, and route owns stroke width. The graph is
laid out left to right because the tree is shallow and wide.
