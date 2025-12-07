package check

import (
	"github.com/isometry/platform-health/pkg/commands/flags"
)

var checkFlags = flags.Merge(
	flags.ConfigFlags(),
	flags.ComponentFlags(),
	flags.OutputFlags(),
	flags.FailFastFlags(),
	flags.ParallelismFlags(),
	flags.TimeoutFlags(),
)
