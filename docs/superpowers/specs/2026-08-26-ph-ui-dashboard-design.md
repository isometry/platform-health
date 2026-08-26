# `ph ui`: Live Platform Health Dashboard

**Date:** 2026-08-26
**Status:** Approved design, ready for implementation planning

---

## 1. Summary

Add `ph ui`: a new cobra subcommand that serves a web dashboard for a running
platform-health server. It is a **gRPC client**, not a server extension. It
polls the existing unary `Check` RPC, holds the latest result, and pushes it to
connected browsers over Server-Sent Events.

The dashboard presents two coordinated views of the same scan: a directed
graph of component topology and a hierarchical component tree, plus a manual
"Scan now" trigger and an optional auto-refresh.

---

## 2. Context and constraints

### What exists today

The server is gRPC-only. `pkg/commands/server/server.go:78` opens one TCP
listener and hands it to `grpc.Server.Serve`. There is no HTTP surface, no REST
API, and no `net/http` server anywhere in the repo. `net/http` is imported only
by the HTTP health-check *provider* (an outbound client) and two test files.

Critically, **nothing runs in the background.** `PlatformHealthServer.Check`
probes every configured component synchronously because a client asked, and
discards the result when the RPC returns. There is no scheduler, no cache, no
last-known status. A UI therefore has nothing to read; it must cause a scan to
learn anything.

`Check` is **unary** (`proto/platform_health.proto:10`). There is no streaming
or watch RPC, so the UI must poll and fan out client-side.

### Constraints set by the user

- The UI must be a **plugin**: essentially no impact on the existing codebase.
- No server-side interval scanner in v1 (deferred until the UI is proven).
- Sleek, modern UI/UX.
- SSE-driven: push only when there is something to push.
- A manual scan trigger.

---

## 3. Non-goals for v1

- No server-side background scanning or persistence. The server is untouched.
- No in-cluster deployment. `ph ui` is a local-use tool; the Helm chart is not
  modified. See §10.4.
- No authentication. Loopback binding plus origin checks; non-loopback binding
  is an explicit opt-in.
- No historical data, alerting, or notification.
- No server-side component scoping (`-c`). See §4.6.

---

## 4. Decisions and rationale

Each decision records what was rejected and why, so the reasoning survives.

### 4.1 A `ph ui` subcommand, not a separate binary or an embedded server

The UI dials a server the same way `ph client` does. It never imports
`pkg/server`, registers no providers, and reads no config file. Registration is
one line in `pkg/commands/root/root.go`.

Rejected: embedding an HTTP listener in `ph server` (modifies the server command
and its lifecycle, the opposite of the plugin constraint); a standalone binary
(same code, worse distribution).

Consequence: `ph ui` is reachable from the `ph` binary only. `cmd/phc` and
`cmd/phs` splice `"client"`/`"server"` into `os.Args` unconditionally, so
`phs ui` becomes `phs server ui` and is rejected by `netutil.ParseHostPort`.
This is expected and acceptable.

### 4.2 Plain JS consuming JSON over SSE, not HTMX

The deciding factor is **client-owned view state layered on constantly-changing
server state**: which nodes are expanded, what is selected, the graph's pan/zoom
transform, and the filter text must all survive every push.

Investigation findings (htmx 2.0.10, `htmx-ext-sse` 2.2.4, idiomorph 0.7.4):

- `hx-preserve` splices the **live DOM node** into the incoming fragment and
  discards the server's version. A preserved node keeps its expanded state but
  can never receive new status data: the wrong trade for a status tree.
- Idiomorph updates content correctly, but `open` and `class` are ordinary
  attributes to it and get overwritten from server HTML. Expansion and selection
  are stripped on every push.
- The documented workaround is a `beforeAttributeUpdated` callback written
  inline in `hx-swap="morph:{...}"`. This is a DOM reconciler policy authored
  inside an HTML attribute, and it **forces `unsafe-eval`**, because idiomorph
  calls `Function()` on that config itself, outside htmx's `allowEval` guard.
- Moving view state server-side makes expansion a network round-trip, makes
  state per-tab rather than per-session, and destroys SSE broadcast fan-out
  (one scan would render N ways for N viewers).
- A mixed architecture cannot share one connection: htmx's `EventSource` lives
  on an internal API, so the SVG canvas would open a second stream. Two streams,
  two reconciliation models, and each scan serialised twice (HTML + JSON).

Corrections to earlier assumptions, recorded so they are not re-litigated:
idiomorph **is** officially supported on htmx 2.x, it **does** compose with the
SSE extension in 2.2.4, and `hx-preserve` **does** work in OOB swaps. HTMX is
more capable here than folklore suggests; it is simply the wrong shape for this
particular UI. htmx's own documentation points at JavaScript for canvas work and
highly client-stateful applications.

Cost comparison: ~21 KB gzip for htmx + sse + morph, versus 0 KB for the native
`EventSource`. htmx 4 additionally absorbs both SSE and morphing back into core,
so both extensions have a known expiry as the recommended path.

**Adopted from the research:** htmx's genuinely good idea is *never repaint the
tree wholesale*: reconcile keyed by stable node identity. This is implemented
client-side, where it is free (§7.2).

### 4.3 Hand-rolled graph layout, no library

The data is a strict tree (§7.4), and collapse-by-default caps node count, so
Reingold-Tilford is roughly eighty lines. This avoids vendoring d3 or cytoscape,
keeps the CSP trivial, and lets the visual design be exact rather than fought
against a library's defaults.

