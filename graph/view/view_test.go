package view_test

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/michalkurzeja/godi/v2/graph"
	"github.com/michalkurzeja/godi/v2/graph/view"
)

// fake is an encoder with no dependencies of its own, so these tests exercise
// the file handling rather than any particular format.
type fake struct {
	ext  string
	body string
	err  error
}

func (f fake) Format() graph.Format { return graph.Format{Name: f.ext, Ext: f.ext} }

func (f fake) Encode(_ *graph.Graph, w io.Writer) error {
	if f.err != nil {
		return f.err
	}
	_, err := io.WriteString(w, f.body)
	return err
}

func model() *graph.Graph {
	return &graph.Graph{Schema: graph.Schema, Scopes: []*graph.Scope{{ID: "root", Name: "root"}}}
}

func TestWriteNamesTheFileAfterTheFormat(t *testing.T) {
	path, err := view.WriteToTmpFile(model(), fake{ext: "html", body: "<!DOCTYPE html>"})
	require.NoError(t, err)
	t.Cleanup(func() { os.Remove(path) })

	require.Equal(t, ".html", filepath.Ext(path))
	require.True(t, strings.HasPrefix(filepath.Base(path), "godi-graph-"))
	require.Equal(t, filepath.Clean(os.TempDir()), filepath.Dir(path), "a temporary file, not one in the way")

	body, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, "<!DOCTYPE html>", string(body))
}

// A graph asked for literal values carries whatever those literals are, so the
// file it lands in must not be readable by anyone else on the machine.
func TestTheFileIsPrivate(t *testing.T) {
	path, err := view.WriteToTmpFile(model(), fake{ext: "dot", body: "digraph {}"})
	require.NoError(t, err)
	t.Cleanup(func() { os.Remove(path) })

	info, err := os.Stat(path)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o600), info.Mode().Perm())
}

// Half a graph is worse than none: it would open, and look like the whole
// thing.
func TestAFailedEncodeLeavesNoFileBehind(t *testing.T) {
	before := tempFiles(t)

	_, err := view.WriteToTmpFile(model(), fake{ext: "html", err: errors.New("ran out of ink")})
	require.ErrorContains(t, err, "ran out of ink")

	require.Equal(t, before, tempFiles(t), "a file survived a failed encode")
}

func TestOpenReportsAnExtractionFailure(t *testing.T) {
	_, err := view.Open(nil, fake{ext: "html"})

	require.ErrorContains(t, err, "nil source")
}

// OpenGraph is what Open becomes once the graph has been narrowed, so it has to
// fail the same way: nothing written, nothing launched.
func TestOpenGraphReportsAnEncodeFailure(t *testing.T) {
	before := tempFiles(t)

	_, err := view.OpenGraph(model(), fake{ext: "html", err: errors.New("ran out of ink")})

	require.ErrorContains(t, err, "ran out of ink")
	require.Equal(t, before, tempFiles(t))
}

func TestLaunchReportsAMissingOpener(t *testing.T) {
	t.Setenv("PATH", t.TempDir()) // Nothing to find.

	err := view.Launch(filepath.Join(t.TempDir(), "graph.html"))

	require.Error(t, err)
	require.Contains(t, err.Error(), "looking for")
}

func tempFiles(t *testing.T) []string {
	t.Helper()

	names, err := filepath.Glob(filepath.Join(os.TempDir(), "godi-graph-*"))
	require.NoError(t, err)
	return names
}
