package main

import (
	"fmt"
	"os"

	"github.com/isometry/platform-health/pkg/commands/client"

	// import details to support google.protobuf.Any
	_ "github.com/isometry/platform-health/pkg/platform_health/details"
)

var (
	version string = "snapshot"
	commit  string = "unknown"
	date    string = "unknown"
)

func main() {
	rootCmd := client.ClientCmd
	rootCmd.Version = fmt.Sprintf("%s-%s (built %s)", version, commit, date)
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}