### 4.4 Manual scan by default; auto-refresh opt-in

Scans on page load and on the button. An auto-refresh toggle exists in the UI
and as `--refresh`, defaulting to off. When enabled, the UI polls and pushes
only when something changed. The poll loop lives entirely in the new package, so
it is still zero modification to existing code.

### 4.5 The dial refactor lands in `pkg/client`, as its own commit

The implied-TLS dial logic exists at **four** sites today:

| Site | Behaviour |
|---|---|
| `pkg/commands/client/client.go:66-85` | viper-driven; TLS if `tls \|\| port==443 \|\| port==8443` |
| `pkg/provider/satellite/satellite.go:80-98` | struct-driven, same ports, **mutates `i.TLS`** |
| `pkg/provider/grpc/grpc.go:75-92` | struct-driven, **port 443 only** (divergent) |
| `pkg/client/client.go:18` | hardcoded `insecure.NewCredentials()`, no TLS at all |

`pkg/client` has **zero callers anywhere in the module** and forces insecure
credentials: dead code that is also a trap for anyone who finds it. Fixing it
retires the trap rather than adding a third util package beside `pkg/netutil`.

```go
type DialConfig struct { Host string; Port int; TLS, Insecure bool }
var TLSPorts = []int{443, 8443}
func (c DialConfig) Address() string { /* net.JoinHostPort */ }
func (c DialConfig) UseTLS() bool    { return c.TLS || slices.Contains(TLSPorts, c.Port) }
func Dial(cfg DialConfig, opts ...grpc.DialOption) (*grpc.ClientConn, error)
```

`pkg/provider/grpc`'s narrower port rule (443 only) is **aligned onto
`TLSPorts`**, gaining 8443. This is a deliberate, noted behaviour change: a
`grpc` component on port 8443 previously dialled without TLS and would have
failed against a TLS endpoint, so the change fixes a latent bug rather than
altering working behaviour. Call it out in the commit message and CHANGELOG.

**This removes a data race for free.** `satellite.go:80-82` and `grpc.go:74-76`
write `TLS = true` on instance structs that are shared across concurrent `Check`
RPCs (`pkg/config/config.go:84-95` hands out the same pointers every call).
`UseTLS()` is a pure function of immutable fields, so deleting the write is
observably identical *except* the race disappears. CI runs `go test` without
`-race`, so this is latent today.

`Dial` returns a `*grpc.ClientConn` and nothing more. It must **not** own
backoff, keepalive or idle policy: `ph ui` wants an aggressive reconnect ceiling
and keepalive, while satellite's per-check ephemeral connections want neither.
Policy is passed by the caller as `grpc.DialOption`s.

**Sequencing:** land this refactor as its own commit *before* the UI work, so
`ph ui` itself really is additive.

### 4.6 `-c/--component` is not offered

Three reasons, all verifiable:

1. A typo produces a **successful** RPC carrying `Status: UNHEALTHY`,
   `Messages: ["invalid components: foo"]`, no `Components` and no `Duration`
   (`pkg/server/server.go:195-201`). A misspelled scope renders an empty red
   screen indistinguishable from a total outage.
2. Scoped results are **not a subset** of unscoped results. `filterInstances`
   wraps matches in `filteredInstance`, which loses `GetOrder()`/`GetAlways()`
   (`pkg/server/server.go:88-98`; same at `pkg/provider/system/system.go:148-151`).
   Order groups collapse to a single wave and `always` loses its fail-fast
   immunity, so a component can report unhealthy purely because the view was
   narrowed.
3. The UI already holds the full snapshot, so filtering in the browser is
   instant, preserves ordering semantics, and cannot produce a phantom
   UNHEALTHY.

Filtering is therefore client-side only. If server-side scoping is ever wanted
to reduce probe load, `filteredInstance`/`filteredChild` should first be fixed
to forward `GetOrder`/`GetAlways`.

### 4.7 Visual direction: "Signal"

Near-black canvas, saturated status colour, subtle glow on live state; reads
across a room. Light mode translates glow into a tinted halo ring and darkens
hues for contrast on white: the same visual grammar in a different medium, not
an inversion.

Layout is graph-primary: the graph owns the canvas, the component tree is a
hierarchical rail on the left (indented, collapsible, with status, type and
timing), and detail docks along the bottom. Selection is shared three ways:
row, node, dock.

---

## 5. Architecture

### 5.1 Package layout

```
pkg/commands/ui/
  ui.go          cobra command, wiring, shutdown; blank-imports details (§6.5)
  ui_flags.go    flag definitions
  http.go        ServeMux, SSE handler, POST /api/scan, static assets, middleware
  scanner.go     connection ownership, trigger loop, subscriber registry
  snapshot.go    canonicalisation, path keying, hashing, change detection
  assets/
    index.html   shell + pre-paint theme script
    app.js       model, reconciler, tree renderer, graph renderer
    app.css      Signal palette as custom properties, light + dark
```

Assets are embedded with `go:embed` and served via `http.FileServerFS`. This is
the repo's first `go:embed` and first HTTP server: it establishes both patterns
rather than following them.

Binary cost is 20-60 KB on a ~65 MB binary (~0.05%). A build tag is not
justified. Note the linker *does* eliminate unreachable `embed.FS` data, but
`root.go` calls `ui.New()` unconditionally, so the assets are reachable and
retained in every `ph` and `phs` binary.

