//go:build ui

package ui

import (
	"embed"
	"encoding/json"
	"io/fs"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
)

// assetsFS embeds the dashboard shell: index.html, app.css and theme.js.
//
//go:embed assets
var assetsFS embed.FS

// Assets roots the embedded FS at "assets" so index.html serves at "/".
func Assets() fs.FS {
	sub, err := fs.Sub(assetsFS, "assets")
	if err != nil {
		panic(err)
	}
	return sub
}

// Mux builds the routes and wraps them in guard, so a caller cannot construct
// an unguarded handler. addr is the bound listener address, not the --listen
// string: only the bound address knows the port an ephemeral bind received.
// Never add compression middleware: it would silently break SSE.
func (s *Scanner) Mux(addr net.Addr, assets fs.FS) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/", noStaleAssets(http.FileServerFS(assets)))
	mux.HandleFunc("GET /api/events", s.sseHandler())
	mux.HandleFunc("POST /api/scan", s.handleScan)
	return guard(addr, mux)
}

// noStaleAssets forces revalidation. Embedded files report a zero modtime, so
// FileServerFS emits neither ETag nor Last-Modified and a browser falls back to
// heuristic caching, serving a previous build's assets after an upgrade.
func noStaleAssets(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-cache")
		next.ServeHTTP(w, r)
	})
}

// handleScan reports what Trigger actually did: started, queued or coalesced.
func (s *Scanner) handleScan(w http.ResponseWriter, r *http.Request) {
	state := s.Trigger("manual")
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(map[string]TriggerState{"state": state})
}

// guard defends what loopback binding does not: DNS rebinding makes an
// attacker's page same-origin with 127.0.0.1, so only Host validation stops
// it reading the snapshot, and a POST is an unpreflighted simple request, so
// only Sec-Fetch-Site (or, absent that, a matching Origin) stops a remote
// probe-amplifier CSRF against /api/scan.
//
// The Host allow-list applies to a loopback bind. A non-loopback bind has no
// enumerable set of valid Hosts (a kubelet probe sends the pod IP, a viewer
// sends whatever name routes there), so it accepts any Host and leaves
// rebinding defence to the network; the CSRF check still applies.
func guard(addr net.Addr, next http.Handler) http.Handler {
	allowed := allowedHosts(addr)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if allowed != nil && !slices.Contains(allowed, r.Host) {
			http.Error(w, "bad host", http.StatusForbidden)
			return
		}
		if r.Method == http.MethodPost && crossOrigin(r) {
			http.Error(w, "cross-origin request rejected", http.StatusForbidden)
			return
		}
		w.Header().Set("Content-Security-Policy",
			"default-src 'none'; script-src 'self'; style-src 'self'; connect-src 'self'")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		next.ServeHTTP(w, r)
	})
}

// crossOrigin trusts Sec-Fetch-Site when present; older browsers that omit it
// still send Origin on a POST, which must then name the request's own Host.
func crossOrigin(r *http.Request) bool {
	if site := r.Header.Get("Sec-Fetch-Site"); site != "" {
		return site != "same-origin"
	}
	if origin := r.Header.Get("Origin"); origin != "" {
		u, err := url.Parse(origin)
		return err != nil || !sameHost(u.Host, r.Host)
	}
	return false
}

// sameHost compares two host[:port] values, treating an omitted port as 80,
// which is how a browser writes the default port in both Host and Origin.
func sameHost(a, b string) bool {
	return strings.TrimSuffix(a, ":80") == strings.TrimSuffix(b, ":80")
}

// allowedHosts lists the accepted Host values for a loopback bind: the bound
// address, its loopback spellings and, on port 80, their port-less forms. It
// returns nil for a non-loopback bind, meaning any Host is accepted.
func allowedHosts(addr net.Addr) []string {
	tcp, ok := addr.(*net.TCPAddr)
	if !ok {
		return []string{addr.String()}
	}
	if !tcp.IP.IsLoopback() {
		return nil
	}
	port := strconv.Itoa(tcp.Port)
	hosts := []string{
		tcp.String(),
		"127.0.0.1:" + port,
		"[::1]:" + port,
		"localhost:" + port,
	}
	if tcp.Port == 80 {
		hosts = append(hosts, "127.0.0.1", "[::1]", "localhost")
	}
	return hosts
}
