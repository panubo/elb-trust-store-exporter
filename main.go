package main

import (
	"os"

	"github.com/panubo/elb-trust-store-exporter/cmd"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
	builtBy = "unknown"
)

func main() {
	cmd.Version = version
	cmd.Commit = commit
	cmd.Date = date
	cmd.BuiltBy = builtBy
	cmd.Run(os.Args)
}