### 5.2 Command wiring

Follows the repo idiom exactly: a package-private
`var uiFlags = cliflags.Merge(...)` registered with
`uiFlags.Register(cmd.Flags(), false)`; `PreRunE: setup` calling
`phctx.Viper(cmd.Context())` and `cliflags.BindFlags(cmd, v)`; `RunE: run`.
`ph ui` needs no `config.Load()` (it is a pure client), so `setup` is the short
`client`-style variant.

Registration in `pkg/commands/root/root.go`: one import plus
`cmd.AddCommand(ui.New())`.

### 5.3 CLI surface

The target server is given as a positional `[host:port]` argument, exactly as
`ph client` and `ph server` do (via `netutil.ParseHostPort`), or via flags.

| Flag | Default | Meaning |
|---|---|---|
| `--server` / `-s` | `localhost` | target server host |
| `--port` / `-p` | `8080` | target server port |
| `--tls`, `--insecure` / `-k` | off | as `ph client`; TLS also implied by port 443/8443 |
| `--timeout` / `-t` | `30s` | per-scan deadline |
| `--listen` | `127.0.0.1:8090` | local HTTP socket, as `host:port` |
| `--refresh` | `0` (off) | auto-refresh interval |
| `--open` | off | open a browser on start |
| `--fixture` | none | serve a canned snapshot; no gRPC connection (§9.3) |

`--server`/`--port`/`--tls`/`--insecure` should be factored into a new
`cliflags.ClientFlags()` helper, used by both `client_flags.go` and
`ui_flags.go`, rather than becoming a third hand-rolled copy. The `30s` timeout
overrides `cliflags.TimeoutFlags()`'s `10s` via `Merge` (last-wins).

Two documented inconsistencies to call out in usage strings:

- `ph server`'s `--listen` has `NoOptDefault: "localhost"` and no default, so it
  binds **all** interfaces and `-l` narrows it. Ours defaults to loopback: the
  opposite polarity for the same flag name in a sibling command.
- `--refresh 0` meaning *off* conflicts with the repo's "0 means default"
  idiom (`parallelism: 0` → GOMAXPROCS; `GetTimeout() == 0` → provider default).
  Document explicitly.

`--open` must be implemented with a small `exec.Command` switch on
`runtime.GOOS` (`open` / `xdg-open` / `rundll32`; no new dependency), and must
reap the child (`Wait` or `Process.Release()`). Skip it when `--listen` is
non-loopback.

### 5.4 Security posture

Loopback binding is **not** a mitigation for the two attacks that apply:

- **CSRF.** Any origin can `<form method=POST action="http://127.0.0.1:8090/api/scan">`.
  This is a simple request, not preflighted; CORS blocks reading the response,
  not sending the request. Each scan re-probes every configured endpoint, so
  this is a remote-triggered probe amplifier pointed at production.
- **DNS rebinding.** An attacker's page rebinds to `127.0.0.1`, becomes
  same-origin, opens the `EventSource`, and reads the entire snapshot: internal
  hostnames, k8s namespaces and resource names, Vault addresses, TLS SANs,
  satellite topology. The browser is inside the loopback boundary.

Required controls:

1. **Host header validation on every route.** Reject unless the header is
   exactly `127.0.0.1:<port>`, `[::1]:<port>`, or `localhost:<port>` (or the
   configured listen address). This defeats DNS rebinding outright.
2. **`Sec-Fetch-Site: same-origin` (or matching `Origin`) required on
   `POST /api/scan`.** Defeats form-POST CSRF.
3. **Minimum interval between auto-triggered scans.** The user's explicit press
   bypasses the floor; the ticker and first-subscriber triggers do not.
4. **Non-loopback `--listen` is an explicit opt-in**, not a log line the user
   never sees. Refuse to bind unless a deliberate flag is passed.
5. **Strict CSP header:** `default-src 'none'; script-src 'self'; style-src 'self'; connect-src 'self'`,
   plus `X-Content-Type-Options: nosniff`.
6. **No `innerHTML` anywhere.** `messages` carry remote error text
   (`err.Error()` from failed dials, HTTP bodies, Kubernetes API errors), making
   them a stored-XSS vector. `textContent` and `createElement` only.
7. **Add `javascript` to the CodeQL matrix** (`.github/workflows/codeql.yaml` is
   currently `["go"]` only), so the repo's first browser code is analysed.

---

## 6. Scan lifecycle

### 6.1 Ownership invariant

One scanner goroutine owns the gRPC connection, the refresh timer, and the
subscriber registry, serialising them in a single `select` over: a cap-1 trigger
channel, the timer, register/unregister channels, and `rootCtx.Done()`.

This makes "subscriber count reached zero" a serialised event rather than a race
between HTTP handlers, and eliminates lost wakeups on the 0↔1 transition.

### 6.2 `Trigger()` takes no `context.Context`

```go
func (s *Scanner) Trigger(reason string) TriggerState // non-blocking, no ctx
```

This is an **invariant, not a convention**. If `Trigger` accepted a context, the
natural implementation in an HTTP handler passes `r.Context()`, and then the
first-subscriber scan is cancelled every time an `EventSource` reconnects (every
3s by default). The timeout context is derived internally from `rootCtx`, which
descends from `cmd.Context()`. State this in `scanner.go`'s package doc.

