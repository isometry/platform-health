# The `ph ui` dashboard

`ph ui` serves a live dashboard for a running platform-health server. It is a gRPC client rather than a server extension: it polls the existing unary `Check` RPC, canonicalises and hashes each response, and pushes changed snapshots to connected browsers over server-sent events. The server is untouched, and `ph ui` reads no config file and registers no providers.

The browser gets two coordinated views of one scan: a left-to-right topology graph, and a hierarchical rail listing every component with its status, type and timing. Selection is shared between the rail, the graph and a detail dock along the bottom. The dashboard is reachable from the `ph` binary only, because `cmd/phc` and `cmd/phs` splice `client` and `server` into `os.Args` before the command tree is built.

## Building the dashboard

The dashboard is behind the `ui` build tag. Every file in `pkg/ui` and `pkg/commands/ui` carries `//go:build ui`, and [`root_ui.go`](../../pkg/commands/root/root_ui.go) / [`root_noui.go`](../../pkg/commands/root/root_noui.go) provide the two halves of `registerUI`, so a default build has no dashboard and no `ui` subcommand.

```bash
go build ./cmd/ph            # no dashboard: `ph ui` does not exist
go build -tags ui ./cmd/ph   # `ph ui` is registered
go test -tags ui ./...       # runs pkg/ui's tests as well
```

The tag keeps the embedded assets, the HTTP server and the SSE machinery out of a build that only needs to probe components. Releases go the other way: `.goreleaser.yml` gives the `unified` build and the `kos` image a shared `&dashboardFlags` anchor carrying `-tags=ui`, while the `phc` and `phs` builds override `flags` with `-trimpath` alone. CI compiles and tests both ways.

The Helm chart runs the dashboard as a sidecar when `ui.enabled` is set, using the same image with `ui --listen=0.0.0.0:8090 --allow-remote --server=127.0.0.1 --port=8080`. Enabling it also names the Service's ports, since a Service with more than one port cannot leave them unnamed.

## Package layout

`pkg/ui` is the dashboard; `pkg/commands/ui` is only its cobra wiring.

| File | Responsibility |
|---|---|
| [`pkg/ui/scanner.go`](../../pkg/ui/scanner.go) | Connection ownership, trigger loop, subscriber registry, snapshot store |
| [`pkg/ui/snapshot.go`](../../pkg/ui/snapshot.go) | Path keying, canonicalisation, hashing, transitions, marshal sanitising |
| [`pkg/ui/sse.go`](../../pkg/ui/sse.go) | Event framing, the streaming handler, heartbeats and write deadlines |
| [`pkg/ui/subscriber.go`](../../pkg/ui/subscriber.go) | Per-subscriber pending state and doorbell |
| [`pkg/ui/events.go`](../../pkg/ui/events.go) | Event payload types and connection-state mapping |
| [`pkg/ui/http.go`](../../pkg/ui/http.go) | Routes, the Host and origin guard, embedded assets |
| `pkg/ui/assets/index.html` | Shell markup |
| `pkg/ui/assets/theme.js` | Pre-paint restore of theme and rail width |
| `pkg/ui/assets/app.js` | Model, reconciler, rail, graph, dock, banners |
| `pkg/ui/assets/app.css` | Palette and layout for both themes |

Assets are embedded with `go:embed`, rooted at `assets` through `fs.Sub`, and served by `http.FileServerFS`. This is the repository's first `go:embed`, first HTTP server and first browser code, which is why `javascript` was added to the CodeQL matrix. Embedded files report a zero modtime, so `FileServerFS` emits neither `ETag` nor `Last-Modified`; the asset handler sets `Cache-Control: no-cache` so a browser cannot serve a previous build's JavaScript after an upgrade.

## Command wiring

[`pkg/commands/ui/ui.go`](../../pkg/commands/ui/ui.go) follows the repository idiom: a package-private `uiFlags` built by `cliflags.Merge`, registered on the command's flag set, `PreRunE: setup` binding flags into the viper instance from the command context, and `RunE: serve`. There is no `config.Load()`, since the dashboard is a pure client.

