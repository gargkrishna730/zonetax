// Command zonetax-cli is a thin HTTP client against the collector's REST API.
package main

import (
	"flag"
	"fmt"
	"os"
)

func main() {
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "zonetax-cli: query cross-AZ Kubernetes network cost")
		fmt.Fprintln(os.Stderr, "\nUsage:")
		fmt.Fprintln(os.Stderr, "  zonetax-cli top              show current top cross-AZ cost offenders")
		fmt.Fprintln(os.Stderr, "  zonetax-cli report --since 1h   show cost summary over a window")
		fmt.Fprintln(os.Stderr, "\nNot yet implemented — see ROADMAP.md M5.")
	}
	flag.Parse()
	flag.Usage()
	os.Exit(1)
}
