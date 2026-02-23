package server

import (
	"github.com/isometry/platform-health/internal/cliflags"
	"github.com/isometry/platform-health/pkg/provider"
)

var serverFlags = cliflags.Merge(
	cliflags.ConfigFlags(),
	cliflags.ParallelismFlags(),
	provider.FlagValues{
		"listen": {
			Shorthand:    "l",
			Kind:         provider.FlagKindString,
			NoOptDefault: "localhost",
			Usage:        "listen on host (default all interfaces)",
		},
		"port": {
			Shorthand:    "p",
			Kind:         provider.FlagKindInt,
			DefaultValue: 8080,
			Usage:        "listen on port",
		},
		"no-grpc-health-v1": {
			Shorthand: "H",
			Kind:      provider.FlagKindBool,
			Usage:     "disable gRPC Health v1",
		},
		"grpc-reflection": {
			Shorthand: "R",
			Kind:      provider.FlagKindBool,
			Usage:     "enable gRPC reflection",
		},
	},
)