`root.New()` calls `registerUI(cmd)` unconditionally; with `-tags ui` that adds `ui.New()`, and without it the function is empty.

The target server is a positional `[host:port]` argument parsed by `netutil.ParseHostPort`, exactly as `ph client` and `ph server` accept one, or the `--server` and `--port` flags. Those four connection flags come from `cliflags.ClientFlags()`, shared with `ph client`.

| Flag | Default | Meaning |
|---|---|---|
| `--server` / `-s` | `localhost` | target server host |
| `--port` / `-p` | `8080` | target server port |
| `--tls` | off | force TLS; also implied by port 443 or 8443 |
| `--insecure` / `-k` | off | skip certificate verification |
| `--timeout` / `-t` | `30s` | per-scan deadline |
| `--listen` | `127.0.0.1:8090` | dashboard socket, as `host:port` |
| `--refresh` | `0` (off) | auto-refresh interval |
| `--open` | off | open a browser on start |
| `--allow-remote` | off | permit a non-loopback `--listen` |
| `--fixture` | none | serve a canned snapshot with no gRPC connection |

`Merge` is last-wins, which is how the `30s` timeout replaces `cliflags.TimeoutFlags()`'s `10s` default.

Two flags read differently from their counterparts elsewhere in `ph`, and both usage strings say so:

- `ph server --listen` takes a bare host, has `NoOptDefault: "localhost"`, and binds every interface by default. `ph ui --listen` takes a full `host:port` and defaults to loopback: the opposite polarity under the same flag name.
- `--refresh 0` disables auto-refresh outright, where `0` means "use the default" for `--parallelism` and for a provider's timeout.

`setup` validates before anything binds or dials. A non-loopback `--listen` without `--allow-remote` is refused; `--fixture` combined with any target flag or a positional argument is refused; and a non-zero `--refresh` at or below `--timeout` is refused, because the next scan would be due before the current one could fail. A refresh under twice the timeout warns on stderr rather than through the logger, whose default level is error.

## Security posture

The dashboard has no authentication. Loopback binding is the default, and `--allow-remote` is the deliberate opt-in for anything else, but loopback alone does not defend against the two attacks that reach it from a web page:

- **DNS rebinding.** An attacker's page rebinds to `127.0.0.1`, becomes same-origin, opens the `EventSource`, and reads the whole snapshot: internal hostnames, namespaces and resource names, Vault addresses, TLS SANs, satellite topology.
- **CSRF.** A cross-origin `<form method=POST action="http://127.0.0.1:8090/api/scan">` is a simple request, so it is never preflighted. CORS would block reading the response, not sending the request, and every scan re-probes the whole estate.

`guard` in [`pkg/ui/http.go`](../../pkg/ui/http.go) wraps every route, and `Mux` is the only constructor, so an unguarded handler cannot be assembled by accident. It rejects any `Host` outside the listen address and its loopback spellings, rejects a POST whose `Sec-Fetch-Site` is not `same-origin` (falling back to a matching `Origin` for browsers that omit it), and sets `Content-Security-Policy: default-src 'none'; script-src 'self'; style-src 'self'; connect-src 'self'` plus `X-Content-Type-Options: nosniff` on every response.

Component `messages` carry remote error text: failed dials, HTTP bodies, Kubernetes API errors. The browser code builds every node with `createElement` and `textContent` and never assigns `innerHTML`.

## Scan lifecycle

### One goroutine owns everything mutable

`Scanner.Run` serialises the gRPC connection, the refresh timer and the subscriber registry in a single `select` over the trigger channel, the register and unregister channels, connection-state updates, the timer and `rootCtx.Done()`. That makes "the subscriber count reached zero" a serialised event rather than a race between HTTP handlers.

### `Trigger` takes no context, permanently

If it accepted one, the obvious call in an HTTP handler passes `r.Context()`, and the first-subscriber scan is then cancelled every time a browser's `EventSource` reconnects. Scan contexts derive from `rootCtx` alone.

### One scan in flight, one queued

The trigger channel has capacity 1. A press during a running scan queues a fresh scan; a second press coalesces into the queued one. `POST /api/scan` returns `202` with `{"state":"started"|"queued"|"coalesced"}`, so the button can say "scanning, refresh queued" instead of pretending the press started something.

