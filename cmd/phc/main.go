package main

import (
	"fmt"
	"os"

	"github.com/isometry/platform-health/pkg/commands/root"

	// import details to support google.protobuf.Any
	_ "github.com/isometry/platform-health/pkg/platform_health/details"
)

var (
	version string = "snapshot"
	commit  string = "unknown"
	date    string = "unknown"
)

func main() {
	// Inject "client" subcommand for backward compatibility
	os.Args = append([]string{os.Args[0], "client"}, os.Args[1:]...)

	cmd := root.New()
	cmd.Version = fmt.Sprintf("%s-%s (built %s)", version, commit, date)
	err := cmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}
