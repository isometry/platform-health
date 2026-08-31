package client

import (
	"github.com/isometry/platform-health/internal/cliflags"
)

var clientFlags = cliflags.Merge(
	cliflags.ComponentFlags(),
	cliflags.OutputFlags(),
	cliflags.FailFastFlags(),
	cliflags.ClientFlags(),
	cliflags.TimeoutFlags(),
)