### A timer, not a ticker

A fixed ticker whose period is shorter than a slow scan leaves the next tick already pending when the scan ends, producing back-to-back scans with no gap. `Ticker.Stop` also does not drain a delivered tick, which would fire one spurious scan after the last subscriber left. Because the timer is armed from completion, the gap between scans is always at least `--refresh`.

### The first-subscriber floor

Auto-triggered scans have a floor of `max(10s, --refresh)`. A browser's `EventSource` reconnects every few seconds. With the server down, every scan fails, so "no snapshot yet" is re-satisfied on every reconnect and would fire a scan each time. The scanner tracks `lastAttempt`, not just the last success, and the stream opens with `retry: 5000` to lengthen the browser's own interval.

### The last subscriber leaving does not cancel a scan

Cancelling the client context cancels the server's handler context, unwinding `provider.Check` mid-probe and leaving half-open connections against production endpoints on every closed tab. The result is also a valid observation that gives the next subscriber an instant paint. Scans are cancelled only when `rootCtx` is.

### The snapshot store holds bytes

Each response is marshalled once, on the scanner goroutine, and only `[]byte` is handed out. `internal/output` mutates the response it formats and `Flatten` aliases the `messages` and `details` slices, so a live message must never escape; `protoimpl.MessageState` is not safe for concurrent mutation either. Replay frames for a new subscriber are kept pre-encoded for the same reason. Timestamps are the dashboard's own, since a response carries only durations, and the displayed target is the dialled address from the flags, never a value read back from a response. Each scan gets a monotonic `seq` and a time-ordered UUIDv7.

### Canonicalisation, before hashing and before sending

Component order in a response is nondeterministic from three independent sources: `GetInstances()` ranges a map, results are collected in goroutine completion order, and `filterInstances` ranges a map. Without sorting, the hash differs on nearly every poll, change detection degrades to pushing every time, and rows visibly reshuffle. `sortTree` sorts children by name, then type, then the child's own subtree digest. The digest tie-break is load-bearing: sibling names are not unique (top-level config is keyed by provider type, and no duplicate-name validation exists), and `slices.SortFunc` is an unstable pdqsort whose output for tied elements depends on the input permutation, which is the very thing being neutralised. Message order is meaningful, because CEL failures append in expression order, and is preserved.

### Change detection

`nodeDigest` covers name, type, status, `failFastTriggered`, `serverId`, messages and details, length-prefixing every field so concatenation cannot collide, then folds in each child's digest. `duration` is excluded: it is set at the root and per component, jitters every scan, and would make detection a permanent false positive. Details are included, so a certificate rotation or a kstatus flip under an unchanged status is still seen, but never as raw wire bytes: each `Any` is resolved through the registry, known-volatile fields are zeroed, and a `Deterministic: true` re-marshal is hashed. `sanitiseDetail` currently zeroes the per-record DNS TTL and sorts DNS records, since a caching resolver counts the TTL down and round-robins the RR set between polls. Deterministic re-marshalling also protects against the first detail type to gain a `map<>` field, which would otherwise break detection with no compile error and no test failure.

### Sanitising the wire copy

protojson resolves `Any` through the global registry and aborts the whole message on the first unresolvable type, so one child carrying a detail type this build does not know, from a satellite on a newer build, would blank the entire snapshot. The sanitised copy is separate from the canonical tree, which hashing and transitions keep using untouched.

### Transitions include the root

`Transitions` diffs per-path statuses between the previous and current canonical trees. The root is the one node the server leaves unnamed, and dropping it would discard "the estate as a whole just went unhealthy", so it is reported under the reserved key `/`. `PathKey` escapes only `%` and `/`, in that order, and the browser recomputes keys with the identical two replacements; widening the scheme on either side would silently desynchronise them for a name like `ssh@localhost`.

### Shutdown order

