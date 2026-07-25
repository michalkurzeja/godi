// Package view opens a dependency graph in whatever the operating system uses
// for the format: a browser tab for the HTML viewer, an editor or a previewer
// for anything else.
//
// It lives apart from graph because the root package's Container names
// graph.Config, so every godi binary carries the model whether or not it ever
// draws a graph. Nothing in that path should be able to run other programs.
// Importing this package is how you ask for that.
//
//	path, err := view.Open(container, html.New())
//
// A browser cannot be handed bytes, so the page goes to a temporary file first,
// created private to you because a graph can carry literals. The operating
// system clears it up in its own time.
package view

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"

	"github.com/michalkurzeja/godi/v2/graph"
	"github.com/michalkurzeja/godi/v2/internal/errorsx"
)

// Open extracts the graph, writes it out and opens it. It returns the path,
// which is worth printing: the reader may want to keep it, and it is the only
// way back to the file once the tab is closed.
func Open(src graph.Source, enc graph.Encoder, opts ...graph.Option) (string, error) {
	g, err := graph.Extract(src, opts...)
	if err != nil {
		return "", err
	}

	path, err := WriteToTmpFile(g, enc)
	if err != nil {
		return "", err
	}
	return path, Launch(path)
}

// WriteToTmpFile encodes the graph to a temporary file named after the format, and
// returns its path. The file is readable only by you: a graph asked for literal
// values carries whatever those literals are.
func WriteToTmpFile(g *graph.Graph, enc graph.Encoder) (string, error) {
	f, err := os.CreateTemp("", "godi-graph-*."+enc.Format().Ext)
	if err != nil {
		return "", errorsx.Wrap(err, "creating a file for the graph")
	}

	err = g.Encode(f, enc)
	if err != nil {
		f.Close()
		os.Remove(f.Name()) // Half a graph is no use to anyone.
		return "", err
	}

	err = f.Close()
	if err != nil {
		return "", errorsx.Wrapf(err, "writing %s", f.Name())
	}
	return f.Name(), nil
}

// Launch hands a path to the operating system to open however it sees fit.
//
// It waits only for the command to start, not for whatever it opened to close:
// on some desktops the opener does not return until the browser does, and a
// graph is something you look at while carrying on.
func Launch(path string) error {
	name, args := opener(path)
	if name == "" {
		return fmt.Errorf("view: no way to open a file on %s", runtime.GOOS)
	}

	bin, err := exec.LookPath(name)
	if err != nil {
		return errorsx.Wrapf(err, "looking for %s", name)
	}

	//nolint:gosec // G204: the binary is resolved from PATH and the path is ours.
	cmd := exec.Command(bin, args...)
	err = cmd.Start()
	if err != nil {
		return errorsx.Wrapf(err, "running %s", name)
	}

	go func() { _ = cmd.Wait() }() // Reap it; nobody is waiting on the result.
	return nil
}

func opener(path string) (name string, args []string) {
	switch runtime.GOOS {
	case "darwin":
		return "open", []string{path}
	case "windows":
		// The empty string is start's title argument, which it insists on.
		return "cmd", []string{"/c", "start", "", path}
	case "linux", "freebsd", "netbsd", "openbsd", "dragonfly", "solaris", "illumos":
		return "xdg-open", []string{path}
	default:
		return "", nil
	}
}
