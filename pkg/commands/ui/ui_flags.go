//go:build ui

package ui

import (
	"time"

	"github.com/isometry/platform-health/internal/cliflags"
	"github.com/isometry/platform-health/pkg/provider"
)

// uiFlags deliberately omits -c/--component. Filtering is client side: a typo
// in a server side component filter returns a successful RPC carrying
// UNHEALTHY, no components and no duration, which renders as an empty red
// screen indistinguishable from a total outage.
var uiFlags = cliflags.Merge(
	cliflags.ClientFlags(),
	cliflags.TimeoutFlags(),
	provider.FlagValues{
		// Merge is last-wins, so this replaces TimeoutFlags' 10s default.
		"timeout": {
			Shorthand:    "t",
			Kind:         provider.FlagKindDuration,
			DefaultValue: 30 * time.Second,
			Usage:        "per-scan timeout",
		},
		"listen": {
			Kind:         provider.FlagKindString,
			DefaultValue: "127.0.0.1:8090",
			Usage:        "dashboard address as host:port (note: 'ph server --listen' takes a bare host and binds all interfaces by default, the opposite polarity)",
		},
		"refresh": {
			Kind:         provider.FlagKindDuration,
			DefaultValue: time.Duration(0),
			Usage:        "auto-refresh interval; 0 disables auto-refresh entirely (note: elsewhere in ph, such as --parallelism, 0 means the default)",
		},
		"open": {
			Kind:         provider.FlagKindBool,
			DefaultValue: false,
			Usage:        "open the dashboard in a browser on start",
		},
		"fixture": {
			Kind:         provider.FlagKindString,
			DefaultValue: "",
			Usage:        "serve a canned snapshot from a protojson file; no server connection",
		},
		"allow-remote": {
			Kind:         provider.FlagKindBool,
			DefaultValue: false,
			Usage:        "permit a non-loopback --listen address; the dashboard is unauthenticated, so anyone who can reach it sees the whole estate",
		},
	},
)
