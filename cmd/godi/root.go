package main

import (
	"io"

	"github.com/spf13/cobra"
)

func newRootCmd(stdout, stderr io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "godi",
		Short: "Read and render godi dependency graphs",
		Long: `Read and render godi dependency graphs.

godi containers write their graph as JSON - on a failed build when
GODI_SNAPSHOT_ON_BUILD_ERR is set, or through graph.Graph.WriteJSON. This turns
one into a drawing, an outline, or a page in a browser.`,
		// An error in the work is not a mistake in the command line. Printing the
		// usage for one buries what went wrong.
		SilenceUsage: true,
	}

	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.AddCommand(newExportCmd(), newViewCmd())

	return cmd
}
