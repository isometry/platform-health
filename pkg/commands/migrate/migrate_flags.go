package migrate

import (
	"github.com/isometry/platform-health/internal/cliflags"
	"github.com/isometry/platform-health/pkg/provider"
)

var migrateFlags = cliflags.Merge(
	provider.FlagValues{
		"output": {
			Shorthand: "O",
			Kind:      "string",
			Usage:     "output file path (default: stdout)",
		},
	},
)
