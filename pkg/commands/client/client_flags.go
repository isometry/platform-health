package client

import (
	"time"

	"github.com/isometry/platform-health/internal/cliflags"
	"github.com/isometry/platform-health/pkg/provider"
)

var clientFlags = cliflags.Merge(
	cliflags.ComponentFlags(),
	cliflags.OutputFlags(),
	cliflags.FailFastFlags(),
	provider.FlagValues{
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
			Usage:        "enable tls",
		},
		"insecure": {
			Shorthand:    "k",
			Kind:         provider.FlagKindBool,
			DefaultValue: false,
			Usage:        "disable certificate verification",
		},
		"timeout": {
			Shorthand:    "t",
			Kind:         provider.FlagKindDuration,
			DefaultValue: 10 * time.Second,
			Usage:        "timeout",
		},
	},
)
