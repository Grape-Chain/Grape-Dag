package main

import (
	"fmt"
	"os"

	"github.com/Grape-Chain/Grape-Dag/run"
)

func main() {
	// Subcommands are taken off the front of os.Args before anything else
	// touches the flag package: run.Start parses os.Args with the global flag
	// set, so an unrecognised first word would be read as a peer flag and stop
	// the node. Anything that is not a subcommand falls through to exactly the
	// path it took before subcommands existed.
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "join":
			exitOn(runJoin(os.Args[2:], os.Stdout), "join")
			return
		case "status":
			exitOn(runStatus(os.Args[2:], os.Stdout), "status")
			return
		}
	}

	// Server for pprof
	run.Start()
}

func exitOn(err error, subcommand string) {
	if err == nil {
		return
	}
	fmt.Fprintf(os.Stderr, "grapepeer %s: %s\n", subcommand, err)
	os.Exit(1)
}
