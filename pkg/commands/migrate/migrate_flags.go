package migrate

import "github.com/isometry/platform-health/pkg/commands/flags"

var migrateFlags = flags.Merge(
	flags.FlagValues{
		"output": {
			Shorthand: "O",
			Kind:      "string",
			Usage:     "output file path (default: stdout)",
		},
	},
)
