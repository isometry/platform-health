package check

import (
	"github.com/isometry/platform-health/internal/cliflags"
)

var checkFlags = cliflags.Merge(
	cliflags.ConfigFlags(),
	cliflags.ComponentFlags(),
	cliflags.OutputFlags(),
	cliflags.FailFastFlags(),
	cliflags.ParallelismFlags(),
	cliflags.TimeoutFlags(),
)
