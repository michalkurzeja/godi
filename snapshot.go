package di

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"

	"github.com/michalkurzeja/godi/v2/di"
	"github.com/michalkurzeja/godi/v2/graph"
	"github.com/michalkurzeja/godi/v2/graph/extract"
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
// what went wrong can be looked at instead of pieced together from an error
// message. The graph is as far as the container ever got, which is where whatever
// stopped it is still visible.
//
// It writes JSON and nothing else. Drawing a graph needs Graphviz or a browser,
// and no godi binary should carry either for a debugging aid it will almost never
// use. The godi CLI turns the file into something to look at:
//
//	godi view <path>
//
// Nothing here may change what Build returns. A snapshot that cannot be written
// says so on stderr and is otherwise forgotten.
func (b *Builder) reportFailedBuild(container *di.Container) {
	w := newSnapshotWriter()
	if !w.enabled {
		return
	}

	// A non-nil container means compilation finished and only preparing the
	// definitions failed. The builder has handed its container over by then and
	// has no graph left to give.
	g, err := extract.FromBuilder(b.cb)
	if container != nil {
		g, err = extract.From(container)
	}
	if err != nil {
		w.warnNoSnapshot(err)
		return
	}

	path, err := w.write(g)
	if err != nil {
		w.warnNoSnapshot(err)
		return
	}

	if g.Snapshot != nil && g.Snapshot.Failed != "" {
		w.warnf("godi: build failed at pass %q\n", g.Snapshot.Failed)
	}
	w.warnf("godi: graph written to %s\n", path)
	w.warnf("godi:   godi view %s\n", path)
}

// snapshotWriter is what the environment asked for, read once. Nothing below
// reads it again, so one failed build cannot write to two different places.
type snapshotWriter struct {
	enabled bool
	// path is a directory to put a file in, or the file itself. Empty means the
	// system's temporary directory.
	path string
	warn io.Writer
}

func newSnapshotWriter() snapshotWriter {
	on, err := strconv.ParseBool(os.Getenv(envSnapshotOnBuildErr))
	return snapshotWriter{
		enabled: err == nil && on,
		path:    os.Getenv(envSnapshotPath),
		warn:    os.Stderr,
	}
}

// write writes the graph out and returns where it went.
func (w snapshotWriter) write(g *graph.Graph) (string, error) {
	f, err := w.createFile()
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

// createFile makes the file readable only by you. A graph asked for literal
// values carries whatever those literals are, and a temporary directory is not a
// private place.
//
// The path is GODI_SNAPSHOT_PATH, set by whoever runs the program. Choosing it is
// the point of the variable, so there is no untrusted input for gosec to follow.
//
//nolint:gosec // G703: the path is the operator's own, by design.
func (w snapshotWriter) createFile() (*os.File, error) {
	path := w.path
	if path == "" {
		path = os.TempDir()
	}

	info, err := os.Stat(path)
	switch {
	case err == nil && info.IsDir():
		return os.CreateTemp(path, "godi-graph-*.json")
	case err == nil, errors.Is(err, os.ErrNotExist):
		// Either a file already there, or nothing yet: there is no other signal
		// for directory intent, so this keeps treating the path as a file.
		return os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	default:
		// A permission error or a bad path component: guessing "must be a file"
		// here only fails a second time, more confusingly, at OpenFile.
		return nil, err
	}
}

func (w snapshotWriter) warnNoSnapshot(err error) {
	w.warnf("godi: could not write the graph of the failed build: %s\n", err)
}

func (w snapshotWriter) warnf(format string, args ...any) {
	fmt.Fprintf(w.warn, format, args...)
}
