package context

import (
	"github.com/isometry/platform-health/internal/cliflags"
	"github.com/isometry/platform-health/pkg/provider"
)

var contextFlags = cliflags.Merge(
	cliflags.ConfigFlags(),
	cliflags.TimeoutFlags(),
	provider.FlagValues{
		"output-format": {
			Shorthand:    "o",
			Kind:         provider.FlagKindString,
			DefaultValue: "json",
			Usage:        "output format (json|yaml)",
		},
	},
)
