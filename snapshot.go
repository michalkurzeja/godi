package di

import (
	"fmt"
	"os"
	"strconv"

	"github.com/michalkurzeja/godi/v2/di"
	"github.com/michalkurzeja/godi/v2/graph"
)

// What godi reads from the environment when a build fails. Both are off, and
// nothing is written, unless the first one says otherwise.
const (
	// envSnapshotOnBuildErr turns the failure snapshot on. Anything
	// strconv.ParseBool accepts as true will do.
	envSnapshotOnBuildErr = "GODI_SNAPSHOT_ON_BUILD_ERR"
	// envSnapshotPath is where to write it: a directory to put a file in, or
	// the file itself. Defaults to the system's temporary directory.
	envSnapshotPath = "GODI_SNAPSHOT_PATH"
)

// reportFailedBuild writes the graph of a container that did not build, so that
// what went wrong can be looked at rather than pieced together from an error
// message. The graph of a failed build is as far as the container ever got,
// which is where whatever stopped it is still visible.
//
// It writes JSON and nothing else. Drawing a graph means Graphviz and a browser,
// and no godi binary should carry those for the sake of a debugging aid it will
// almost never use. The godi CLI turns the file into something to look at:
//
//	godi view <path>
//
// Nothing here may change what Build returns. A snapshot that cannot be written
// says so and is otherwise forgotten: the build error is the one that matters.
func (b *Builder) reportFailedBuild(container *di.Container) {
	if !snapshotOnBuildErr() {
		return
	}

	// A non-nil container means compilation finished and only preparing the
	// definitions failed. The builder has handed its container over by then and
	// has no graph left to give.
	src := graph.Source(b)
	if container != nil {
		src = container
	}

	g, err := graph.Extract(src)
	if err != nil {
		warnNoSnapshot(err)
		return
	}

	path, err := writeSnapshot(g)
	if err != nil {
		warnNoSnapshot(err)
		return
	}

	if g.Snapshot != nil && g.Snapshot.Failed != "" {
		fmt.Fprintf(os.Stderr, "godi: build failed at pass %q\n", g.Snapshot.Failed)
	}
	fmt.Fprintf(os.Stderr, "godi: graph written to %s\n", path)
	fmt.Fprintf(os.Stderr, "godi:   godi view %s\n", path)
}

func snapshotOnBuildErr() bool {
	on, err := strconv.ParseBool(os.Getenv(envSnapshotOnBuildErr))
	return err == nil && on
}

// writeSnapshot writes the graph out and returns where it went.
func writeSnapshot(g *graph.Graph) (string, error) {
	f, err := createSnapshotFile()
	if err != nil {
		return "", err
	}

	err = g.WriteJSON(f)
	if err != nil {
		f.Close()
		os.Remove(f.Name()) // Half a graph is no use to anyone.
		return "", err
	}

	err = f.Close()
	if err != nil {
		return "", err
	}
	return f.Name(), nil
}

// createSnapshotFile makes the file private to you: a graph asked for literal
// values carries whatever those literals are, and a temporary directory is not
// a private place.
// The path is GODI_SNAPSHOT_PATH, set by whoever runs the program to say where
// they want the graph written. Choosing it is the point of the variable, so
// there is no untrusted end to the taint gosec follows.
//
//nolint:gosec // G703: the path is the operator's own, by design.
func createSnapshotFile() (*os.File, error) {
	path := os.Getenv(envSnapshotPath)
	if path == "" {
		path = os.TempDir()
	}

	if info, err := os.Stat(path); err == nil && info.IsDir() {
		return os.CreateTemp(path, "godi-graph-*.json")
	}
	return os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
}

func warnNoSnapshot(err error) {
	fmt.Fprintf(os.Stderr, "godi: could not write the graph of the failed build: %s\n", err)
}