### 6.3 Trigger semantics: at most one in flight, at most one queued

A cap-1 channel. A press during a running scan **queues** a fresh scan; a second
press coalesces into the queued one.

Rejected: join-in-flight. Pressing "Scan now" 29 seconds into a 30-second scan
would silently discard an explicit user request and return near-stale data, or
an error, if the joined scan was about to hit its deadline.

Rejected: `golang.org/x/sync/singleflight` (already a direct dependency, so cost
was not the issue). It is keyed for a single key, runs `fn` on the *caller's*
goroutine (contradicting §6.1 and re-opening §6.2), re-panics into every
waiter, and has no coalesce-then-run-again semantics.

`POST /api/scan` returns `202` with `{"state":"started"|"queued"|"coalesced"}`,
and the `scanning` event carries `{scanID, seq, startedAt, queuedFollowUp}`, so
the button can render "Scanning… (refresh queued)" rather than lying.

### 6.4 Timer, not ticker

Use a `time.Timer` reset to `--refresh` **after each scan completes**. A fixed
`Ticker` whose period is shorter than a slow scan leaves the next tick already
pending on completion, producing back-to-back scans with no gap: exactly the
hammering the single-scan invariant exists to prevent. `Ticker.Stop()` also does
not drain an already-delivered tick, which would fire one spurious scan after
the last subscriber leaves.

Because the timer resets on **completion**, overlapping scans are structurally
impossible and the gap between scans is always at least `--refresh`. Startup
validation is therefore a usability guard, not a safety one: **refuse**
`refresh <= timeout` (guaranteed continuous load), and **warn** below
`2 * timeout` (a scan that routinely takes most of the interval leaves almost no
idle window). Validation applies only when `--refresh` is non-zero.

### 6.5 First-subscriber trigger needs a floor

`EventSource` reconnects every 3s by default. With the server down, every scan
fails, so no snapshot exists, so every reconnect re-satisfies "no snapshot" and
fires another scan. The condition is therefore *no snapshot **and** no attempt
within the floor interval* (`max(10s, --refresh)`), so store `lastAttemptAt`,
not just `lastSuccessAt`. The stream
also opens with `retry: 5000` to lengthen the browser's reconnect interval.

### 6.6 In-flight scan when the last subscriber leaves: finish it

Do not cancel. The result is a valid observation that gives the next subscriber
an instant paint; cancelling the client context cancels the server-side handler
context, unwinding `provider.Check` mid-probe and leaving half-open TCP/TLS/k8s
connections against real production endpoints on every closed browser tab. Scans
are cancelled **only** on `rootCtx` cancellation (shutdown).

### 6.7 Snapshot store

Holds `{bytes, hash, scanID, seq, observedAt, lastError, scanState, transitions}`.

**Snapshots are immutable bytes.** Marshal once on the scanner goroutine and
hand out only `[]byte`. Two reasons: `internal/output.FormatAndPrint` *mutates*
its argument (`status.Components = nil`, `= filterUnhealthy(...)`,
`= status.Flatten(...)` at `internal/output/render.go:12-26`) and `Flatten`
aliases the `Messages`/`Details` slices into its output; and
`*HealthCheckResponse` carries a `protoimpl.MessageState` that is not safe for
concurrent mutation. Marshalling once also removes per-subscriber cost.

Timestamps are ours. Responses carry no wall-clock instant, only `duration`.
`serverId` is populated **only** on loop detection
(`pkg/server/server.go:211-213`), so the UI displays the dialled address from
its own flags, never a value from the response.

Scan identity carries both a monotonic `seq uint64` (so the UI can detect gaps
and render "scan #47") and a UUID. Use `uuid.NewV7()` (time-ordered) from the
`github.com/google/uuid` dependency already present.

### 6.8 Canonicalisation: mandatory before hashing *and* before sending

Component ordering in responses is nondeterministic from three independent
sources:

1. `pkg/config/config.go:90-94`: `GetInstances()` ranges a Go map; top-level
   order is randomised per call.
2. `pkg/provider/provider.go:152-163`: results are collected off a channel in
   **goroutine completion order**, so sibling order is a function of network
   latency.
3. `pkg/server/server.go:75-91`: `filterInstances` ranges a map.

Without canonicalisation, the hash differs on essentially every poll, change
detection becomes a no-op that pushes on every refresh, **and** rows visibly
reshuffle on every push.

Therefore: recursively **sort children** before hashing and before marshalling.
`messages` order is meaningful (CEL failures are appended in expression order)
and must be preserved as-is.

**The comparator must be total.** Sorting on `(name, type)` alone is not enough.
Two siblings sharing both compare equal, and `slices.SortFunc` is pdqsort: it is
unstable, and its output for tied elements depends on the input permutation,
which is precisely the nondeterministic order being neutralised. The result is a
hash that flaps between polls, silently.

An earlier draft of this section claimed sibling names are unique because they
come from YAML map keys. That is false at the top level: `pkg/config/config.go:84-95`
holds `map[providerType][]Instance`, so the provider *type* is the key and
instances are a list. No duplicate-name validation exists anywhere in `pkg/config`,
so two `tcp` components both named `db` is legal, silently-accepted config.

`SortStableFunc` is not the fix, because stability preserves the random input
order. Tie-break on the child's own subtree digest after `(name, type)`.

### 6.9 Change detection

Hash the canonicalised, decoded tree over:
`{path, type, status, messages, failFastTriggered, serverId, detailsDigest}`.

