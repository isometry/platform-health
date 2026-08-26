# `ph ui` Dashboard Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add `ph ui`, a subcommand that serves a live web dashboard (graph view plus component tree) for a running platform-health server, driven by SSE.

**Architecture:** `ph ui` is a gRPC *client*, not a server extension. One goroutine owns a long-lived `grpc.ClientConn`, polls the existing unary `Check` RPC, canonicalises and hashes the result, and fans it out to browsers over one SSE stream. The browser holds the snapshot as a model and reconciles the DOM keyed by component path, so expand/select/zoom state survives every push. Assets are embedded with `go:embed`.

**Tech Stack:** Go 1.26 (stdlib `net/http`, `embed`, `http.ResponseController`), grpc-go v1.82.1, `protojson`, cobra + viper, and dependency-free vanilla JavaScript. **No new Go or JS dependencies are added by this plan.**

**Spec:** `docs/superpowers/specs/2026-08-26-ph-ui-dashboard-design.md`. Read it alongside this plan; every task argues from it.

## Global Constraints

- **No new dependencies.** `--open` uses `exec.Command`, not a library. No JS framework, no vendored graph library, no test framework.
- **Testing is calibrated to the repo, not to habit.** Per spec §9: pure logic gets table-driven tests with `stretchr/testify`; cobra/HTTP/scanner wiring does **not** (only 2 of 8 command packages in this repo have any test). Do not add `-race` to CI. Do not add a JS test harness. Tasks below say explicitly which have tests and which have manual verification.
- **No `innerHTML`, ever.** `messages` carry remote error text and are a stored-XSS vector. `textContent` and `createElement` only.
- **SSE payloads must be compact JSON.** `protojson.MarshalOptions{Multiline: false, EmitDefaultValues: true}`. Never reuse `internal/output`, whose formatter sets `Multiline: true` and whose `FormatAndPrint` *mutates* its argument.
- **`pkg/commands/ui/ui.go` must blank-import `pkg/platform_health/details`** or `protojson.Marshal` fails for the entire response whenever any provider attaches a detail.
- **Commit style:** conventional commits (`feat:`, `refactor:`, `docs:`). **No `Co-Authored-By` trailers and no tool attribution**: a standing rule for this user, in every repo.
- **Branch:** `feat/ph-ui`, already created.
- Component ordering from the server is nondeterministic (three independent sources, spec §6.8). Canonicalise before hashing *and* before marshalling, always.

---

# Phase 1: The dial refactor

Ships independently of the UI and is worth reviewing on its own. Land all four tasks before starting Phase 2.

---

### Task 1: `pkg/client` becomes the shared dial helper

`pkg/client` currently wraps `ph.HealthClient`, has **zero callers in the module**, and hardcodes `insecure.NewCredentials()`. Rewrite it as the real dial helper. This is the only Phase 1 task with a test, and that test is the first coverage this logic has had at any of its four sites.

**Files:**
- Modify: `pkg/client/client.go` (whole file)
- Test: `pkg/client/client_test.go` (create)

**Interfaces:**
- Consumes: nothing.
- Produces: `client.DialConfig{Host string; Port int; TLS, Insecure bool}`, `client.TLSPorts []int`, `(DialConfig) Address() string`, `(DialConfig) UseTLS() bool`, `client.Dial(cfg DialConfig, opts ...grpc.DialOption) (*grpc.ClientConn, error)`.

- [ ] **Step 1: Write the failing test**

Create `pkg/client/client_test.go`:

```go
package client_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/isometry/platform-health/pkg/client"
)

func TestDialConfigUseTLS(t *testing.T) {
	tests := []struct {
		name     string
		config   client.DialConfig
		expected bool
	}{
		{"plain port", client.DialConfig{Host: "localhost", Port: 8080}, false},
		{"explicit tls", client.DialConfig{Host: "localhost", Port: 8080, TLS: true}, true},
		{"implied by 443", client.DialConfig{Host: "example.com", Port: 443}, true},
		{"implied by 8443", client.DialConfig{Host: "example.com", Port: 8443}, true},
		{"insecure does not imply tls", client.DialConfig{Host: "localhost", Port: 8080, Insecure: true}, false},
		{"explicit tls on implied port", client.DialConfig{Host: "example.com", Port: 443, TLS: true}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.config.UseTLS())
		})
	}
}

func TestDialConfigAddress(t *testing.T) {
	tests := []struct {
		name     string
		config   client.DialConfig
		expected string
	}{
		{"hostname", client.DialConfig{Host: "example.com", Port: 8080}, "example.com:8080"},
		{"ipv4", client.DialConfig{Host: "127.0.0.1", Port: 443}, "127.0.0.1:443"},
		{"ipv6 is bracketed", client.DialConfig{Host: "::1", Port: 8080}, "[::1]:8080"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.config.Address())
		})
	}
}

func TestDialIsLazy(t *testing.T) {
	// grpc.NewClient performs no I/O, so dialling an address nothing listens on
	// must still succeed. This is the property ph ui relies on to hold one
	// connection for the process lifetime.
	conn, err := client.Dial(client.DialConfig{Host: "127.0.0.1", Port: 1})
	assert.NoError(t, err)
	assert.NotNil(t, conn)
	assert.NoError(t, conn.Close())
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/client/... -v`
Expected: FAIL: `undefined: client.DialConfig`.

- [ ] **Step 3: Write the implementation**

Replace the contents of `pkg/client/client.go`:

```go
// Package client provides shared gRPC dialling for platform-health clients.
package client

import (
	"crypto/tls"
	"net"
	"slices"
	"strconv"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
)

// TLSPorts are ports for which TLS is implied even when DialConfig.TLS is false.
var TLSPorts = []int{443, 8443}

// DialConfig describes how to reach a platform-health gRPC server.
type DialConfig struct {
	Host     string
	Port     int
	TLS      bool // force TLS; also implied by TLSPorts
	Insecure bool // skip certificate verification
}

// Address returns the host:port dial target.
func (c DialConfig) Address() string {
	return net.JoinHostPort(c.Host, strconv.Itoa(c.Port))
}

// UseTLS reports whether the connection should use TLS. It is a pure function
// of immutable fields: callers must not cache or write back its result.
func (c DialConfig) UseTLS() bool {
	return c.TLS || slices.Contains(TLSPorts, c.Port)
}

// Dial returns a lazily-connecting ClientConn for the target. It performs no
// I/O; the connection is established on first RPC. Callers own Close(), and
// own any backoff, keepalive or call-size policy they need via opts.
func Dial(cfg DialConfig, opts ...grpc.DialOption) (*grpc.ClientConn, error) {
	if cfg.UseTLS() {
		opts = append(opts, grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{
			ServerName:         cfg.Host,
			InsecureSkipVerify: cfg.Insecure,
		})))
	} else {
		opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}
	return grpc.NewClient(cfg.Address(), opts...)
}
```

