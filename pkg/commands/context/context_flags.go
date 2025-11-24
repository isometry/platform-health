package context

import (
	"github.com/isometry/platform-health/pkg/commands/flags"
)

var contextFlags = flags.Merge(
	flags.ConfigFlags(),
	flags.FlagValues{
		"output-format": {
			Shorthand:    "o",
			Kind:         "string",
			DefaultValue: "json",
			Usage:        "output format (json|yaml)",
		},
	},
)
