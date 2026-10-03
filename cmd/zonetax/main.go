// Command zonetax is a small CLI for the ZoneTax collector's REST API.
//
//	zonetax top    [--range 24h] [-n 10]   top cross-AZ routes by cost for a time window
//	zonetax report [--range 24h]           totals plus an hourly cost breakdown
//	zonetax version
//
// Point it at a collector with --url or ZONETAX_URL (default http://localhost:8080), e.g. after
// `kubectl -n zonetax port-forward svc/zonetax-collector 8080:8080`.
package main

import (
	"fmt"
	"io"
	"os"
)

// version is overridden at build time via -ldflags "-X main.version=v0.1.0".
var version = "dev"

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

const usage = `zonetax: cross-AZ Kubernetes network cost from your terminal

Usage:
  zonetax top    [--range 24h] [-n 10] [--json]   top cross-AZ routes by cost
  zonetax report [--range 24h] [--json]           totals + hourly breakdown
  zonetax version

Ranges: 15m, 1h, 6h, 24h, 7d (report breakdown needs 1h or more).

Global flags (any position after the command):
  --url string   collector base URL (env ZONETAX_URL, default http://localhost:8080)
  --json         print the raw API JSON instead of a table

Reach the collector first, for example:
  kubectl -n zonetax port-forward svc/zonetax-collector 8080:8080
`

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, usage)
		return 2
	}
	var err error
	switch args[0] {
	case "top":
		err = cmdTop(args[1:], stdout)
	case "report":
		err = cmdReport(args[1:], stdout)
	case "version", "--version", "-v":
		fmt.Fprintf(stdout, "zonetax %s\n", version)
	case "help", "--help", "-h":
		fmt.Fprint(stdout, usage)
	default:
		fmt.Fprintf(stderr, "unknown command %q\n\n%s", args[0], usage)
		return 2
	}
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}
	return 0
}
