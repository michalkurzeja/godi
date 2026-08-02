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

			srv, err := listen(cmd, addr, graph.Static(g), serve.WithEncoder(enc))
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
	cmd.Flags().StringVar(&addr, "addr", defaultAddr,
		"address to serve on; port 0 takes whatever is free, as the default does when it is already in use")
	cmd.Flags().BoolVar(&noOpen, "no-open", false, "print the address without opening a browser")
	completeGraphFiles(cmd)

	return cmd
}

// defaultAddr is a fixed port rather than whatever is free, because the page
// keeps the reader's settings — the colour scheme, what the wheel is taken to be,
// the panel — in the browser's storage for the origin it was served from. A port
// that changes every run is a new origin every run, and every setting starts
// again from the default.
const defaultAddr = "127.0.0.1:7777"

// listen serves on addr, and falls back to a free port when the default one is
// taken. Two graphs open at once is worth more than the second one remembering
// its settings, and only the default falls back: an address asked for by name is
// one the caller means.
func listen(cmd *cobra.Command, addr string, src graph.Source, opts ...serve.Option) (*serve.Server, error) {
	srv, err := serve.Listen(addr, src, opts...)
	if err == nil || addr != defaultAddr {
		return srv, err
	}

	fmt.Fprintf(cmd.ErrOrStderr(),
		"godi: %s is taken, serving on a free port instead; this page will not have your saved settings\n", addr)
	return serve.Listen("127.0.0.1:0", src, opts...)
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