`http.Server.Shutdown` blocks on active handlers and an SSE handler never returns on its own, so `serve` cancels `rootCtx`, calls `Release` to let the handlers write their farewell and return, then `Shutdown` with a five-second bound, then waits for `Run` to exit before closing the connection. Closing the connection first makes an in-flight `Check` fail with a confusing `Unavailable`; calling `Shutdown` before `Release` is the deadlock.

### gRPC dial options

[`client.Dial`](../../pkg/client/client.go) returns a lazily-connecting `ClientConn` and owns no policy. The scanner passes a 15s backoff ceiling, because the default is 120s and an unattended dashboard would stay dark for two minutes after the server recovered; a 5-minute keepalive with a 20s timeout, because the client sends no pings by default and an idle socket across a NAT is silently reaped, and because 5 minutes is the server's `EnforcementPolicy` minimum; and a 32 MB receive limit, because the 4 MB default fails a large estate with `detail: true` as `ResourceExhausted` on every scan forever. One connection is held for the process. `watchConnection` feeds channel state into the loop's select rather than broadcasting directly, and exits on `SHUTDOWN`, after which the channel never changes state again.

## The SSE protocol

Six named event types on one stream. Named events mean `onmessage` never fires, so the browser registers an explicit listener for each.

| Event | Payload | When |
|---|---|---|
| `snapshot` | protojson tree, `scanID`, `seq`, `observedAt`, `transitions[]` | on change, and on subscribe |
| `scan` | `scanID`, `seq`, `reason`, `observedAt`, `durationMs`, `changed` | after every completed scan |
| `scanning` | `scanID`, `seq`, `reason`, `startedAt`, `queuedFollowUp` | scan started |
| `scan-error` | gRPC code and message | scan failed |
| `connection` | channel state, severity, target, refresh interval | on a state change |
| `shutdown` | `{}` | the handler is closing deliberately |

One scan on the wire, with the snapshot elided:

```
retry: 5000

event: connection
data: {"state":"READY","severity":"ok","target":"localhost:8080","refreshMs":0}

event: scanning
data: {"scanID":"019...","seq":1,"reason":"first-subscriber","startedAt":"...","queuedFollowUp":false}

event: scan
data: {"scanID":"019...","seq":1,"reason":"first-subscriber","observedAt":"...","durationMs":81,"changed":true}

event: snapshot
data: {"snapshot":{...},"scanID":"019...","seq":1,"observedAt":"...","transitions":[{"path":"/","from":"","to":"HEALTHY"}]}

:heartbeat

```

The `scan` event is what makes suppressing unchanged snapshots safe: it drives "last checked 3s ago" and proves the pipeline is alive. `shutdown` tells the browser to call `es.close()`, or `EventSource` reconnects against a dead port every five seconds forever.

Snapshots are marshalled with `Multiline: false` and `EmitDefaultValues: true`. SSE framing is line-oriented, so a pretty-printed body would yield one `data:` line followed by bare lines the browser's parser drops without an error; `WriteEvent` prefixes every line as defence in depth. `EmitDefaultValues` keeps a `status` key on `UNKNOWN` nodes. `pkg/ui` blank-imports `pkg/platform_health/details` so protojson can resolve detail types at all.

Responses carry `Content-Type: text/event-stream`, `Cache-Control: no-cache, no-transform`, `Connection: keep-alive`, `X-Accel-Buffering: no` and `nosniff`. `no-transform` stops a compressing intermediary buffering the stream; no compression middleware may ever be added to this mux for the same reason. A `:heartbeat` comment goes out every 20 seconds. `Last-Event-ID` is not handled, because snapshots are full state and a reconnecting client that receives current state is caught up by definition.

**The hub never writes bytes.** Each subscriber holds a mutex-guarded pending snapshot, a bounded drop-oldest ring of 64 transient events, and a capacity-1 doorbell. The pending snapshot is replaced rather than queued, since snapshots are full state and dropping an intermediate is correct. The handler goroutine is the sole writer for its subscriber, because `http.ResponseWriter` is not safe for concurrent use. Lock order is scanner then subscriber; only the hub closes a subscriber, and the handler reads the doorbell two-valued, since a closed channel is permanently ready and a bare receive would spin.