Note the old `Client` struct and its `NewClient`/`Check` methods are **deleted**, not kept. They have no callers, and the goroutine-closes-on-ctx pattern in the old `NewClient` is a leak trap.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/client/... -v`
Expected: PASS: all three test functions.

Then confirm nothing else referenced the deleted type:
Run: `go build ./...`
Expected: no output.

- [ ] **Step 5: Commit**

```bash
git add pkg/client/client.go pkg/client/client_test.go
git commit -m "refactor(client): replace unused wrapper with shared dial helper

pkg/client had no callers anywhere in the module and forced insecure
credentials. Replace it with DialConfig/Dial, which the client command,
the satellite provider and the grpc provider will all share.

UseTLS is a pure function of immutable fields, which is what lets the
call sites stop writing TLS back onto shared instance structs."
```

---

### Task 2: `cliflags.ClientFlags()` and the client command

**Files:**
- Modify: `internal/cliflags/flags.go` (add helper)
- Modify: `pkg/commands/client/client_flags.go` (use helper)
- Modify: `pkg/commands/client/client.go:66-85` (use `client.Dial`)

**Interfaces:**
- Consumes: `client.DialConfig`, `client.Dial` from Task 1.
- Produces: `cliflags.ClientFlags() provider.FlagValues`, supplying `server`, `port`, `tls`, `insecure`.

**No test.** `pkg/commands/client` has zero tests today and this plan does not change that (spec §9.3).

- [ ] **Step 1: Add the shared flag helper**

In `internal/cliflags/flags.go`, alongside the existing `ConfigFlags()`/`OutputFlags()`/`TimeoutFlags()` helpers:

```go
// ClientFlags returns flags for connecting to a platform-health server.
func ClientFlags() provider.FlagValues {
	return provider.FlagValues{
		"server": {
			Shorthand:    "s",
			Kind:         provider.FlagKindString,
			DefaultValue: "localhost",
			Usage:        "server host",
		},
		"port": {
			Shorthand:    "p",
			Kind:         provider.FlagKindInt,
			DefaultValue: 8080,
			Usage:        "server port",
		},
		"tls": {
			Kind:         provider.FlagKindBool,
			DefaultValue: false,
			Usage:        "enable tls (implied by port 443 or 8443)",
		},
		"insecure": {
			Shorthand:    "k",
			Kind:         provider.FlagKindBool,
			DefaultValue: false,
			Usage:        "disable certificate verification",
		},
	}
}
```

- [ ] **Step 2: Rewrite `client_flags.go` to use it**

Replace the inline `server`/`port`/`tls`/`insecure`/`timeout` definitions in `pkg/commands/client/client_flags.go` with:

```go
var clientFlags = cliflags.Merge(
	cliflags.ComponentFlags(),
	cliflags.OutputFlags(),
	cliflags.FailFastFlags(),
	cliflags.ClientFlags(),
	cliflags.TimeoutFlags(),
)
```

`cliflags.Merge` is last-wins, so ordering matters only if two helpers define the same key. `TimeoutFlags()` supplies the existing 10s default; the inline copy it replaces was identical apart from its usage string.

- [ ] **Step 3: Replace the dial block in `client.go`**

In `pkg/commands/client/client.go`, delete the `tlsEnabled` / `dialOptions` block at lines 66-85 and the now-unused `crypto/tls`, `credentials` and `insecure` imports. Replace with:

```go
	conn, err := client.Dial(client.DialConfig{
		Host:     targetHost,
		Port:     targetPort,
		TLS:      v.GetBool("tls"),
		Insecure: v.GetBool("insecure"),
	})
	if err != nil {
		log.Error("failed to connect to server", slog.String("server", targetHost), slog.Any("error", err))
		return err
	}
	defer func() { _ = conn.Close() }()
```

Add the import `"github.com/isometry/platform-health/pkg/client"`. Note the added `defer conn.Close()`. The original leaked the connection and relied on process exit.

- [ ] **Step 4: Verify**

Run: `go build ./... && go test ./...`
Expected: build clean, all existing tests pass.

Manual check that implied TLS still behaves. Start a local server and confirm a plain connection works:
```bash
go run ./cmd/ph server -l & sleep 1 && go run ./cmd/ph client && kill %1
```
Expected: `{"status":"HEALTHY", ...}`.

- [ ] **Step 5: Commit**

```bash
git add internal/cliflags/flags.go pkg/commands/client/
git commit -m "refactor(client): dial via pkg/client, share connection flags

Adds cliflags.ClientFlags() so the server/port/tls/insecure block has one
definition rather than one per command, and fixes a leaked ClientConn in
the client command."
```

---

### Task 3: Migrate the satellite provider and remove the data race

`satellite.go:80-82` writes `i.TLS = true` on an instance struct that is **shared across concurrent `Check` RPCs** (`pkg/config/config.go:84-95` hands out the same pointers every call). Deleting the write is observably identical (`UseTLS()` recomputes the same value from immutable fields) and removes a real race.

**Files:**
- Modify: `pkg/provider/satellite/satellite.go:80-101`

**Interfaces:**
- Consumes: `client.DialConfig`, `client.Dial`.
- Produces: nothing new.

**No test.** There are no concurrency tests in this repo and CI does not run `-race` (spec §9.3). The race is removed *structurally* (the mutation is deleted, not guarded), so there is nothing to regress against.

- [ ] **Step 1: Replace the mutation and dial block**

In `GetHealth`, delete these lines entirely:

```go
	if i.Port == 443 || i.Port == 8443 {
		i.TLS = true
	}
```

and replace the `dialOptions` construction plus `grpc.NewClient` call with:

```go
	conn, err := client.Dial(client.DialConfig{
		Host:     i.Host,
		Port:     i.Port,
		TLS:      i.TLS,
		Insecure: i.Insecure,
	})
	if err != nil {
		return component.Unhealthy(err.Error())
	}
	defer func() { _ = conn.Close() }()
```

Remove the now-unused `crypto/tls`, `credentials`, `insecure` and `net` imports if nothing else in the file uses them (`fmt` and `net` may still be needed; check before deleting).

- [ ] **Step 2: Verify behaviour is unchanged**

Run: `go test ./pkg/provider/satellite/... -v`
Expected: PASS: the existing satellite tests cover the health-check path.

Run: `go build ./...`
Expected: no output.

- [ ] **Step 3: Confirm the race is gone**

Run: `go test -race ./pkg/provider/... -count=1`
Expected: PASS with no `DATA RACE` report. (Run locally only. Do **not** add `-race` to CI; that is a repo-wide policy change outside this plan's scope.)

- [ ] **Step 4: Commit**

```bash
git add pkg/provider/satellite/satellite.go
git commit -m "refactor(satellite): dial via pkg/client, drop TLS field mutation

