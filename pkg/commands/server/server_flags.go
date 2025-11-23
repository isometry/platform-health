package server

import (
	"github.com/isometry/platform-health/pkg/commands/flags"
)

var serverFlags = flags.Merge(
	flags.ConfigFlags(),
	flags.FlagValues{
		"listen": {
			Shorthand:    "l",
			Kind:         "string",
			DefaultValue: "",
			NoOptDefault: "localhost",
			Usage:        "listen on host (default all interfaces)",
		},
		"port": {
			Shorthand:    "p",
			Kind:         "int",
			DefaultValue: 8080,
			Usage:        "listen on port",
		},
		"no-grpc-health-v1": {
			Shorthand:    "H",
			Kind:         "bool",
			DefaultValue: false,
			Usage:        "disable gRPC Health v1",
		},
		"grpc-reflection": {
			Shorthand:    "R",
			Kind:         "bool",
			DefaultValue: false,
			Usage:        "enable gRPC reflection",
		},
	},
)
