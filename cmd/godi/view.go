package main

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/michalkurzeja/godi/v2/graph"
	"github.com/michalkurzeja/godi/v2/graph/serve"
)

func newViewCmd() *cobra.Command {
	var (
		htmlOpts htmlOptions
		addr     string
		noOpen   bool
	)

	cmd := &cobra.Command{
		Use:   "view [file]",
		Short: "Serve the graph and open it in a browser",
		Long: `Serve the graph and open it in a browser.

The page is built for each request, so no file is written anywhere. The server
runs until you stop it.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			g, err := readGraph(cmd, args)
			if err != nil {
				return err
			}

			enc, err := htmlOpts.encoder()
			if err != nil {
				return err
			}

			srv, err := serve.Listen(addr, graph.Static(g), serve.WithEncoder(enc))
			if err != nil {
				return err
			}

			fmt.Fprintf(cmd.ErrOrStderr(), "godi: serving on %s\n", srv.URL())

			if !noOpen {
				err = launch(srv.URL())
				if err != nil {
					// The address is on screen either way, so this is a
					// nuisance rather than a failure.
					fmt.Fprintf(cmd.ErrOrStderr(), "godi: could not open a browser: %s\n", err)
				}
			}

			return serveUntilDone(cmd.Context(), srv)
		},
	}

	htmlOpts.register(cmd)
	cmd.Flags().StringVar(&addr, "addr", "127.0.0.1:0", "address to serve on; port 0 takes whatever is free")
	cmd.Flags().BoolVar(&noOpen, "no-open", false, "print the address without opening a browser")
	completeGraphFiles(cmd)

	return cmd
}

// serveUntilDone serves until the context is cancelled, which is what an
// interrupt does. Stopping on purpose is not an error.
func serveUntilDone(ctx context.Context, srv *serve.Server) error {
	done := make(chan error, 1)
	go func() { done <- srv.Serve() }()

	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		stopCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()

		err := srv.Shutdown(stopCtx)
		if err != nil {
			return err
		}
		return <-done
	}
}