Every write is preceded by `SetWriteDeadline` through `http.NewResponseController`. Without it, a backgrounded tab stops reading, the kernel send buffer fills, and the handler parks in `write(2)` indefinitely while `r.Context()` stays live because the connection is still open. That goroutine also blocks shutdown. The handler writes and flushes its headers and the `retry:` line before subscribing, because `Subscribe` blocks until the loop reaches its select, which during a scan can be a full timeout.

On subscribe the scanner replays the connection frame, the last snapshot, the current error and any scan in progress. A tab that connects after a failed first scan must not see an unexplained blank page.

## The browser

### Model and path keying

`app.js` is one IIFE with its pure functions at the top and no DOM access among them. Each snapshot is walked into a `Map` keyed by path. The walk is the dashboard's own rather than `Flatten`, which drops `Components`, aliases slices and elides satellite nodes; for a topology view the satellite node is exactly the boundary worth seeing.

View state lives in a separate object from server data and is never derived from it:

```js
view = { expanded: Set<pathKey>, selected: pathKey|null, filter: "", zoom: {x,y,k} }
```

### Reconciliation

Reconciliation is keyed on that path. For each incoming node the existing row is looked up and only what changed is updated; rows are created for new paths and removed for vanished ones, and an element already in position is never re-inserted. Expansion, selection, focus and scroll position survive a push by construction rather than by restoration. The rail, the graph nodes, the edges, the dock and the banners all share the same keyed-list discipline. Nodes appearing and disappearing mid-session is normal, since the server hot-reloads config and Kubernetes selector mode churns with the cluster; a vanished path drops its view state, and a vanished selection clears the dock.

### Collapse policy

A container with more than 25 children defaults to collapsed, badged with its child count and unhealthy count. The default is applied only to newly seen paths, which is what lets a manual expand or collapse survive every later push. Two rules keep collapse from hiding problems: the unhealthy filter and the header pill are computed from the model, not from rendered rows, so an unhealthy descendant of a collapsed container is still counted and still reachable; and only the first snapshot auto-expands the ancestors of unhealthy nodes, because an auto-expand that fights a manual collapse every refresh is worse than none.

### The graph

The data is a strict tree. The only edges are containment: `system` to its children, `satellite` to a remote subtree, `kubernetes` to one child per matched resource. There are no dependency edges in the model, and `order` is a sibling-scoped execution wave that never appears in the response, so the graph shows topology and not dependencies. Layout is Reingold-Tilford in Buchheim's linear-time form, computed in the browser because it depends on which nodes are expanded, which the server does not know. Only visible nodes enter the layout, so a collapsed container is one badged node. Pan and zoom are a single transform on one group, which leaves hit-testing to the browser and keeps a push from disturbing the viewport.

### Persisted state and theme

The address bar carries what a link has to reproduce: selection, filters and viewport, synced with `replaceState` so panning does not bury the back button. Preferences the viewer owns, theme and rail width and follow mode, stay in `localStorage`, so a shared link never rewrites someone else's dashboard. `theme.js` runs synchronously in `<head>` and applies both the stored theme and the stored rail width before first paint, since either applied afterwards is a visible flash. Every storage access is wrapped, because private browsing and blocked site data make the accessor throw rather than return null.

### Wire-format traps

Two wire-format details bite silently. `duration` is the canonical proto3 string (`"0.076347145s"`), not a number and not Go's `"76ms"`. And `failFastTriggered` means the tree is incomplete: it can arrive from a satellite configured with `fail_fast: true` even though the dashboard never asks for it, and showing a partial estate as complete is the worst thing this UI can do, so it leads the banner list.

### Degraded states

`computeBanners` turns every degraded state into data, ordered worst first: fail-fast truncation, a stopped or dropped event stream, a gRPC channel in trouble, a failed manual trigger, a failed scan, no snapshot yet, and a successful scan of an estate with no components. Each has its own wording; none of them is a blank canvas. A failed scan keeps the previous tree and marks it stale, because a timeout is not evidence of unhealth.

## Visual decisions

Names are never truncated; scrolling, collapsing and zooming are the answer. Status colour outranks every other emphasis. On a graph edge the precedence is status, then route, then duration, and route owns stroke width. The graph is laid out left to right because the tree is shallow and wide.

