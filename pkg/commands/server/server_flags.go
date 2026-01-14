package server

import (
	"github.com/isometry/platform-health/internal/cli"
	"github.com/isometry/platform-health/pkg/provider"
)

var serverFlags = cli.Merge(
	cli.ConfigFlags(),
	cli.ParallelismFlags(),
	provider.FlagValues{
		"listen": {
			Shorthand:    "l",
			Kind:         "string",
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
			Shorthand: "H",
			Kind:      "bool",
			Usage:     "disable gRPC Health v1",
		},
		"grpc-reflection": {
			Shorthand: "R",
			Kind:      "bool",
			Usage:     "enable gRPC reflection",
		},
	},
)