GetHealth wrote i.TLS = true on an instance struct shared across
concurrent Check RPCs. DialConfig.UseTLS derives the same value from
immutable fields, so the write is redundant as well as racy.

Also stops dialling and closing a connection per health check where the
result was discarded anyway."
```

---

### Task 4: Migrate the gRPC provider and align the port rule

`grpc.go:74-76` implies TLS on **port 443 only**, unlike the other sites. Aligning it onto `TLSPorts` gains 8443. This is a deliberate behaviour change and fixes a latent bug: a `grpc` component on 8443 previously dialled plaintext and would fail against a TLS endpoint.

**Files:**
- Modify: `pkg/provider/grpc/grpc.go:74-92`

**Interfaces:**
- Consumes: `client.DialConfig`, `client.Dial`.
- Produces: nothing new.

**No test.** Same rationale as Task 3.

- [ ] **Step 1: Replace the mutation and dial block**

Delete:

```go
	if c.Port == 443 {
		c.TLS = true
	}
```

and replace the dial construction with:

```go
	conn, err := client.Dial(client.DialConfig{
		Host:     c.Host,
		Port:     c.Port,
		TLS:      c.TLS,
		Insecure: c.Insecure,
	})
	if err != nil {
		return component.Unhealthy(err.Error())
	}
	defer func() { _ = conn.Close() }()
```

- [ ] **Step 2: Verify**

Run: `go test ./pkg/provider/grpc/... -v && go build ./...`
Expected: PASS, clean build.

- [ ] **Step 3: Commit**

```bash
git add pkg/provider/grpc/grpc.go
git commit -m "refactor(grpc): dial via pkg/client, imply TLS on 8443

Behaviour change: the grpc provider previously implied TLS on port 443
only, while the client command and satellite provider also treat 8443 as
implying TLS. A grpc component on 8443 therefore dialled plaintext and
failed against a TLS endpoint. All four dial sites now share one rule.

Also removes the same shared-struct TLS mutation fixed in satellite."
```

---

# Phase 2: The `ph ui` Go backend

At the end of this phase `ph ui` runs, serves an SSE stream, and can be driven with `curl`, with no browser code yet.

---

### Task 5: `snapshot.go`, the canonicalisation and hashing core

The tested core of the feature. If this is wrong, change detection silently becomes a no-op that pushes on every refresh, and rows reshuffle on every push.

**Files:**
- Create: `pkg/commands/ui/snapshot.go`
- Test: `pkg/commands/ui/snapshot_test.go`

**Interfaces:**
- Consumes: `ph.HealthCheckResponse` from `pkg/platform_health`.
- Produces:
  - `func Canonicalise(resp *ph.HealthCheckResponse) *ph.HealthCheckResponse`: returns a deep copy with children recursively sorted by `(name, type)`.
  - `func PathKey(parent, name string) string`: `encodeURIComponent`-style escaping, `/`-joined.
  - `func Hash(resp *ph.HealthCheckResponse) string`: stable digest excluding `duration`.
  - `func Transitions(prev, next *ph.HealthCheckResponse) []Transition` where `type Transition struct{ Path, From, To string }`.

- [ ] **Step 1: Write the failing tests**

Create `pkg/commands/ui/snapshot_test.go`:

```go
package ui_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"google.golang.org/protobuf/types/known/durationpb"

	ph "github.com/isometry/platform-health/pkg/platform_health"
	"github.com/isometry/platform-health/pkg/commands/ui"
)

func node(name, typ string, status ph.Status, children ...*ph.HealthCheckResponse) *ph.HealthCheckResponse {
	return &ph.HealthCheckResponse{Name: name, Type: typ, Status: status, Components: children}
}

func TestHashIgnoresChildOrder(t *testing.T) {
	// Component order is nondeterministic: GetInstances ranges a map and
	// results arrive in goroutine-completion order. Two responses that differ
	// only in sibling order describe the same estate.
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

func TestCanonicaliseDoesNotMutateInput(t *testing.T) {
	in := node("", "", ph.Status_HEALTHY,
		node("zulu", "tcp", ph.Status_HEALTHY),
		node("alpha", "tcp", ph.Status_HEALTHY),
	)
	_ = ui.Canonicalise(in)
	assert.Equal(t, "zulu", in.Components[0].Name, "input must be left untouched")
}

func TestPathKey(t *testing.T) {
	tests := []struct {
		name           string
		parent, child  string
		expected       string
	}{
		{"root child has no prefix", "", "google", "google"},
		{"nested", "fluxcd", "source-controller", "fluxcd/source-controller"},
		{"slash in name is escaped", "", "a/b", "a%2Fb"},
		{"escaped name cannot collide with nesting", "a", "b", "a/b"},
		{"percent is escaped", "", "100%", "100%25"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, ui.PathKey(tt.parent, tt.child))
		})
	}
}

func TestTransitions(t *testing.T) {
	prev := node("", "", ph.Status_HEALTHY, node("db", "tcp", ph.Status_HEALTHY))
	next := node("", "", ph.Status_UNHEALTHY, node("db", "tcp", ph.Status_UNHEALTHY))

	got := ui.Transitions(ui.Canonicalise(prev), ui.Canonicalise(next))

	assert.Contains(t, got, ui.Transition{Path: "db", From: "HEALTHY", To: "UNHEALTHY"})
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./pkg/commands/ui/... -v`
Expected: FAIL: `no required module provides package .../pkg/commands/ui`, or undefined symbols once the file exists.

> **Correction, 2026-08-26.** The implementation below shipped and then failed
> review on two counts, both defects in this plan rather than in the code that
> transcribed it. Do not implement it as written. See spec 6.8 and 6.9 for the
> corrected design.
>
> 1. Sorting children on `(name, type)` is not a total order. Ties are resolved
>    by `slices.SortFunc`, which is unstable, so the hash flaps between polls.
>    Duplicate sibling names are legal config: `pkg/config/config.go:84-95` is
>    `map[providerType][]Instance`, so names are not map keys and nothing
>    validates them. Tie-break on the child's subtree digest.
> 2. Hashing `Any.Value` as raw bytes churns every scan for any `dns` component
>    with `detail: true`, because `Detail_DNS` carries a resolver TTL that counts
>    down. Resolve the `Any`, zero volatile fields, hash a deterministic
>    re-marshal.
>
> The test suite below is also too weak: seven of nine mutations against the
> implementation survived it, including deleting the details block entirely.

- [ ] **Step 3: Write the implementation**

Create `pkg/commands/ui/snapshot.go`:

```go
package ui

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/url"
	"slices"
	"strings"

	"google.golang.org/protobuf/proto"

	ph "github.com/isometry/platform-health/pkg/platform_health"
)

// Transition records one component's status change between two scans.
type Transition struct {
	Path string `json:"path"`
	From string `json:"from"`
	To   string `json:"to"`
}

