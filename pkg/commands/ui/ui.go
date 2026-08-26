package ui

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/isometry/platform-health/internal/cliflags"
	"github.com/isometry/platform-health/pkg/client"
	"github.com/isometry/platform-health/pkg/netutil"
	"github.com/isometry/platform-health/pkg/phctx"
	"github.com/isometry/platform-health/pkg/ui"
)

// shutdownTimeout bounds http.Server.Shutdown, so a wedged handler cannot hang
// exit.
const shutdownTimeout = 5 * time.Second

// targetFlags are the flags that name a gRPC server, and so conflict with
// --fixture.
var targetFlags = []string{"server", "port", "tls", "insecure"}

func New() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "ui [host:port]",
		Short:   "Serve a live health dashboard",
		Long:    "Poll a Platform Health gRPC server and serve a live dashboard over server-sent events.",
		Args:    cobra.MaximumNArgs(1),
		PreRunE: setup,
		RunE:    serve,
	}

	uiFlags.Register(cmd.Flags(), false)

	return cmd
}

func setup(cmd *cobra.Command, args []string) error {
	v := phctx.Viper(cmd.Context())
	cliflags.BindFlags(cmd, v)

	if len(args) == 1 {
		host, port, err := netutil.ParseHostPort(args[0])
		if err != nil {
			return err
		}
		v.Set("server", host)
		v.Set("port", port)
	}

	if v.GetString("fixture") != "" && (len(args) == 1 || anyChanged(cmd, targetFlags...)) {
		return errors.New("--fixture serves a canned snapshot and cannot be combined with a target server")
	}

	listen := v.GetString("listen")
	if _, _, err := net.SplitHostPort(listen); err != nil {
		return fmt.Errorf("--listen %q must be host:port: %w", listen, err)
	}
	if !isLoopback(listen) && !v.GetBool("allow-remote") {
		return fmt.Errorf("--listen %q is not loopback and the dashboard is unauthenticated; pass --allow-remote to bind it anyway", listen)
	}

	// Normalise exactly as the scanner does, or --timeout 0 validates against 0
	// while the scan actually runs for defaultTimeout.
	timeout := v.GetDuration("timeout")
	if timeout <= 0 {
		timeout = ui.DefaultTimeout
	}
	return checkRefresh(cmd, v.GetDuration("refresh"), timeout)
}

// checkRefresh rejects a refresh interval a scan cannot fit inside, and warns
// about one with no headroom. Both checks are skipped when refresh is 0, which
// disables auto-refresh entirely.
func checkRefresh(cmd *cobra.Command, refresh, timeout time.Duration) error {
	if refresh <= 0 {
		return nil
	}
	if refresh <= timeout {
		return fmt.Errorf("--refresh %s must exceed --timeout %s, or the next scan is due before the current one can fail", refresh, timeout)
	}
	if refresh < 2*timeout {
		// Stderr, not slog: the default log level is error, so a warning nobody
		// sees is not a warning.
		fmt.Fprintf(cmd.ErrOrStderr(), "warning: --refresh %s leaves little headroom over --timeout %s; a slow estate will scan almost continuously\n", refresh, timeout)
	}
	return nil
}

func serve(cmd *cobra.Command, _ []string) error {
	v := phctx.Viper(cmd.Context())
	log := phctx.Logger(cmd.Context())

	rootCtx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg := ui.ScannerConfig{
		Dial: client.DialConfig{
			Host:     v.GetString("server"),
			Port:     v.GetInt("port"),
			TLS:      v.GetBool("tls"),
			Insecure: v.GetBool("insecure"),
		},
		Timeout: v.GetDuration("timeout"),
		Refresh: v.GetDuration("refresh"),
	}

	var (
		scanner *ui.Scanner
		err     error
	)
	if fixture := v.GetString("fixture"); fixture != "" {
		scanner, err = ui.NewFixtureScanner(rootCtx, cfg, fixture)
	} else {
		scanner, err = ui.NewScanner(rootCtx, cfg)
	}
	if err != nil {
		log.Error("failed to start scanner", slog.Any("error", err))
		return err
	}
	// Backstop for the early returns below. Close is idempotent, so it does not
	// disturb the ordered shutdown at the end.
	defer func() { _ = scanner.Close() }()

	listen := v.GetString("listen")
	listener, err := net.Listen("tcp", listen)
	if err != nil {
		log.Error("failed to open listener", slog.String("listen", listen), slog.Any("error", err))
		return err
	}

	srv := &http.Server{
		Handler:           scanner.Mux(listen, ui.Assets()),
		ReadHeaderTimeout: 10 * time.Second,
	}

	go scanner.Run()

	serveErr := make(chan error, 1)
	go func() { serveErr <- srv.Serve(listener) }()

	log.Info("dashboard listening", slog.String("listen", listen), slog.String("target", scanner.Target()))
	fmt.Fprintf(cmd.OutOrStdout(), "platform-health dashboard: http://%s\n", listen)

	if v.GetBool("open") {
		switch {
		case !isLoopback(listen):
			fmt.Fprintf(cmd.ErrOrStderr(), "warning: --open ignored because --listen %s is not loopback; open the dashboard from the machine you want to view it on\n", listen)
		default:
			if err := openBrowser("http://" + listen); err != nil {
				log.Warn("failed to open browser", slog.Any("error", err))
			}
		}
	}

	select {
	case <-rootCtx.Done():
	case err = <-serveErr:
		if errors.Is(err, http.ErrServerClosed) {
			err = nil
		}
	}

	// Order is load-bearing. stop cancels rootCtx, which is the only thing that
	// makes Run return; Release lets SSE handlers return, which Shutdown would
	// otherwise wait on forever; and the connection outlives Run so an
	// in-flight Check does not fail with a confusing Unavailable.
	stop()
	scanner.Release()

	shutCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	_ = srv.Shutdown(shutCtx)

	<-scanner.Done()
	_ = scanner.Close()

	return err
}

func anyChanged(cmd *cobra.Command, names ...string) bool {
	for _, name := range names {
		if cmd.Flags().Changed(name) {
			return true
		}
	}
	return false
}

// isLoopback reports whether a host:port binds only the local machine. An empty
// host means every interface, so it is not loopback.
func isLoopback(listen string) bool {
	host, _, err := net.SplitHostPort(listen)
	if err != nil {
		return false
	}
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func openBrowser(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	// Reap the launcher, which exits as soon as it has handed off to the
	// browser, or it lingers as a zombie for the lifetime of the dashboard.
	go func() { _ = cmd.Wait() }()
	return nil
}
