// Command opcli is a CLI and MCP server for OpenProject.
package main

import (
	"os"

	"github.com/carlosprados/openproject-cli/internal/cli"
)

// Set by GoReleaser via ldflags.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	os.Exit(cli.Execute(cli.BuildInfo{Version: version, Commit: commit, Date: date}))
}
