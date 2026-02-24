package main

import (
	"fmt"
	"os"

	"github.com/isometry/platform-health/pkg/commands/root"

	// import providers to trigger registration
	_ "github.com/isometry/platform-health/pkg/provider/dns"
	_ "github.com/isometry/platform-health/pkg/provider/grpc"
	_ "github.com/isometry/platform-health/pkg/provider/helm"
	_ "github.com/isometry/platform-health/pkg/provider/http"
	_ "github.com/isometry/platform-health/pkg/provider/kubernetes"
	_ "github.com/isometry/platform-health/pkg/provider/satellite"
	_ "github.com/isometry/platform-health/pkg/provider/ssh"
	_ "github.com/isometry/platform-health/pkg/provider/system"
	_ "github.com/isometry/platform-health/pkg/provider/tcp"
	_ "github.com/isometry/platform-health/pkg/provider/tls"
	_ "github.com/isometry/platform-health/pkg/provider/vault"

	// import details to support google.protobuf.Any
	_ "github.com/isometry/platform-health/pkg/platform_health/details"
)

var (
	version string = "snapshot"
	commit  string = "unknown"
	date    string = "unknown"
)

func main() {
	cmd := root.New()
	cmd.Version = fmt.Sprintf("%s-%s (built %s)", version, commit, date)
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
