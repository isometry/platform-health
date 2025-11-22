package client

import (
	"time"

	"github.com/isometry/platform-health/pkg/commands/flags"
)

var clientFlags = flags.Merge(
	flags.ComponentFlags(&components),
	flags.OutputFlags(&flatOutput, &quietLevel),
	flags.FlagValues{
		"server": {
			Shorthand:    "s",
			Kind:         "string",
			Variable:     &targetHost,
			DefaultValue: "localhost",
			Usage:        "server host",
		},
		"port": {
			Shorthand:    "p",
			Kind:         "int",
			Variable:     &targetPort,
			DefaultValue: 8080,
			Usage:        "server port",
		},
		"tls": {
			Kind:         "bool",
			Variable:     &tlsClient,
			DefaultValue: false,
			Usage:        "enable tls",
		},
		"insecure": {
			Shorthand:    "k",
			Kind:         "bool",
			Variable:     &insecureSkipVerify,
			DefaultValue: false,
			Usage:        "disable certificate verification",
		},
		"timeout": {
			Shorthand:    "t",
			Kind:         "duration",
			Variable:     &clientTimeout,
			DefaultValue: 10 * time.Second,
			Usage:        "timeout",
		},
	},
)
