package graph_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	godi "github.com/michalkurzeja/godi/v2"
	"github.com/michalkurzeja/godi/v2/di"
	"github.com/michalkurzeja/godi/v2/graph"
	"github.com/michalkurzeja/godi/v2/graph/extract"
)

// JSON is the interchange format: the library writes it when a build fails, and
// a separately installed CLI reads it. The two are versioned apart, so what is
// pinned here is what holds them together.

type Greeter interface{ Greet() string }

type EnGreeter struct{}

func (EnGreeter) Greet() string { return "hello" }

type Store struct{}

type Server struct {
	greeter Greeter
	store   *Store
	addr    string
}

func NewEnGreeter() EnGreeter { return EnGreeter{} }
func NewStore() *Store        { return &Store{} }

func NewServer(g Greeter, s *Store, addr string) *Server {
	return &Server{greeter: g, store: s, addr: addr}
}

// builtGraph is a graph of a container that compiled, i.e. one with no snapshot.
func builtGraph(t *testing.T) *graph.Graph {
	t.Helper()

	c, err := godi.New().Services(
		godi.Svc(NewServer, "localhost:8080"),
		godi.Svc(NewEnGreeter),
		godi.Svc(NewStore),
	).Build()
	require.NoError(t, err)

	g, err := extract.From(c.(*di.Container))
	require.NoError(t, err)
	return g
}

// failedGraph is a graph of a build that stopped, which is the case the format
// exists for. The builder keeps its container when the compiler stops, which is
// what leaves anything to graph at all.
func failedGraph(t *testing.T) *graph.Graph {
	t.Helper()

	dir := t.TempDir()
	t.Setenv("GODI_SNAPSHOT_ON_BUILD_ERR", "true")
	t.Setenv("GODI_SNAPSHOT_PATH", dir)

	_, err := godi.New().Services(
		godi.Svc(NewServer, "localhost:8080"),
		godi.Svc(NewEnGreeter),
	).Build()
	require.Error(t, err)

	found, err := filepath.Glob(filepath.Join(dir, "*.json"))
	require.NoError(t, err)
	require.Len(t, found, 1, "the failed build wrote its graph")

	f, err := os.Open(found[0])
	require.NoError(t, err)
	defer f.Close()

	g, _, err := graph.ReadJSON(f)
	require.NoError(t, err)
	return g
}

func writeJSON(t *testing.T, g *graph.Graph) []byte {
	t.Helper()

	var buf bytes.Buffer
	require.NoError(t, g.WriteJSON(&buf))
	return buf.Bytes()
}

// Everything an encoder reads has to survive the trip, or a graph rendered from
// a file says something different from the same graph rendered in process.
func TestAGraphSurvivesTheRoundTrip(t *testing.T) {
	t.Parallel()

	want := builtGraph(t)

	got, _, err := graph.ReadJSON(bytes.NewReader(writeJSON(t, want)))
	require.NoError(t, err)

	require.Equal(t, want.Schema, got.Schema)
	require.Equal(t, want.SourceRoot, got.SourceRoot)
	require.Equal(t, want.Scopes, got.Scopes)
	require.Equal(t, want.Nodes, got.Nodes)
	require.Equal(t, want.Edges, got.Edges)
	require.Equal(t, want.Bindings, got.Bindings)
	require.Equal(t, want.GraphDiagnostics, got.GraphDiagnostics)
	require.Equal(t, want.WiringDiagnostics(), got.WiringDiagnostics(),
		"the wiring half recomputes from the parameters, so it survives the trip")
	require.Nil(t, got.Snapshot, "a built container has nothing to say about when it was read")
}

// A partial graph that lost its snapshot would read as a finished container with
// dependencies missing, which is the one thing the snapshot exists to prevent.
// Not parallel: getting a failed build's graph means asking godi to write one,
// and that is an environment variable.
func TestAPartialGraphKeepsItsSnapshot(t *testing.T) {
	want := failedGraph(t)
	require.Equal(t, "argument validation", want.Snapshot.Failed)

	got, _, err := graph.ReadJSON(bytes.NewReader(writeJSON(t, want)))
	require.NoError(t, err)

	require.Equal(t, want.Snapshot, got.Snapshot)
	require.True(t, got.Partial())
	require.Equal(t, "taken where the argument validation pass failed", got.Snapshot.Label())
}

// The file says what it is once. Two copies of the schema could disagree, and
// then neither would be worth reading.
func TestTheEnvelopeSaysTheSchemaOnlyOnce(t *testing.T) {
	t.Parallel()

	var file map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(writeJSON(t, builtGraph(t)), &file))

	require.Len(t, file, 2)
	require.Contains(t, file, "metadata")
	require.Contains(t, file, "graph")

	var inner map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(file["graph"], &inner))
	require.NotContains(t, inner, "schema", "the schema belongs to the file, not to the graph")

	var md graph.Metadata
	require.NoError(t, json.Unmarshal(file["metadata"], &md))
	require.Equal(t, graph.Schema, md.Schema)
}

