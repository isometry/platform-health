package migrate

import (
	"github.com/isometry/platform-health/internal/cli"
	"github.com/isometry/platform-health/pkg/provider"
)

var migrateFlags = cli.Merge(
	provider.FlagValues{
		"output": {
			Shorthand: "O",
			Kind:      "string",
			Usage:     "output file path (default: stdout)",
		},
	},
)
