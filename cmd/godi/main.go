// Command godi turns the dependency graph of a godi container into something to
// look at.
//
// A container that fails to build writes its graph as JSON, when
// GODI_SNAPSHOT_ON_BUILD_ERR says to. This is what reads it:
//
//	godi view /tmp/godi-graph-4821.json
//	godi export dot graph.json | dot -Tsvg -o graph.svg
//
// The library writes JSON and nothing else, so that no godi binary carries a
// renderer or the means to start a browser. All of that lives here instead,
// installed once and used from the terminal.
package main

import (
	"context"
	"os"
	"os/signal"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	err := newRootCmd(os.Stdout, os.Stderr).ExecuteContext(ctx)
	if err != nil {
		os.Exit(1)
	}
}
