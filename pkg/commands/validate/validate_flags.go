package validate

import (
	"github.com/isometry/platform-health/internal/cliflags"
	"github.com/isometry/platform-health/pkg/provider"
)

var validateFlags = cliflags.Merge(
	cliflags.ConfigFlags(),
	provider.FlagValues{
		"output": {
			Shorthand:    "o",
			Kind:         "string",
			DefaultValue: "text",
			Usage:        "output format (text|json)",
		},
	},
)
