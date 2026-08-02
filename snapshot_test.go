package di_test

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	godi "github.com/michalkurzeja/godi/v2"
	"github.com/michalkurzeja/godi/v2/graph"
)

// A build that failed is the one worth graphing, and reaching for the graph by
// hand means writing Go at the moment you least want to. These tests pin the
// code-free path: an environment variable, and a file to hand to the CLI.
//
// None of them can run in parallel: they set the environment.

// failingBuilder wires a Server without the *Store it needs, so the argument
// validation pass stops the build.
func failingBuilder() *godi.Builder {
	return godi.New().Services(
		godi.Svc(NewServer, "localhost:8080"),
		godi.Svc(NewEnGreeter),
	)
}

// onlyPrepareFails registers a service whose args cannot be slotted, which goes
// wrong before compilation rather than during it.
func onlyPrepareFails() *godi.Builder {
	return godi.New().Services(godi.Svc(NewStore, "unslottable").NotAutowired())
}

func snapshotIn(t *testing.T, dir string) *graph.Graph {
	t.Helper()

	found, err := filepath.Glob(filepath.Join(dir, "*.json"))
	require.NoError(t, err)
	require.Len(t, found, 1, "expected exactly one graph in %s", dir)

	return readSnapshot(t, found[0])
}

func readSnapshot(t *testing.T, path string) *graph.Graph {
	t.Helper()

	f, err := os.Open(path)
	require.NoError(t, err)
	defer f.Close()

	g, md, err := graph.ReadJSON(f)
	require.NoError(t, err)
	require.Equal(t, graph.Schema, md.Schema)
	return g
}

// captureStderr collects what is written to os.Stderr while fn runs. The
// snapshot reports itself there, and where the file went is the whole point.
func captureStderr(t *testing.T, fn func()) string {
	t.Helper()

	r, w, err := os.Pipe()
	require.NoError(t, err)

	orig := os.Stderr
	os.Stderr = w
	defer func() { os.Stderr = orig }()

	fn()

	require.NoError(t, w.Close())
	out, err := io.ReadAll(r)
	require.NoError(t, err)
	return string(out)
}

func TestAFailedBuildWritesItsGraphWhenAsked(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("GODI_SNAPSHOT_ON_BUILD_ERR", "true")
	t.Setenv("GODI_SNAPSHOT_PATH", dir)

	_, err := failingBuilder().Build()
	require.Error(t, err)

	g := snapshotIn(t, dir)

	require.Equal(t, "argument validation", g.Snapshot.Failed,
		"the graph says where the compiler stopped, which is where the fault is")
	require.NotEmpty(t, g.Nodes)
}

// The graph is a debugging aid nobody asked for until they did. Writing one
// unbidden would litter every temporary directory godi ever runs in.
func TestNothingIsWrittenUnlessAsked(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("GODI_SNAPSHOT_PATH", dir)

	for _, value := range []string{"", "false", "0", "nonsense"} {
		t.Setenv("GODI_SNAPSHOT_ON_BUILD_ERR", value)

		_, err := failingBuilder().Build()
		require.Error(t, err)

		found, globErr := filepath.Glob(filepath.Join(dir, "*.json"))
		require.NoError(t, globErr)
		require.Empty(t, found, "GODI_SNAPSHOT_ON_BUILD_ERR=%q wrote a graph", value)
	}
}

// Whatever the snapshot does, the error the caller gets back has to be the error
// they would have got anyway.
func TestTheBuildErrorIsUntouched(t *testing.T) {
	_, want := failingBuilder().Build()
	require.Error(t, want)

	t.Setenv("GODI_SNAPSHOT_ON_BUILD_ERR", "true")
	t.Setenv("GODI_SNAPSHOT_PATH", t.TempDir())

	_, got := failingBuilder().Build()
	require.EqualError(t, got, want.Error())
}

func TestTheSnapshotPathCanNameTheFileItself(t *testing.T) {
	path := filepath.Join(t.TempDir(), "failure.json")
	t.Setenv("GODI_SNAPSHOT_ON_BUILD_ERR", "true")
	t.Setenv("GODI_SNAPSHOT_PATH", path)

	_, err := failingBuilder().Build()
	require.Error(t, err)

	require.Equal(t, "argument validation", readSnapshot(t, path).Snapshot.Failed)
}

// A snapshot that cannot be written is a nuisance. A snapshot that swallows the
// build error while failing is a bug hunt.
func TestAnUnwritableSnapshotPathDoesNotHideTheBuildError(t *testing.T) {
	t.Setenv("GODI_SNAPSHOT_ON_BUILD_ERR", "true")
	t.Setenv("GODI_SNAPSHOT_PATH", filepath.Join(t.TempDir(), "no-such-dir", "graph.json"))

	var err error
	stderr := captureStderr(t, func() { _, err = failingBuilder().Build() })

	require.ErrorContains(t, err, "compiler pass (argument validation) returned an error")
	require.Contains(t, stderr, "godi: could not write the graph of the failed build")
}

// Preparing the definitions can fail while compilation goes on to succeed, and
// then the builder has handed its container over and has no graph left to give.
// The graph has to come from the container instead.
func TestAGraphIsStillWrittenWhenOnlyPreparingFailed(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("GODI_SNAPSHOT_ON_BUILD_ERR", "true")
	t.Setenv("GODI_SNAPSHOT_PATH", dir)

	_, err := onlyPrepareFails().Build()
	require.ErrorContains(t, err, "cannot be slotted to function")

	require.NotNil(t, snapshotIn(t, dir), "a graph, even though the builder had none to give")
}

// The path is the only way back to the file, and the command is the only thing
// the reader has to know to use it.
func TestTheSnapshotSaysWhereItWentAndWhatToRun(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("GODI_SNAPSHOT_ON_BUILD_ERR", "true")
	t.Setenv("GODI_SNAPSHOT_PATH", dir)

	stderr := captureStderr(t, func() {
		_, err := failingBuilder().Build()
		require.Error(t, err)
	})

	found, err := filepath.Glob(filepath.Join(dir, "*.json"))
	require.NoError(t, err)
	require.Len(t, found, 1)

	require.Contains(t, stderr, `godi: build failed at pass "argument validation"`)
	require.Contains(t, stderr, "godi: graph written to "+found[0])
	require.Contains(t, stderr, "godi:   godi view "+found[0])
}