// PathKey joins a parent path and a component name into a stable unique key.
// Names come from YAML keys and may legally contain "/", so each segment is
// escaped: a root component named "a/b" must not collide with "b" nested under
// system "a".
func PathKey(parent, name string) string {
	escaped := url.PathEscape(name)
	escaped = strings.ReplaceAll(escaped, "/", "%2F")
	if parent == "" {
		return escaped
	}
	return parent + "/" + escaped
}

// Canonicalise returns a deep copy with children recursively sorted by
// (name, type). The server's ordering is nondeterministic, so this is required
// before hashing or marshalling. The input is never mutated: responses are
// shared and protobuf messages are not safe for concurrent mutation.
func Canonicalise(resp *ph.HealthCheckResponse) *ph.HealthCheckResponse {
	if resp == nil {
		return nil
	}
	out := proto.Clone(resp).(*ph.HealthCheckResponse)
	sortTree(out)
	return out
}

func sortTree(n *ph.HealthCheckResponse) {
	slices.SortFunc(n.Components, func(a, b *ph.HealthCheckResponse) int {
		if c := strings.Compare(a.GetName(), b.GetName()); c != 0 {
			return c
		}
		return strings.Compare(a.GetType(), b.GetType())
	})
	for _, c := range n.Components {
		sortTree(c)
	}
}

// Hash returns a digest of the semantically meaningful content of a
// canonicalised tree. Duration is deliberately excluded: it is set at every
// node and jitters on every scan, so including it would make change detection
// 100% false-positive. Details ARE included, so a rotated certificate or a
// kstatus flip under an unchanged HEALTHY is still visible.
func Hash(resp *ph.HealthCheckResponse) string {
	h := sha256.New()
	hashNode(h, "", resp)
	return hex.EncodeToString(h.Sum(nil))
}

func hashNode(h interface{ Write([]byte) (int, error) }, parent string, n *ph.HealthCheckResponse) {
	if n == nil {
		return
	}
	path := parent
	if n.GetName() != "" {
		path = PathKey(parent, n.GetName())
	}
	fmt.Fprintf(h, "%s\x00%s\x00%s\x00%t\x00%s\x00",
		path, n.GetType(), n.GetStatus().String(), n.GetFailFastTriggered(), n.GetServerId())
	for _, m := range n.GetMessages() {
		// Message order is meaningful: CEL failures are appended in expression
		// order. Hash as-ordered.
		fmt.Fprintf(h, "m:%s\x00", m)
	}
	for _, d := range n.GetDetails() {
		fmt.Fprintf(h, "d:%s\x00", d.GetTypeUrl())
		_, _ = h.Write(d.GetValue())
	}
	for _, c := range n.GetComponents() {
		hashNode(h, path, c)
	}
}

// Transitions reports per-path status changes between two canonicalised trees.
// Paths present in only one tree are reported with an empty From or To.
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
	if n.GetName() != "" {
		path = PathKey(parent, n.GetName())
		m[path] = n.GetStatus().String()
	}
	for _, c := range n.GetComponents() {
		collectStatuses(m, path, c)
	}
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./pkg/commands/ui/... -v`
Expected: PASS: all six test functions.

- [ ] **Step 5: Commit**

```bash
git add pkg/commands/ui/snapshot.go pkg/commands/ui/snapshot_test.go
git commit -m "feat(ui): add snapshot canonicalisation, path keys and hashing

Server component ordering is nondeterministic from three independent
sources, so a naive hash differs on every poll and change detection
becomes a no-op. Canonicalise sorts children by (name, type) before
hashing or marshalling.

Duration is excluded from the hash (it jitters every scan); details are
included, so a cert rotation under an unchanged HEALTHY is still seen."
```

---

### Task 6: `scanner.go`, connection ownership and the trigger loop

**Files:**
- Create: `pkg/commands/ui/scanner.go`

**Interfaces:**
- Consumes: `Canonicalise`, `Hash`, `Transitions` (Task 5); `client.Dial`, `client.DialConfig` (Task 1).
- Produces:
  - `type Scanner struct{...}` with `NewScanner(rootCtx context.Context, cfg ScannerConfig) (*Scanner, error)`
  - `func (s *Scanner) Trigger(reason string) TriggerState`: **takes no context, deliberately**
  - `func (s *Scanner) Run()`: the loop; returns when `rootCtx` is done
  - `func (s *Scanner) Subscribe() *Subscriber` / `func (s *Scanner) Unsubscribe(*Subscriber)`
  - `func (s *Scanner) State() StoreState`: snapshot bytes, hash, scanID, seq, observedAt, lastError, scanState
  - `type TriggerState string` with constants `TriggerStarted`, `TriggerQueued`, `TriggerCoalesced`

**No test.** This is command runtime wiring (spec §9.3). Verified manually in Task 9.

- [ ] **Step 1: Write the scanner**

Key invariants to encode, each with a comment explaining *why* (they are all non-obvious and all cost real debugging if reverted):

```go
// Trigger takes no context.Context, deliberately and permanently.
//
// If it accepted one, the natural implementation in an HTTP handler passes
// r.Context() — and then the first-subscriber scan is cancelled every time a
// browser's EventSource reconnects, which it does every few seconds while a
// page loads. The scan context is derived internally from rootCtx only.
func (s *Scanner) Trigger(reason string) TriggerState {
	select {
	case s.trigger <- reason:
		if s.scanning.Load() {
			return TriggerQueued
		}
		return TriggerStarted
	default:
		return TriggerCoalesced // one already queued; coalesce into it
	}
}
```

The loop: one goroutine owns the connection, the timer and the subscriber registry, so "subscriber count reached zero" is a serialised event rather than a race between handlers:

```go
func (s *Scanner) Run() {
	defer close(s.done)

	timer := time.NewTimer(time.Hour)
	if !timer.Stop() {
		<-timer.C
	}
	defer timer.Stop()

	for {
		select {
		case <-s.rootCtx.Done():
			return

		case sub := <-s.register:
			s.subs[sub] = struct{}{}
			s.replayTo(sub) // full state: snapshot + lastError + scanState
			if len(s.subs) == 1 {
				s.armTimer(timer)
			}
			// Trigger a first scan only if we have nothing AND have not tried
			// recently: EventSource reconnects every ~3s, so without the floor
			// a down server produces a scan per reconnect.
			if s.store.snapshot == nil && time.Since(s.lastAttempt) > s.floor() {
				s.Trigger("first-subscriber")
			}

		case sub := <-s.unregister:
			delete(s.subs, sub)
			if len(s.subs) == 0 {
				stopTimer(timer)
			}
			// An in-flight scan is NOT cancelled here. Cancelling would unwind
			// provider.Check mid-probe on the server, leaving half-open
			// connections against production endpoints every time a tab closes.

		case reason := <-s.trigger:
			s.runScan(reason)
			s.armTimer(timer) // reset from COMPLETION, never a fixed ticker

		case <-timer.C:
			if len(s.subs) > 0 { // re-check: Stop() does not drain a delivered tick
				s.runScan("refresh")
			}
			s.armTimer(timer)
		}
	}
}
```

`runScan` derives its own context, recovers from panics, marshals once, and broadcasts:

```go
func (s *Scanner) runScan(reason string) {
	defer func() {
		if r := recover(); r != nil {
			s.setError(fmt.Errorf("panic during scan: %v", r))
		}
	}()

	s.scanning.Store(true)
	defer s.scanning.Store(false)

	seq := s.seq.Add(1)
	scanID := uuid.Must(uuid.NewV7()).String()
	s.broadcastScanning(scanID, seq)

	ctx, cancel := context.WithTimeout(s.rootCtx, s.cfg.Timeout)
	defer cancel()

	s.lastAttempt = time.Now()
	resp, err := s.health.Check(ctx, &ph.HealthCheckRequest{})
	if err != nil {
		s.setError(err)
		s.broadcastScanError(scanID, seq, err)
		return
	}

	canon := Canonicalise(resp)
	hash := Hash(canon)
	transitions := Transitions(s.store.canon, canon)

	// Marshal ONCE, on this goroutine, and store only bytes. Downstream code
	// must never receive the live message: internal/output mutates its
	// argument, and protobuf messages are not safe for concurrent mutation.
	payload, err := protojson.MarshalOptions{
		Multiline:         false,
		EmitDefaultValues: true, // so UNKNOWN nodes carry a status key
	}.Marshal(canon)
	if err != nil {
		s.setError(err)
		s.broadcastScanError(scanID, seq, err)
		return
	}

	changed := hash != s.store.hash
	s.store.update(payload, canon, hash, scanID, seq, time.Now(), transitions)

	s.broadcastScan(scanID, seq, changed) // liveness: EVERY scan, changed or not
	if changed {
		s.broadcastSnapshot()
	}
}
```

Dial options, per spec §6.13:

```go
conn, err := client.Dial(cfg.Dial,
	// Default backoff ceiling is 120s, so an unattended dashboard stays dark
	// for two minutes after the server recovers. ResetConnectBackoff is
	// experimental and its own doc says not to use it, so fix it here instead.
	grpc.WithConnectParams(grpc.ConnectParams{Backoff: backoff.Config{
		BaseDelay: time.Second, Multiplier: 1.6, Jitter: 0.2, MaxDelay: 15 * time.Second,
	}}),
	// The client sends no keepalive pings by default, so an idle socket across
	// a NAT is silently reaped. 5m is the server's EnforcementPolicy minimum —
	// going lower earns a GOAWAY ENHANCE_YOUR_CALM.
	grpc.WithKeepaliveParams(keepalive.ClientParameters{
		Time: 5 * time.Minute, Timeout: 20 * time.Second,
	}),
	// Default is 4MB; a large estate with detail:true exceeds it, and the
	// failure mode is ResourceExhausted on every scan forever.
	grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(32<<20)),
)
```

- [ ] **Step 2: Verify it compiles**

Run: `go build ./...`
Expected: no output.

- [ ] **Step 3: Commit**

```bash
git add pkg/commands/ui/scanner.go
git commit -m "feat(ui): add scan loop with coalescing trigger and snapshot store

