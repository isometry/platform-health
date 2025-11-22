package check

import (
	"github.com/isometry/platform-health/pkg/commands/flags"
)

var checkFlags = flags.Merge(
	flags.ConfigFlags(&configPaths, &configName),
	flags.ComponentFlags(&components),
	flags.OutputFlags(&flatOutput, &quietLevel),
)
