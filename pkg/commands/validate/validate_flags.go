package validate

import (
	"github.com/isometry/platform-health/internal/cli"
	"github.com/isometry/platform-health/pkg/provider"
)

var validateFlags = cli.Merge(
	cli.ConfigFlags(),
	provider.FlagValues{
		"output": {
			Shorthand:    "o",
			Kind:         "string",
			DefaultValue: "text",
			Usage:        "output format (text|json)",
		},
	},
)
