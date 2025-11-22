package server

import (
	"github.com/isometry/platform-health/pkg/commands/flags"
)

var serverFlags = flags.Merge(
	flags.ConfigFlags(&configPaths, &configName),
	flags.FlagValues{
		"listen": {
			Shorthand:    "l",
			Kind:         "string",
			Variable:     &listenHost,
			DefaultValue: "",
			NoOptDefault: "localhost",
			Usage:        "listen on host (default all interfaces)",
		},
		"port": {
			Shorthand:    "p",
			Kind:         "int",
			Variable:     &listenPort,
			DefaultValue: 8080,
			Usage:        "listen on port",
		},
		"no-grpc-health-v1": {
			Shorthand:    "H",
			Kind:         "bool",
			Variable:     &noGrpcHealthV1,
			DefaultValue: false,
			Usage:        "disable gRPC Health v1",
		},
		"grpc-reflection": {
			Shorthand:    "R",
			Kind:         "bool",
			Variable:     &grpcReflection,
			DefaultValue: false,
			Usage:        "enable gRPC reflection",
		},
	},
)
