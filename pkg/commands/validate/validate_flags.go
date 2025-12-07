package validate

import (
	"github.com/isometry/platform-health/pkg/commands/flags"
)

var validateFlags = flags.Merge(
	flags.ConfigFlags(),
	flags.FlagValues{
		"output": {
			Shorthand:    "o",
			Kind:         "string",
			DefaultValue: "text",
			Usage:        "output format (text|json)",
		},
	},
)
