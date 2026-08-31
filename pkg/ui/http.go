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
// an unguarded handler. Never add compression middleware: it would silently break SSE.
func (s *Scanner) Mux(listen string, assets fs.FS) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/", noStaleAssets(http.FileServerFS(assets)))
	mux.HandleFunc("GET /api/events", s.sseHandler())
	mux.HandleFunc("POST /api/scan", s.handleScan)
	return guard(listen, mux)
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
func guard(listen string, next http.Handler) http.Handler {
	allowed := allowedHosts(listen)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !slices.Contains(allowed, r.Host) {
			http.Error(w, "bad host", http.StatusForbidden)
			return
		}
		if r.Method == http.MethodPost && crossOrigin(r, allowed) {
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
// still send Origin on a POST, so a mismatching Origin is rejected too.
func crossOrigin(r *http.Request, allowed []string) bool {
	if site := r.Header.Get("Sec-Fetch-Site"); site != "" {
		return site != "same-origin"
	}
	if origin := r.Header.Get("Origin"); origin != "" {
		u, err := url.Parse(origin)
		return err != nil || !slices.Contains(allowed, u.Host)
	}
	return false
}

// allowedHosts lists accepted Host values: listen itself, plus its loopback spellings.
func allowedHosts(listen string) []string {
	_, port, err := net.SplitHostPort(listen)
	if err != nil {
		return []string{listen}
	}
	return []string{
		listen,
		"127.0.0.1:" + port,
		"[::1]:" + port,
		"localhost:" + port,
	}
}
