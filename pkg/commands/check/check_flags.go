package check

import (
	"github.com/isometry/platform-health/internal/cli"
)

var checkFlags = cli.Merge(
	cli.ConfigFlags(),
	cli.ComponentFlags(),
	cli.OutputFlags(),
	cli.FailFastFlags(),
	cli.ParallelismFlags(),
	cli.TimeoutFlags(),
)