One goroutine owns the connection, timer and subscriber registry, so
subscriber-count transitions are serialised rather than raced between
HTTP handlers.

Trigger() takes no context by design: accepting one invites passing
r.Context(), which would cancel scans on every EventSource reconnect.
Semantics are at most one scan in flight and at most one queued, so an
explicit press during a scan is honoured rather than silently dropped."
```

---

### Task 7: `sse.go`, the hub and the SSE handler

**Files:**
- Create: `pkg/commands/ui/sse.go`
- Test: `pkg/commands/ui/sse_test.go`

**Interfaces:**
- Consumes: `Scanner.Subscribe`/`Unsubscribe`/`State`.
- Produces: `func (s *Scanner) SSEHandler() http.HandlerFunc`, `func writeEvent(w io.Writer, event string, data []byte) error`.

**One test**, per spec §9.2: `httptest` has precedent in two provider tests, and the `data: ` prefixing bug fails silently in a browser with no error anywhere.

- [ ] **Step 1: Write the failing test**

Create `pkg/commands/ui/sse_test.go`:

```go
package ui_test

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/isometry/platform-health/pkg/commands/ui"
)

func TestWriteEventPrefixesEveryLine(t *testing.T) {
	// SSE framing is line-oriented. A payload containing a newline must have
	// EVERY line prefixed, or the browser silently discards the remainder and
	// reports no error at all.
	var buf bytes.Buffer
	require.NoError(t, ui.WriteEvent(&buf, "snapshot", []byte("{\n  \"a\": 1\n}")))

	assert.Equal(t, "event: snapshot\ndata: {\ndata:   \"a\": 1\ndata: }\n\n", buf.String())
}

func TestWriteEventSingleLine(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, ui.WriteEvent(&buf, "scan", []byte(`{"seq":1}`)))

	assert.Equal(t, "event: scan\ndata: {\"seq\":1}\n\n", buf.String())
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/commands/ui/... -run TestWriteEvent -v`
Expected: FAIL: `undefined: ui.WriteEvent`.

- [ ] **Step 3: Implement `WriteEvent` and the handler**

```go
// WriteEvent writes one SSE frame. Every line of data is prefixed, because SSE
// framing is line-oriented: an unprefixed line is treated as an unknown field
// and silently dropped by the browser's parser.
func WriteEvent(w io.Writer, event string, data []byte) error {
	if _, err := fmt.Fprintf(w, "event: %s\n", event); err != nil {
		return err
	}
	for _, line := range bytes.Split(data, []byte("\n")) {
		if _, err := fmt.Fprintf(w, "data: %s\n", line); err != nil {
			return err
		}
	}
	_, err := fmt.Fprint(w, "\n")
	return err
}
```

The handler: the subscriber's own goroutine is the **sole writer**, because `http.ResponseWriter` is not safe for concurrent use:

```go
func (s *Scanner) SSEHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Type", "text/event-stream")
		h.Set("Cache-Control", "no-cache, no-transform") // no-transform stops proxy buffering
		h.Set("Connection", "keep-alive")
		h.Set("X-Accel-Buffering", "no")
		h.Set("X-Content-Type-Options", "nosniff")

		rc := http.NewResponseController(w)
		// Lengthen the browser's reconnect interval from its 3s default.
		fmt.Fprint(w, "retry: 5000\n\n")
		_ = rc.Flush()

		sub := s.Subscribe()
		defer s.Unsubscribe(sub)

		heartbeat := time.NewTicker(20 * time.Second)
		defer heartbeat.Stop()

		for {
			select {
			case <-r.Context().Done():
				return
			case <-s.shutdown:
				_ = WriteEvent(w, "shutdown", []byte(`{}`))
				_ = rc.Flush()
				return
			case <-sub.doorbell:
				// Without a write deadline, a backgrounded tab that stops
				// reading fills the send buffer and parks this goroutine in
				// write(2) forever — r.Context() is NOT cancelled, because the
				// connection is still open. That leak also blocks shutdown.
				_ = rc.SetWriteDeadline(time.Now().Add(10 * time.Second))
				if err := sub.drainTo(w); err != nil {
					return
				}
				if err := rc.Flush(); err != nil {
					return
				}
			case <-heartbeat.C:
				_ = rc.SetWriteDeadline(time.Now().Add(10 * time.Second))
				if _, err := fmt.Fprint(w, ":heartbeat\n\n"); err != nil {
					return
				}
				if err := rc.Flush(); err != nil {
					return
				}
			}
		}
	}
}
```

Subscriber state: a mutex-guarded `pending` snapshot that is **replaced, not queued** (snapshots are full state, so dropping an intermediate is correct), a bounded drop-oldest ring for transient events, and a cap-1 doorbell. The hub sets state and rings the doorbell; it never writes bytes and never blocks. Lock order is `hub.mu` → `sub.mu`, never the reverse, and only the hub closes a subscriber.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./pkg/commands/ui/... -v`
Expected: PASS: including the Task 5 tests.

