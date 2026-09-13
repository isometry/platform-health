//go:build ui

package ui_test

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/isometry/platform-health/pkg/ui"
)

func tcpAddr(t *testing.T, hostport string) net.Addr {
	t.Helper()
	addr, err := net.ResolveTCPAddr("tcp", hostport)
	require.NoError(t, err)
	return addr
}

func newMux(t *testing.T, addr net.Addr) http.Handler {
	t.Helper()
	s, err := ui.NewFixtureScanner(context.Background(), ui.ScannerConfig{}, "testdata/fixture.json")
	require.NoError(t, err)
	return s.Mux(addr, ui.Assets())
}

func get(h http.Handler, host string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Host = host
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func post(h http.Handler, host string, headers map[string]string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(http.MethodPost, "/api/scan", nil)
	r.Host = host
	for k, v := range headers {
		r.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func TestGuardLoopbackAcceptsLoopbackSpellings(t *testing.T) {
	h := newMux(t, tcpAddr(t, "127.0.0.1:8090"))
	for _, host := range []string{"127.0.0.1:8090", "[::1]:8090", "localhost:8090"} {
		w := get(h, host)
		assert.Equal(t, http.StatusOK, w.Code, host)
		assert.NotEmpty(t, w.Header().Get("Content-Security-Policy"), host)
	}
}

func TestGuardLoopbackRejectsForeignHost(t *testing.T) {
	h := newMux(t, tcpAddr(t, "127.0.0.1:8090"))
	for _, host := range []string{"evil.example:8090", "127.0.0.1:9999", "localhost", "10.1.2.3:8090"} {
		assert.Equal(t, http.StatusForbidden, get(h, host).Code, host)
	}
}

func TestGuardLoopbackDefaultPortMayBeOmitted(t *testing.T) {
	h := newMux(t, tcpAddr(t, "127.0.0.1:80"))
	for _, host := range []string{"localhost", "localhost:80", "127.0.0.1", "[::1]"} {
		assert.Equal(t, http.StatusOK, get(h, host).Code, host)
	}
}

func TestGuardUsesTheBoundPort(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer l.Close()

	h := newMux(t, l.Addr())
	assert.Equal(t, http.StatusOK, get(h, l.Addr().String()).Code)
	assert.Equal(t, http.StatusForbidden, get(h, "127.0.0.1:0").Code)
}

func TestGuardRemoteAcceptsAnyHost(t *testing.T) {
	for _, bind := range []string{"0.0.0.0:8090", "10.0.0.5:8090", "[::]:8090"} {
		h := newMux(t, tcpAddr(t, bind))
		for _, host := range []string{"10.1.2.3:8090", "dashboard.example", "localhost:8090"} {
			assert.Equal(t, http.StatusOK, get(h, host).Code, "%s via %s", host, bind)
		}
	}
}

func TestGuardPostSecFetchSite(t *testing.T) {
	h := newMux(t, tcpAddr(t, "127.0.0.1:8090"))
	assert.Equal(t, http.StatusForbidden, post(h, "localhost:8090", map[string]string{"Sec-Fetch-Site": "cross-site"}).Code)
	assert.Equal(t, http.StatusAccepted, post(h, "localhost:8090", map[string]string{"Sec-Fetch-Site": "same-origin"}).Code)
}

func TestGuardPostOriginFallback(t *testing.T) {
	h := newMux(t, tcpAddr(t, "127.0.0.1:8090"))
	assert.Equal(t, http.StatusAccepted, post(h, "localhost:8090", map[string]string{"Origin": "http://localhost:8090"}).Code)
	assert.Equal(t, http.StatusForbidden, post(h, "localhost:8090", map[string]string{"Origin": "http://evil.example"}).Code)
	assert.Equal(t, http.StatusAccepted, post(h, "localhost:8090", nil).Code)

	h80 := newMux(t, tcpAddr(t, "127.0.0.1:80"))
	assert.Equal(t, http.StatusAccepted, post(h80, "localhost", map[string]string{"Origin": "http://localhost"}).Code)
	assert.Equal(t, http.StatusAccepted, post(h80, "localhost", map[string]string{"Origin": "http://localhost:80"}).Code)
}

func TestGuardPostRemoteOriginMustMatchHost(t *testing.T) {
	h := newMux(t, tcpAddr(t, "0.0.0.0:8090"))
	assert.Equal(t, http.StatusAccepted, post(h, "10.1.2.3:8090", map[string]string{"Origin": "http://10.1.2.3:8090"}).Code)
	assert.Equal(t, http.StatusForbidden, post(h, "10.1.2.3:8090", map[string]string{"Origin": "http://evil.example"}).Code)
	assert.Equal(t, http.StatusForbidden, post(h, "10.1.2.3:8090", map[string]string{"Sec-Fetch-Site": "cross-site"}).Code)
}
