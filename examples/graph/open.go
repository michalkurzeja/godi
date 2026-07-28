package main

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"

	"github.com/michalkurzeja/godi/v2/graph"
)

// -open writes the graph out and hands it to the desktop.
//
// A browser cannot be handed bytes, so the page goes to a file first. The file is
// readable only by you, because a graph asked for literal values carries whatever
// those literals are. The operating system clears it up in its own time.
//
// It lives in the example rather than in godi. A program that draws a graph may
// start a browser; a library nobody asked to do that may not.
func openGraphFile(g *graph.Graph, enc graph.Encoder) (string, error) {
	path, err := writeToTmpFile(g, enc)
	if err != nil {
		return "", err
	}
	return path, launch(path)
}

func writeToTmpFile(g *graph.Graph, enc graph.Encoder) (string, error) {
	f, err := os.CreateTemp("", "godi-graph-*."+enc.Format().Ext)
	if err != nil {
		return "", fmt.Errorf("creating a file for the graph: %w", err)
	}

	err = g.Encode(f, enc)
	if err != nil {
		f.Close()
		os.Remove(f.Name()) // Half a graph is no use to anyone.
		return "", err
	}

	err = f.Close()
	if err != nil {
		return "", fmt.Errorf("writing %s: %w", f.Name(), err)
	}
	return f.Name(), nil
}

func launch(path string) error {
	name, args := opener(path)
	if name == "" {
		return fmt.Errorf("no way to open a file on %s", runtime.GOOS)
	}

	bin, err := exec.LookPath(name)
	if err != nil {
		return fmt.Errorf("looking for %s: %w", name, err)
	}

	//nolint:gosec // G204: the binary is resolved from PATH and the path is ours.
	cmd := exec.Command(bin, args...)
	err = cmd.Start()
	if err != nil {
		return fmt.Errorf("running %s: %w", name, err)
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