- [ ] **Step 5: Commit**

```bash
git add pkg/commands/ui/sse.go pkg/commands/ui/sse_test.go
git commit -m "feat(ui): add SSE hub and handler

The hub never writes bytes: it sets per-subscriber state and rings a
doorbell, so one slow reader cannot stall the broadcast or the scan.
Each subscriber's handler goroutine is its sole writer, since
http.ResponseWriter is not safe for concurrent use.

Every write sets a deadline. Without one, a backgrounded tab that stops
reading parks the handler in write(2) indefinitely, since the request
context is not cancelled while the connection remains open."
```

---

### Task 8: `http.go`, mux, security middleware and assets

**Files:**
- Create: `pkg/commands/ui/http.go`
- Create: `pkg/commands/ui/assets/index.html` (placeholder shell, replaced in Task 10)

**Interfaces:**
- Consumes: `Scanner.SSEHandler`, `Scanner.Trigger`.
- Produces: `func (s *Scanner) Mux(assets fs.FS) *http.ServeMux`, `func guard(listen string, next http.Handler) http.Handler`.

**No test.** HTTP wiring (spec §9.3). Verified with `curl` in Task 9.

- [ ] **Step 1: Write the middleware and mux**

The two controls that matter, with the reasoning inline so nobody removes them as ceremony:

```go
// guard defends the two attacks that loopback binding does NOT prevent.
//
// DNS rebinding: an attacker's page rebinds to 127.0.0.1, becomes same-origin,
// opens the EventSource, and reads the whole snapshot — internal hostnames,
// k8s resource names, Vault addresses, TLS SANs. The browser is inside the
// loopback boundary, so binding to localhost does not help. Host validation
// defeats it outright.
//
// CSRF: any origin can POST a form to /api/scan. It is a simple request, so it
// is not preflighted; CORS blocks reading the response, not sending it. Each
// scan re-probes every configured endpoint, making this a remote-triggered
// probe amplifier pointed at production.
func guard(listen string, next http.Handler) http.Handler {
	allowed := allowedHosts(listen)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !slices.Contains(allowed, r.Host) {
			http.Error(w, "bad host", http.StatusForbidden)
			return
		}
		if r.Method == http.MethodPost {
			if site := r.Header.Get("Sec-Fetch-Site"); site != "" && site != "same-origin" {
				http.Error(w, "cross-origin request rejected", http.StatusForbidden)
				return
			}
		}
		w.Header().Set("Content-Security-Policy",
			"default-src 'none'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'; connect-src 'self'")
		next.ServeHTTP(w, r)
	})
}
```

> **Note on `'unsafe-inline'`:** the pre-paint theme script in `index.html` must run before first paint to avoid a white flash on dark-mode load, and it is authored by us, not derived from any health-check data. If you prefer a strict policy, replace it with a nonce generated per request, but do **not** move the theme script to an external file, which reintroduces the flash.

Routes: `GET /` and `/app.js`, `/app.css` from the embedded FS; `GET /api/events` (SSE); `POST /api/scan` returning `202` with `{"state":"started"|"queued"|"coalesced"}`.

- [ ] **Step 2: Verify it compiles and serves**

Run: `go build ./...`
Expected: no output.

- [ ] **Step 3: Commit**

```bash
git add pkg/commands/ui/http.go pkg/commands/ui/assets/
git commit -m "feat(ui): add HTTP mux with host and origin guards

Loopback binding does not mitigate DNS rebinding (the browser is inside
the loopback boundary) or form-POST CSRF (a simple request is not
preflighted). Host-header validation and a Sec-Fetch-Site check do."
```

---

### Task 9: `ui.go` and `ui_flags.go`, command wiring and shutdown

**Files:**
- Create: `pkg/commands/ui/ui.go`, `pkg/commands/ui/ui_flags.go`
- Modify: `pkg/commands/root/root.go` (import + `AddCommand`)

**Interfaces:**
- Consumes: everything from Tasks 5-8.
- Produces: `func New() *cobra.Command`.

**No test.** Command wiring. Manual verification below is the gate.

- [ ] **Step 1: Write the flags**

```go
var uiFlags = cliflags.Merge(
	cliflags.ClientFlags(),
	cliflags.TimeoutFlags(),
	provider.FlagValues{
		"timeout": {Shorthand: "t", Kind: provider.FlagKindDuration, DefaultValue: 30 * time.Second,
			Usage: "per-scan timeout"}, // Merge is last-wins; overrides the 10s default
		"listen": {Kind: provider.FlagKindString, DefaultValue: "127.0.0.1:8090",
			Usage: "local dashboard address (note: ph server's --listen defaults to ALL interfaces)"},
		"refresh": {Kind: provider.FlagKindDuration, DefaultValue: time.Duration(0),
			Usage: "auto-refresh interval; 0 disables auto-refresh entirely"},
		"open":    {Kind: provider.FlagKindBool, DefaultValue: false, Usage: "open a browser on start"},
		"fixture": {Kind: provider.FlagKindString, DefaultValue: "", Usage: "serve a canned snapshot; no server connection"},
	},
)
```

- [ ] **Step 2: Write the command, including the blank import**

```go
package ui

import (
	// ... cobra, etc.

	// Required: protojson resolves Any details through the global registry and
	// returns an error if a type is unregistered — and that error kills
	// marshalling of the ENTIRE response, not just the detail. Without this,
	// every scan fails the moment any provider sets detail: true.
	_ "github.com/isometry/platform-health/pkg/platform_health/details"
)
```

