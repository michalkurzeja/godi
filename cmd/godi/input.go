package main

import (
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/michalkurzeja/godi/v2/graph"
)

// readGraph reads the graph named on the command line, or standard input when
// nothing is named or the name is "-". Reading a pipe is what makes the export
// commands worth chaining.
func readGraph(cmd *cobra.Command, args []string) (*graph.Graph, error) {
	path := "-"
	if len(args) > 0 {
		path = args[0]
	}

	if path == "-" {
		g, _, err := graph.ReadJSON(cmd.InOrStdin())
		return g, err
	}

	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	g, _, err := graph.ReadJSON(f)
	return g, err
}

// openOutput returns where to write. The graph goes to standard output unless
// --output names a file, so that a pipe into dot is the ordinary case.
func openOutput(cmd *cobra.Command) (io.Writer, func(), error) {
	path, err := cmd.Flags().GetString("output")
	if err != nil {
		return nil, nil, err
	}
	if path == "" || path == "-" {
		return cmd.OutOrStdout(), func() {}, nil
	}

	f, err := os.Create(path)
	if err != nil {
		return nil, nil, err
	}
	return f, func() { f.Close() }, nil
}