// Which failure graph is this one? A directory of them is otherwise silent.
func TestMetadataComesBackWithTheGraph(t *testing.T) {
	t.Parallel()

	_, md, err := graph.ReadJSON(bytes.NewReader(writeJSON(t, builtGraph(t))))
	require.NoError(t, err)

	require.Equal(t, graph.Schema, md.Schema)
	require.False(t, md.WrittenAt.IsZero())
}

// The library and the CLI are installed separately and will drift. The model
// grows by adding fields, so an unrecognised schema is nearly always still worth
// reading - and a build that already failed is a bad moment to refuse.
func TestAnUnknownSchemaIsAWarningNotAFailure(t *testing.T) {
	t.Parallel()

	file := bytes.Replace(writeJSON(t, builtGraph(t)),
		[]byte(`"schema":"`+graph.Schema+`"`),
		[]byte(`"schema":"godi.graph/v99"`), 1)

	got, md, err := graph.ReadJSON(bytes.NewReader(file))
	require.NoError(t, err)

	require.NotEmpty(t, got.Nodes, "the graph is read anyway")
	require.Equal(t, "godi.graph/v99", md.Schema)
	require.Equal(t, "godi.graph/v99", got.Schema, "the graph keeps the file's account of itself")

	require.Len(t, got.GraphDiagnostics, 1)
	require.Equal(t, graph.SeverityWarning, got.GraphDiagnostics[0].Severity)
	require.Contains(t, got.GraphDiagnostics[0].Message, "godi.graph/v99")
	require.Contains(t, got.GraphDiagnostics[0].Message, graph.Schema)
}

func TestAGraphNamingNoSchemaSaysSo(t *testing.T) {
	t.Parallel()

	got, _, err := graph.ReadJSON(strings.NewReader(`{"metadata":{},"graph":{"nodes":[]}}`))
	require.NoError(t, err)

	require.Len(t, got.GraphDiagnostics, 1)
	require.Contains(t, got.GraphDiagnostics[0].Message, "names no schema")
}

// The lookup indexes are not serialised, so a decoded graph has to build them
// on first use like any other.
func TestADecodedGraphAnswersTheSameQuestions(t *testing.T) {
	t.Parallel()

	want := builtGraph(t)

	got, _, err := graph.ReadJSON(bytes.NewReader(writeJSON(t, want)))
	require.NoError(t, err)

	for _, node := range want.Nodes {
		found, ok := got.Node(node.ID)
		require.True(t, ok, "node %s", node.ID)
		require.Equal(t, node, found)

		require.Equal(t, want.OutEdges(node.ID), got.OutEdges(node.ID))
		require.Equal(t, want.InEdges(node.ID), got.InEdges(node.ID))
	}

	for _, scope := range want.Scopes {
		require.Equal(t, want.ScopeNodes(scope.ID), got.ScopeNodes(scope.ID))
	}
}

// graph.Source is otherwise satisfiable only by godi's own types, which leaves
// anyone holding a graph read from a file unable to use what takes one.
func TestStaticPresentsAGraphYouAlreadyHaveAsASource(t *testing.T) {
	t.Parallel()

	want := builtGraph(t)

	got, err := graph.Extract(graph.Static(want), graph.WithoutLiterals())
	require.NoError(t, err)

	require.Same(t, want, got, "extraction already happened; the config has nothing left to say")
}

func TestTheEncoderWritesWhatReadJSONReads(t *testing.T) {
	t.Parallel()

	want := builtGraph(t)

	var buf bytes.Buffer
	require.NoError(t, want.Encode(&buf, graph.JSON(graph.Indent("  "))))
	require.Contains(t, buf.String(), "\n  \"metadata\": {", "the indent reaches the output")

	got, _, err := graph.ReadJSON(&buf)
	require.NoError(t, err)
	require.Equal(t, want.Nodes, got.Nodes)
}

func TestTheJSONEncoderNamesItsFormat(t *testing.T) {
	t.Parallel()

	require.Equal(t,
		graph.Format{Name: "json", Ext: "json", MediaType: "application/json"},
		graph.JSON().Format())
}

func TestReadJSONRejectsWhatItCannotUse(t *testing.T) {
	t.Parallel()

	_, _, err := graph.ReadJSON(strings.NewReader(`not json`))
	require.ErrorContains(t, err, "graph: reading json")

	_, _, err = graph.ReadJSON(strings.NewReader(`{"metadata":{"schema":"godi.graph/v1"}}`))
	require.ErrorContains(t, err, "graph: the file carries no graph")
}

func TestWritingANilGraphFails(t *testing.T) {
	t.Parallel()

	var g *graph.Graph
	require.ErrorContains(t, g.WriteJSON(&bytes.Buffer{}), "graph: cannot write a nil graph")
}