Length-prefix every field rather than delimiting with NUL. Concatenating
variable-length fields with a separator that can occur inside them is
forgeable, and an unnamed node contributes no path segment, so depth is not
otherwise encoded. Both produce confirmed collisions between semantically
different trees.

- **`duration` is excluded**: it is set at both root
  (`pkg/server/server.go:203-205`) and per-component
  (`pkg/provider/provider.go:215-219`), jitters every scan, and would make
  detection 100% false-positive.
- **`details` are included**, but **never as raw wire bytes**. A cert rotation,
  or a Deployment's kstatus flipping `InProgress` to `Current` while status stays
  HEALTHY, is a real change that would otherwise be invisible.

  Hashing the raw `Any.Value` reintroduces the duration bug through another door.
  `Detail_DNS` carries a per-record `ttl` taken straight from the resolver
  response header (`pkg/provider/dns/dns.go:213`, serialised at `dns.go:279`).
  Behind a caching resolver that counts down between polls, so any `dns`
  component with `detail: true` changes its hash on every scan.

  Instead: resolve each `Any` through the global registry, zero the known-volatile
  fields, and hash a `proto.MarshalOptions{Deterministic: true}` re-marshal. The
  volatile set today is exactly `Detail_DNS`'s per-record `ttl`. Keep it as one
  named list so the next volatile field has an obvious home.

  Deterministic re-marshalling also closes a latent trap: raw-byte hashing of a
  proto containing a `map<>` field is nondeterministic, so the first detail type
  to gain one would break change detection with no compile error and no test
  failure. No detail proto has a map field today.

  `Detail_TLS.ValidUntil` is an absolute timestamp (the human-readable "3 days" is
  computed at render time), so it is stable and needs no special handling.
  Remaining known false-positive: DNS RR-set ordering, which caching resolvers
  round-robin.

Note: an earlier rationale (that protojson's randomised whitespace makes byte
comparison unstable) is **wrong**. `internal/detrand` seeds from a hash of the
binary and is stable for the lifetime of a process. The real reasons to hash the
decoded tree are the three above: nondeterministic ordering, duration exclusion,
and diffability. Do not restate the whitespace rationale in code comments.

Two known spurious-change sources to watch:
`pkg/provider/kubernetes/kubernetes.go:269` formats `invalid components: %v`
over a map-ordered slice; and client-go errors can embed resourceVersions.

The scanner also computes a `transitions` array of `{path, from, to}` entries
against the previous tree, which is cheap since it already holds it, and which
drives a UI transition log.

**Transitions must include the root.** Walking only named nodes drops it, because
the root is the one node the server leaves unnamed (`pkg/server/server.go:203-207`
sets neither `type` nor `name`). That discards "the estate as a whole just went
unhealthy", which is the most important transition a dashboard can report. Emit
the root under a reserved path key that cannot collide with an escaped component
name.

### 6.10 SSE protocol

Five named event types on one stream:

| Event | Payload | When |
|---|---|---|
| `snapshot` | full protojson + `scanID`, `seq`, `observedAt`, `transitions[]` | on change, and on subscribe |
| `scan` | `{scanID, seq, observedAt, durationMs, changed}` | **every** completed scan |
| `scanning` | `{scanID, seq, startedAt, queuedFollowUp}` | scan started |
| `scan-error` | gRPC status message | RPC failed |
| `connection` | channel state | state transitions |

The `scan` liveness event is what makes suppressing unchanged snapshots safe: it
drives "last checked 3s ago" and proves the pipeline is alive, so a quiet estate
is not indistinguishable from a dead one. It is ~120 bytes.

**Flapping between two polls is inherent to polling and no design fixes it**:
the server keeps no history. The UI must say "sampled every 30s" rather than
imply continuity.

**Marshalling:** `protojson.MarshalOptions{Multiline: false, EmitDefaultValues: true}`.
Never reuse `internal/output`, whose formatter sets `Multiline: true`
(`internal/output/json.go:18-23`). SSE framing is line-oriented, and a
pretty-printed body produces one `data:` line followed by bare lines that the
browser's parser silently discards. Write payloads through a helper that splits
on `\n` and prefixes every line, as defence in depth. `EmitDefaultValues`
ensures `UNKNOWN` nodes carry a `status` key rather than omitting it.

**`ui.go` must blank-import `pkg/platform_health/details`**, mirroring
`pkg/commands/client/client.go:20`. protojson resolves `Any` through the global
registry and returns an error if a type is unregistered, and that error kills
marshalling of the **entire response**, not just the detail. This is invisible
until someone enables `detail: true`.

**Headers:**

```
Content-Type: text/event-stream
Cache-Control: no-cache, no-transform
Connection: keep-alive
X-Accel-Buffering: no
X-Content-Type-Options: nosniff
```

`no-transform` prevents a compressing intermediary from buffering the stream. Go
does not compress responses and sets no write deadline by default, so nothing in
stdlib breaks SSE, but a gzip middleware would kill it silently. Note this in
`http.go`.

Heartbeat comment (`:heartbeat\n\n`) every 15-25s. `Last-Event-ID` handling is
**not** needed: snapshots are full state and idempotent, so a reconnecting
client that receives current state is caught up by definition.

### 6.11 Hub and backpressure

