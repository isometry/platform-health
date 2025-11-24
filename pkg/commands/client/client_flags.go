package client

import (
	"time"

	"github.com/isometry/platform-health/pkg/commands/flags"
)

var clientFlags = flags.Merge(
	flags.ComponentFlags(),
	flags.OutputFlags(),
	flags.FailFastFlags(),
	flags.FlagValues{
		"server": {
			Shorthand:    "s",
			Kind:         "string",
			DefaultValue: "localhost",
			Usage:        "server host",
		},
		"port": {
			Shorthand:    "p",
			Kind:         "int",
			DefaultValue: 8080,
			Usage:        "server port",
		},
		"tls": {
			Kind:         "bool",
			DefaultValue: false,
			Usage:        "enable tls",
		},
		"insecure": {
			Shorthand:    "k",
			Kind:         "bool",
			DefaultValue: false,
			Usage:        "disable certificate verification",
		},
		"timeout": {
			Shorthand:    "t",
			Kind:         "duration",
			DefaultValue: 10 * time.Second,
			Usage:        "timeout",
		},
	},
)