Status is hue, route is opacity: an unhealthy edge keeps its hue at full strength, since dimming a failing edge below a passing one would invert the first rule. An edge on the route to the selection takes the accent colour and a wider stroke; off the route its opacity drops, and a failing edge drops less than a passing one so it never falls below the 3:1 contrast floor. The slowest sibling under each parent gets one discrete dot and a heavier label weight, never a stroke width and never a status hue. Edge strokes carry `vector-effect: non-scaling-stroke`, or a 1.25px line would render at 0.31px at the zoom floor. Below a scale where an 11px label would fall under 8.25px on screen, ordinary names are dropped, while selected and unhealthy names stay and are counter-scaled: an alert nobody can read is not an alert.

## Why plain JavaScript

The dashboard is client-owned view state layered on constantly-changing server state. Which nodes are expanded, what is selected, the graph's transform and the filter text all have to survive every push. Server-rendered fragments make expansion a network round-trip and render one scan N ways for N viewers, which costs the broadcast fan-out that lets a single scan serve every subscriber.

The alternative weighed was htmx with idiomorph. Idiomorph treats `open` and `class` as ordinary attributes and overwrites them from the server's HTML, so expansion and selection are stripped on every push; the documented escape is a reconciler policy written inside an `hx-swap` attribute, which forces `unsafe-eval` and would cost the CSP above. The one good idea it has, that the tree is never repainted wholesale, is what the keyed reconciliation here implements.

The graph layout is not a library either. Reingold-Tilford over a strict tree is about 150 lines in `app.js`, it needs no vendored bundle and so keeps the CSP trivial, and the visual design is exact rather than fought against a library's defaults.

## Behaviour choices

Scanning is manual by default: the first subscriber to connect triggers one, and so does the button. `--refresh` opts into polling, and the toggle in the header reflects it. A poll pushes a snapshot only when the hash changed, and the `scan` event covers liveness in between.

`-c/--component` is deliberately not offered. A typo produces a *successful* RPC carrying `UNHEALTHY`, one `invalid components: ...` message, no components and no duration, which renders as an empty red screen indistinguishable from a total outage. Scoped results are also not a subset of unscoped ones: `filterInstances` wraps matches in `filteredInstance`, which loses `GetOrder()` and `GetAlways()`, so order groups collapse into a single wave and `always` instances lose their fail-fast immunity. The dashboard already holds the full snapshot, so filtering in the browser is instant and cannot invent a phantom `UNHEALTHY`. Server-side scoping would first require `filteredInstance` and `filteredChild` to forward those two methods.

`--fixture` serves a protojson snapshot from disk with no dial, no timer and no scanner loop. `POST /api/scan` re-reads the file, so editing it and pressing Scan now is the iteration loop, and every degraded state is reachable without a live estate. It is mutually exclusive with the target flags and fails at startup if the file does not parse.

## Error handling

| Failure | Behaviour |
|---|---|
| Server unreachable at startup | Banner naming the dialled address and the gRPC error, never an empty tree |
| Server dies mid-session | `connection` banner; last snapshot retained and marked stale |
| Scan deadline exceeded | `scan-error`; previous snapshot retained, since a timeout is not evidence of unhealth |
| `failFastTriggered` | Leading banner naming the components that stopped the scan |
| Zero components | Distinct banner, not confused with "nothing loaded" |
| Marshal failure | Emitted as `scan-error`; never silenced into an empty envelope |
| Panic during a scan | Recovered in the scan loop and reported; one bad response must not kill polling for every viewer |
| Subscriber write fails or deadlines | That subscriber's handler returns; nobody else is affected |
| `--listen` port in use | Startup error naming the address |

## Testing

`pkg/ui` tests the pure functions that fail silently in production: canonicalisation and hashing (child order and duration must not change the hash, status and details must), path keying and root-path collision, transitions, marshal sanitising, and SSE line prefixing. The scanner's lifecycle, the hub's concurrency and all browser code are untested here, matching the repository's practice of testing a command package's transforms rather than its runtime wiring.