The hub **never writes bytes**. Each subscriber has a mutex-guarded `pending`
snapshot (**replaced**, not queued: snapshots are full state, so dropping an
intermediate is correct), a bounded drop-oldest ring for transient events, and a
cap-1 doorbell channel. Broadcast sets state under `sub.mu`, then does a
non-blocking send on the doorbell.

The **handler goroutine is the sole writer** for its subscriber.
`http.ResponseWriter` is not safe for concurrent use, so a hub that wrote
directly would race with heartbeats.

**`SetWriteDeadline` before every write**, via
`http.NewResponseController(w)`. Without it, a backgrounded tab stops reading,
the kernel send buffer fills, and the handler parks in `write(2)` indefinitely.
`r.Context()` is *not* cancelled, because the connection is still open. That
leaked goroutine also blocks shutdown. This is the single most important line in
`http.go`. Check `Flush()`'s returned error; that is how a dead subscriber is
detected.

Lock order is `hub.mu` → `sub.mu`, never the reverse. Only the hub closes a
subscriber; never `close(sub.doorbell)` from the handler side.

**On subscribe, replay all current state**: snapshot **plus** current error
**plus** scan-in-progress, as a single hello sequence. Never assume a
subscriber witnessed a transient event: a tab connecting after a failed first
scan would otherwise see an unexplained blank page, which is exactly when a
dashboard most needs to speak.

### 6.12 Shutdown sequence

`http.Server.Shutdown()` blocks on active handlers, and an SSE handler never
returns on its own, so ordering is load-bearing:

1. SIGINT → cancel `rootCtx`.
2. Scanner stops the timer and **finishes any in-flight scan**; publishes nothing further.
3. Hub releases every SSE handler (they write `event: shutdown` and return).
4. `srv.Shutdown(ctx)` with a 5s bound, so a wedged handler cannot hang the process.
5. Wait for the scanner to exit, then `conn.Close()`, then cancel the state watcher.

Two traps: closing the gRPC connection before the scanner exits makes the
in-flight `Check` fail with a confusing `Unavailable`; and calling `Shutdown`
before releasing the handlers is the deadlock. Do not "fix" it with
`srv.Close()`, which also drops the POST handler mid-flight.

### 6.13 gRPC client options

- `grpc.WithConnectParams` with `MaxDelay: 15s`. The default backoff ceiling is
  **120s** (`grpc/backoff`), so an unattended dashboard would stay dark for two
  minutes after the server recovered. `ResetConnectBackoff()` is **experimental**
  and its own doc says it "should not be used", so it must not be load-bearing;
  keep it, if at all, behind a one-function shim.
- `grpc.WithKeepaliveParams(keepalive.ClientParameters{Time: 5m, Timeout: 20s})`.
  The client sends no pings by default, so an idle socket across a NAT or VPN is
  silently reaped and the next scan burns a full timeout before reconnecting.
  **Do not go below 5 minutes**: the server's default `EnforcementPolicy.MinTime`
  is 5 minutes and will send `GOAWAY ENHANCE_YOUR_CALM`. Do not set
  `PermitWithoutStream`.
- `grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(32<<20))`. The default is
  4 MB; a large estate with `detail: true` can exceed it, and the failure mode is
  `ResourceExhausted` on every scan forever.
- One connection held for the process. `grpc.NewClient` performs no I/O and
  connects lazily. Do **not** copy `satellite.go:98-102`, which dials and closes
  per check.

Verified API stability in grpc-go v1.82.1: `GetState` and `WaitForStateChange`
are **not** marked experimental and are safe to depend on; `ResetConnectBackoff`,
`Connect`, `ConnectParams` (the type) and `WithIdleTimeout` **are**.

**Connection state mapping:** `READY` → ok; `IDLE` → **benign**, not red (with
manual refresh the channel idles after 30 minutes and would otherwise show a
false alarm on a healthy setup); `CONNECTING` → transient; `TRANSIENT_FAILURE`
→ red; `SHUTDOWN` → terminal. The watcher must exit on `SHUTDOWN` and on context
cancellation, or it leaks: after `Close()` the channel never changes state again.

No server-side limits are hit by one connection polling every N seconds: the
server is a bare `grpc.NewServer()` whose transport defaults for
`MaxConnectionIdle`/`MaxConnectionAge` are infinite.

---

## 7. Client (browser)

### 7.1 Model and path keying

Each `snapshot` is parsed once, then walked to build a `Map` keyed by path.

We do **our own walk**, not `Flatten`: that helper drops `Components`, aliases
the `messages`/`details` slices, and deliberately elides satellite nodes
(`pkg/platform_health/platform_health.go:56`). For a topology view the satellite
node is precisely what should be visible: it is the boundary between two servers.

Names come from YAML keys and may legally contain `/`, so a root component named
`a/b` would collide with `b` under system `a`. Each segment is
`encodeURIComponent`-ed before joining: reversible and collision-free. The root
is anonymous (no `type`, no `name`) and contributes no segment.

### 7.2 View state and reconciliation

```js
view = { expanded: Set<pathKey>, selected: pathKey|null, filter: "", zoom: {x,y,k} }
```

View state is **separate from server data and never derived from it.**

Reconciliation is keyed: for each incoming node, look up the existing row by
path and update only what changed. Rows are created only for new paths and
removed only for vanished ones. Because a row is never destroyed and rebuilt,
expansion, selection and scroll position survive **by construction** rather than
by restoration. This is htmx's good idea, implemented where it costs nothing.