Validation in `setup`: reject `--fixture` combined with target flags; when `--refresh` is non-zero, **refuse** `refresh <= timeout` and **warn** below `2 * timeout`; refuse a non-loopback `--listen` unless explicitly opted in.

- [ ] **Step 3: Write the shutdown sequence**

Order is load-bearing: `http.Server.Shutdown()` blocks on active handlers, and an SSE handler never returns on its own:

```go
	<-ctx.Done()                       // 1. SIGINT cancels rootCtx
	// 2. scanner sees rootCtx.Done(), stops the timer, finishes any in-flight scan
	close(scanner.shutdown)            // 3. release every SSE handler
	shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutCtx)          // 4. bounded, so a wedged handler cannot hang exit
	<-scanner.done                     // 5. wait for the scanner BEFORE closing the conn:
	_ = conn.Close()                   //    closing first makes the in-flight Check fail
	                                   //    with a confusing Unavailable
```

Do **not** substitute `srv.Close()`: it drops the POST handler mid-flight too.

- [ ] **Step 4: Register the command**

In `pkg/commands/root/root.go`, add the import and `cmd.AddCommand(ui.New())` alongside the existing subcommands.

- [ ] **Step 5: Verify end to end**

```bash
go build ./... && go run ./cmd/ph server -l -vv &
sleep 1
go run ./cmd/ph ui &
sleep 2

# SSE stream: expect retry:, then a snapshot frame with every line prefixed
curl -sN -H 'Host: 127.0.0.1:8090' http://127.0.0.1:8090/api/events | head -20

# Scan trigger
curl -s -X POST -H 'Host: 127.0.0.1:8090' -H 'Sec-Fetch-Site: same-origin' \
     http://127.0.0.1:8090/api/scan
# Expected: {"state":"started"} or {"state":"queued"}

# Guards reject a rebound host and a cross-origin POST
curl -s -o /dev/null -w '%{http_code}\n' -H 'Host: evil.com' http://127.0.0.1:8090/api/events
# Expected: 403
curl -s -o /dev/null -w '%{http_code}\n' -X POST -H 'Host: 127.0.0.1:8090' \
     -H 'Sec-Fetch-Site: cross-site' http://127.0.0.1:8090/api/scan
# Expected: 403

kill %1 %2
```

Then verify graceful shutdown: with `ph ui` running and a `curl -N` stream attached, press Ctrl-C. Expected: exits within ~1s, not hanging on the open stream.

- [ ] **Step 6: Commit**

```bash
git add pkg/commands/ui/ui.go pkg/commands/ui/ui_flags.go pkg/commands/root/root.go
git commit -m "feat(ui): add the ph ui command and register it

Shutdown order is load-bearing: SSE handlers must be released before
http.Server.Shutdown, which otherwise blocks forever on them, and the
gRPC connection must outlive the scanner so an in-flight scan does not
fail with a confusing Unavailable."
```

---

# Phase 3: The browser client

No test harness (spec §9.3). Reconciler, path keying, duration parsing and layout are still written as **pure functions** so a harness can be added later without a rewrite. Each task's gate is a specific visual check against `--fixture`.

---

### Task 10: Shell, theme, and the Signal palette

**Files:**
- Modify: `pkg/commands/ui/assets/index.html`
- Create: `pkg/commands/ui/assets/app.css`

- [ ] **Step 1: Write the shell with a pre-paint theme script**

```html
<script>
  // Runs before first paint: without this, a dark-mode user sees a white flash.
  (function () {
    try {
      var t = localStorage.getItem('theme');
      if (t === 'light' || t === 'dark') document.documentElement.dataset.theme = t;
    } catch (e) { /* private mode: fall through to prefers-color-scheme */ }
  })();
</script>
```

- [ ] **Step 2: Write the palette as custom properties**

Define the complete light palette on bare `:root`, then redefine **only** the changed tokens under `@media (prefers-color-scheme: dark)` guarded as `:root:not([data-theme="light"])`, and again under `:root[data-theme="dark"]` so a manual pin wins in both directions. Dark: `--bg:#08090c`, `--ok:#22e07a`, `--bad:#ff2d55`, `--accent:#5aa9ff`, with glow via `box-shadow`. Light: `--bg:#f6f7fa`, `--ok:#0f9d55`, `--bad:#d81b45`, `--accent:#2f7ef7`, glow replaced by a tinted halo ring: a blur on white reads as a smudge, not as energy.

- [ ] **Step 3: Implement the cycling theme button**

One button cycling `auto → light → dark → auto`, showing the icon for its *current* state with a tooltip naming the next. In auto, a `matchMedia('(prefers-color-scheme: dark)')` listener re-follows the OS live.

- [ ] **Step 4: Verify**

Run `go run ./cmd/ph ui --fixture testdata/snapshot.json --open`. Check: no white flash on load with the OS in dark mode; the button cycles all three states; the pinned choice survives a reload; in auto, flipping the OS theme changes the page without a reload.

- [ ] **Step 5: Commit:** `feat(ui): add dashboard shell, Signal palette and theme toggle`

---

### Task 11: Model, path keying, reconciler, and the component rail

**Files:**
- Create: `pkg/commands/ui/assets/app.js`

- [ ] **Step 1: Wire the SSE client**

```js
// Named events mean onmessage NEVER fires — it only handles unnamed events.
// Forgetting one addEventListener fails silently and completely.
for (const name of ['snapshot', 'scan', 'scanning', 'scan-error', 'connection', 'shutdown']) {
  es.addEventListener(name, (e) => handlers[name](JSON.parse(e.data || '{}')));
}
```

- [ ] **Step 2: Build the path-keyed index**

Walk the tree ourselves: **not** via `Flatten`, which drops `Components`, aliases slices, and elides satellite nodes. The satellite node is exactly what a topology view should show: it is the boundary between two servers. Escape each segment so a component named `a/b` cannot collide with `b` under system `a`.

- [ ] **Step 3: Implement keyed reconciliation**

```js
// Rows are updated in place, never destroyed and rebuilt. That is what makes
// expand/collapse, selection and scroll position survive a push for free.
// View state lives here, separate from server data, and is never derived from it.
const view = { expanded: new Set(), selected: null, filter: '', zoom: { x: 0, y: 0, k: 1 } };
```

A vanished path drops its view state; a vanished **selected** path clears the selection and the dock. Nodes appearing and disappearing mid-session is normal: the server hot-reloads config, and `kubernetes` selector mode churns with the cluster.

- [ ] **Step 4: Render the rail, with collapse policy**

Indented tree, disclosure triangles, status dot, type, duration. Any node with **more than 25 children** renders collapsed, badged with child count and unhealthy count. The Unhealthy filter searches the **full model**, not visible rows. On the **first** snapshot only, auto-expand paths leading to unhealthy nodes, never on later snapshots, since an auto-expand that fights a manual collapse every 30 seconds is worse than none.

