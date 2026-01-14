package context

import (
	"github.com/isometry/platform-health/internal/cli"
	"github.com/isometry/platform-health/pkg/provider"
)

var contextFlags = cli.Merge(
	cli.ConfigFlags(),
	provider.FlagValues{
		"output-format": {
			Shorthand:    "o",
			Kind:         "string",
			DefaultValue: "json",
			Usage:        "output format (json|yaml)",
		},
	},
)
