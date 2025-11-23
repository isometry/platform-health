package migrate

import "github.com/isometry/platform-health/pkg/commands/flags"

var migrateFlags = flags.Merge(
	flags.FlagValues{
		"output": {
			Shorthand:    "o",
			Kind:         "string",
			DefaultValue: "",
			Usage:        "output file path (default: stdout)",
		},
	},
)
