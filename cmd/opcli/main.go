// Command opcli is a CLI for OpenProject, for humans and AI agents.
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