`duration` is the canonical proto3 string (`"0.076347145s"`): strip the trailing `s`, `parseFloat`, format for humans.

**Every insertion uses `textContent`.** Never `innerHTML`.

- [ ] **Step 5: Verify:** with `--fixture`, confirm: expanding a node then forcing a scan leaves it expanded; selection survives a push; a 142-child node renders collapsed with a badge; filtering finds a node inside a collapsed parent.

- [ ] **Step 6: Commit:** `feat(ui): add client model, keyed reconciler and component rail`

---

### Task 12: The graph view

**Files:**
- Modify: `pkg/commands/ui/assets/app.js`

- [ ] **Step 1: Implement tidy-tree layout as a pure function**

`layout(root, expandedSet) -> {nodes:[{path,x,y,status,type,collapsedCount}], edges:[{from,to}]}`. Reingold-Tilford, laying out **only visible nodes**: a collapsed parent is one badged node, so the layout never places more than the tree is showing.

The data is a **strict tree**: the only edges are containment (`system`→children, `satellite`→remote subtree, `kubernetes`→one child per matched resource). There are **no dependency edges** in the model; `order` is a sibling-scoped execution wave and is never in the response. The view shows topology, not dependencies. Do not imply otherwise in labels or legend.

- [ ] **Step 2: Render SVG with pan and zoom**

One transform on a single `<g>`, which keeps hit-testing trivial. Selection draws the accent ring. No layout animation in v1.

- [ ] **Step 3: Link selection three ways:** clicking a row rings the node and fills the dock; clicking a node scrolls the row into view.

- [ ] **Step 4: Verify:** with `--fixture`, confirm pan/zoom work, the zoom transform survives a push, clicking either surface selects in both, and a collapsed parent shows as one badged node.

- [ ] **Step 5: Commit:** `feat(ui): add tidy-tree graph view with linked selection`

---

### Task 13: Dock, degraded states, and the transition log

**Files:**
- Modify: `pkg/commands/ui/assets/app.js`, `app.css`

- [ ] **Step 1: Build the detail dock:** one collapsed line expanding to messages and rendered details (TLS chain and expiry, kstatus conditions, DNS answers).

- [ ] **Step 2: Render `failFastTriggered` prominently**

It means the tree is **incomplete**, and it can arrive from a satellite configured with `fail_fast: true` even though the dashboard never requests it. Showing a partial estate as complete is the worst failure this UI can have: a banner, not a footnote.

- [ ] **Step 3: Implement every degraded state:** no snapshot yet; first scan failed (show dialled address and gRPC error); disconnected; connected with zero components; results incomplete. Map connection states explicitly: `READY` ok, **`IDLE` benign** (with manual refresh the channel idles after 30 minutes and would otherwise show a false alarm), `CONNECTING` transient, `TRANSIENT_FAILURE` red, `SHUTDOWN` terminal.

- [ ] **Step 4: Add liveness and the transition log:** "last checked 3s ago · sampled every 30s" driven by the `scan` event, and a transition list from `transitions[]`. Say "sampled", because flapping between two polls is undetectable by any design here and the UI must not imply continuity.

- [ ] **Step 5: Manage the connection lifecycle**

```js
// Close on hidden: frees one of the browser's six per-origin connections (a
// limit shared across ALL tabs), and removes the backgrounded-tab case where a
// stalled reader parks the server's write.
document.addEventListener('visibilitychange', () => { /* close after grace / reopen */ });
window.addEventListener('pagehide', () => es.close());
```

On `shutdown`, call `es.close()` explicitly and render "server stopped"; otherwise `EventSource` reconnects against a dead port every 5s forever.

- [ ] **Step 6: Verify:** exercise each degraded state via `--fixture` and by killing the server mid-session; confirm the tab-hidden path closes and reopens the stream.

- [ ] **Step 7: Commit:** `feat(ui): add detail dock, degraded states and transition log`

---

# Phase 4: Documentation and CI

---

### Task 14: Docs and CodeQL

**Files:**
- Modify: `README.md`, `CLAUDE.md`, `.github/workflows/codeql.yaml`

- [ ] **Step 1: README:** a Dashboard subsection under `## Usage`, after "Context Inspection". Cover `ph ui`, `--listen`, `--refresh`, and state plainly that v1 is local-use only.

- [ ] **Step 2: CLAUDE.md:** add `ui` to the subcommand list in the architecture section; note that it is the repo's first `go:embed` and first HTTP server; note that `ph ui` is unreachable from the `phc`/`phs` shims because they splice a subcommand into `os.Args`.

- [ ] **Step 3: CodeQL:** add `javascript` to the language matrix, currently `["go"]` only. This is the repo's first browser code and would otherwise go unanalysed.

- [ ] **Step 4: Verify:** `helm template platform-health deploy/charts/platform-health | kubeconform -strict -summary` still passes (the chart is deliberately untouched), and `go test ./...` is green.

- [ ] **Step 5: Commit:** `docs: document ph ui and add javascript to CodeQL`

---

## Self-review

**Spec coverage.** §4.1→T9, §4.2→T11, §4.3→T12, §4.4→T6/T9, §4.5→T1-4, §4.6 (no `-c` flag in T9's flag set), §4.7→T10, §5.1→T5-9, §5.2→T9, §5.3→T9, §5.4→T8, §6.1-6.6→T6, §6.7-6.9→T5/T6, §6.10-6.11→T7, §6.12→T9, §6.13→T6, §7.1-7.2→T11, §7.3→T11, §7.4→T12, §7.5→T11/T13, §7.6→T13, §7.7→T10, §7.8→T13, §8→T13, §9→per-task test notes, §10→T14. No gaps.

**Type consistency.** `DialConfig`/`Dial`/`UseTLS`/`Address` are used identically in Tasks 1-4. `Canonicalise`/`Hash`/`Transitions`/`PathKey`/`Transition` defined in Task 5 are consumed with matching signatures in Task 6. `WriteEvent` is defined and tested in Task 7 and used in Task 6's broadcast helpers. `Trigger`/`TriggerState` are consistent between Tasks 6 and 8. The SSE event names in Task 6 (`snapshot`, `scan`, `scanning`, `scan-error`, `connection`) match Task 11's listener list, plus `shutdown` from Tasks 7 and 9.

**Known deviation from the skill's default, stated deliberately.** Tasks 2, 3, 4, 6, 8, 9 and 10-13 have **no tests**, and this is intentional per spec §9 rather than an omission: only 2 of this repo's 8 command packages have any test, there are no concurrency tests anywhere, and there is no JS toolchain. Tasks 1, 5 and 7 carry real red-green cycles and cover the logic where a silent failure would be most expensive. Anyone executing this plan should not "helpfully" add the missing tests without raising it first.