Nodes appearing and disappearing mid-session is **normal**: the server
hot-reloads config under an RWMutex, and `kubernetes` selector mode churns with
the cluster. A vanished path drops its view state; a vanished *selected* path
clears the selection and the dock.

The `transitions[]` array drives a transition log ("fluxcd/source-controller ·
HEALTHY → UNHEALTHY · 14:32"), rather than a silent repaint. The reconciler's own
keyed diff remains authoritative.

### 7.3 Collapse policy

Any node with more than **25 children** renders collapsed in both views, badged
with child count and unhealthy count. Real configs in the repo are ~12 nodes, but
a `kubernetes` component in selector mode (`namespace: "*"`, no `name`) emits one
child per matched resource, realistically hundreds to thousands. The provider's
own `summarize` flag exists because this became unmanageable.

Two rules keep collapse from hiding problems: the Unhealthy filter searches the
**full model**, not visible rows; and on the **first** snapshot only, the tree
auto-expands paths leading to unhealthy nodes, never on subsequent snapshots,
since an auto-expand that fights a manual collapse every 30 seconds is worse than
none.

### 7.3a Rail collapse

The left rail collapses as a whole panel, independently of the expand and collapse
state of the tree nodes inside it. The toggle lives in the header and stays visible
while the rail is hidden, because a collapsed rail with no visible affordance reads
as a broken layout rather than a hidden one.

The graph canvas reflows to the reclaimed width. Collapsed state persists in
`localStorage` and is restored in the synchronous head script alongside the theme,
for the same reason: a rail that snaps shut after first paint is the same class of
flash the theme script exists to prevent. Both storage accesses are wrapped in
try/catch, since private browsing and blocked site data make the accessor throw
rather than return null.

### 7.4 Graph rendering

The data is a **strict tree**, not a general graph. The only directed edges are
containment: `system`→children, `satellite`→remote subtree,
`kubernetes`→one child per matched resource. There are **no dependency edges**
in the data model; `order` is a sibling-scoped execution wave, never recorded in
the response. The graph view therefore shows topology, not dependencies.

Layout is Reingold-Tilford tidy-tree computed in JS, because it depends on which
nodes are expanded: client state the server does not know. Only visible nodes
are laid out; a collapsed parent is one badged node. Pan and zoom are a single
SVG group transform, keeping hit-testing trivial. No layout animation in v1.

### 7.5 Wire-format requirements

- Named SSE events mean `onmessage` **never fires**. Each type needs an explicit
  `addEventListener`. This fails silently and completely if forgotten.
- `duration` is the canonical proto3 string (`"0.076347145s"`), not a number and
  not Go's `"76ms"`. Strip the trailing `s`, `parseFloat`, format for humans.
- With `EmitDefaultValues` server-side, every node carries `status`; no branch on
  `undefined` is required.
- **`failFastTriggered` must render prominently.** It means the tree is
  *incomplete*, and it can arrive from a satellite configured with
  `fail_fast: true` even though the dashboard never requests it. Showing a
  partial estate as complete is the worst failure this UI can have.

### 7.6 Connection lifecycle

Close the `EventSource` on `visibilitychange → hidden` after a short grace
period; reopen on visible; close on `pagehide`. This frees one of the browser's
six per-origin connections **and** removes the backgrounded-tab case where a
stalled reader parks the server's write.

The six-connection limit is per-origin **across all tabs**, and there is no
escape hatch: browsers negotiate HTTP/2 only over TLS via ALPN, and no browser
supports h2c. Six dashboard tabs consume every slot.

On `event: shutdown`, call `es.close()` explicitly and render "server stopped";
otherwise `EventSource` reconnects against a dead port every 5s forever.

### 7.7 Theme

One button cycling `auto → light → dark → auto`, showing the icon for its
current state with a tooltip naming the next.

Colours are custom properties on `:root`, redefined under **both**
`@media (prefers-color-scheme: dark)` and `[data-theme="dark"]`, so a manual pin
wins in either direction. A small inline script sets the attribute before first
paint to avoid a white flash on a dark-mode load. In auto mode a `matchMedia`
listener re-follows the OS if it changes.

### 7.8 Empty and degraded states

Each needs a designed appearance, not a blank canvas: no snapshot yet (first
scan in flight); first scan failed; gRPC disconnected; connected with zero
components configured; results incomplete due to fail-fast.

---

## 8. Error handling

| Failure | Behaviour |
|---|---|
| Server unreachable at startup | Explain why (dialled address and gRPC error), never an empty tree |
| Server dies mid-session | `connection` banner; scans fail fast; last snapshot retained, visibly stale with age |
| Scan deadline exceeded | `scan-error` with status; previous snapshot retained (a timeout is not evidence of unhealth) |
| `failFastTriggered` | Prominent incompleteness warning |
| Zero components | Distinct empty state, not confused with "nothing loaded" |
| `ResourceExhausted` | Error names the cause (receive limit), not a generic failure |
| Marshal failure | Emit `scan-error`; never silence |
| Subscriber write fails/deadlines | Drop that subscriber only |
| `--listen` port in use | Fail at startup with the address, before dialling |
| Panic during a scan | `recover` in the scanner loop; one bad response must not kill the loop for every viewer |

---

## 9. Testing

Calibrated to the repo's measured conventions, not to an external standard.

### 9.1 What this repo actually does

211 test functions, table-driven with testify throughout. Every provider,
`pkg/provider` core, `pkg/config`, `pkg/server`, `pkg/checks`, `internal/output`,
`pkg/netutil` and `details` are tested. **Only two cobra commands have tests at
all**: `pkg/commands/{check,client,context,root,server,validate}` have zero,
including `client`, the closest analogue to `ph ui`. `internal/cliflags`,
`pkg/phctx`, `pkg/client` and `pkg/platform_health` are untested. `httptest`
appears in two files. There are no concurrency stress tests, and CI runs without
`-race`.

The governing precedent is `pkg/commands/migrate`: a command package with a test
covering its **pure transform function**, not its cobra wiring.

### 9.2 In scope

- **`snapshot.go` canonicalise-and-hash**: same tree in different child order or
  with different durations hashes identically; changed status,
  `failFastTriggered` or detail payload does not. This protects the feature from
  being silently useless.
- **Path keying**: names containing `/`, duplicate names at different depths,
  the anonymous root.
- **`DialConfig.UseTLS`**: a four-line table. `pkg/netutil` is tested and this is
  its sibling; it is also the first coverage this logic has had at any of its four
  sites.
- **One `httptest` test of the SSE handler**: headers, `retry:` line, and correct
  `data: ` prefixing of a multi-line payload. `httptest` has precedent, and the
  prefixing bug fails silently in a browser with no error anywhere.

### 9.3 Out of scope, deliberately

- Scanner lifecycle, trigger coalescing, timer-reset and shutdown-ordering tests:
  no command in this repo tests its runtime wiring.
- Hub concurrency tests and a `-race` satellite test: there are no concurrency
  tests here, and the satellite race disappears *structurally* (the mutation is
  deleted, not guarded), so there is nothing to regress against.
- Adding `-race` to CI: a repo-wide policy change, not part of this feature.
- **All JavaScript testing.** No JS toolchain, lint or CI dimension exists.
  Adding one introduces a maintenance category. The reconciler, path keying,
  duration parsing and layout are still written as **pure functions**, so they
  are testable the day the project wants a harness, but no harness ships in v1.

### 9.4 Development affordance

`--fixture <file>` serves a canned snapshot with no gRPC connection at all: no
dial, no scanner loop, no timer. `POST /api/scan` re-reads the file from disk and
re-broadcasts, so editing the fixture and pressing Scan now is the iteration
loop. It makes the degraded states in §7.8 reviewable without a live estate.
`--fixture` is mutually exclusive with the target flags and refuses to start if
both are given. This is tooling, not a test.

---

## 10. Impact on the existing codebase

Honest accounting. The *command* is a clean plugin; the dial refactor is a
deliberate, separately-reviewable change.

### 10.1 Modified

| File | Change |
|---|---|
| `pkg/commands/root/root.go` | import + `AddCommand` (2 lines) |
| `pkg/client/client.go` | rewritten as the real dial helper; retires dead code |
| `pkg/commands/client/client.go:66-85` | dial block replaced |
| `pkg/provider/satellite/satellite.go:80-98` | dial block replaced; race removed |
| `pkg/provider/grpc/grpc.go:74-92` | dial block replaced; port rule reconciled |
| `internal/cliflags/flags.go` | new `ClientFlags()` helper |
| `pkg/commands/client/client_flags.go` | use `ClientFlags()` |
| `.github/workflows/codeql.yaml` | add `javascript` to the matrix |
| `README.md` | Dashboard subsection under Usage |
| `CLAUDE.md` | add `ui` to the subcommand list; note the first `go:embed`/HTTP server; note `ui` is unreachable from `phc`/`phs` |
| `.gitignore` | add `.superpowers/` |

### 10.2 New

`pkg/commands/ui/**` (§5.1), and `docs/superpowers/specs/` (this document).

### 10.3 Verified as needing no change

`.goreleaser.yml` (`go:embed` needs no build step; assets are checked in; the
existing `unified` build already covers `./cmd/ph`), `.ko.yaml`,
`.github/workflows/test.yaml`, `go.mod`/`go.sum` (no new dependencies:
`--open` uses `exec.Command`).

### 10.4 Deployment

**v1 is local-use only**, stated explicitly in the command's `Long` text and the
README. The chart cannot express the alternative without real work: one
container, one unnamed service port, gRPC liveness/readiness probes. In-cluster
deployment would need a second container, a second `containerPort`, **named**
service ports (adding a second port to an unnamed-port Service is invalid), an
HTTP probe, ingress backend selection, `values.yaml` plumbing, and kubeconform
re-validation. If it becomes a goal it needs its own design, and the token auth
deferred in §5.4 becomes mandatory.

Note the released container image is built from `./cmd/ph` (goreleaser `kos:`),
so `ph ui` *is* present in the published image even though the chart cannot yet
run it.

---

## 11. Deferred

- Server-side interval scanning and result retention (the original "once proven"
  item).
- In-cluster deployment and chart support, with token authentication.
- Server-side component scoping, contingent on first fixing
  `filteredInstance`/`filteredChild` to forward `GetOrder`/`GetAlways`.
- A JS test harness.
- Layout animation in the graph view.

---

## Appendix: sources

Design decisions above cite `file:line` in this repo. External findings,
including htmx/idiomorph/SSE-extension behaviour and versions, grpc-go API
stability and defaults, and protojson encoding behaviour, were verified against
shipped source and vendor documentation during four subagent investigations on
2026-08-26.
