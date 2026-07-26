package json_test

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"

	godi "github.com/michalkurzeja/godi/v2"
	"github.com/michalkurzeja/godi/v2/di"
	"github.com/michalkurzeja/godi/v2/graph"
	"github.com/michalkurzeja/godi/v2/graph/extract"
	"github.com/michalkurzeja/godi/v2/graph/json"
)

type Store struct{}

func NewStore() *Store { return &Store{} }

type Server struct{ store *Store }

func NewServer(s *Store) *Server { return &Server{store: s} }

func testGraph(t *testing.T) *graph.Graph {
	t.Helper()

	c, err := godi.New().Services(godi.Svc(NewServer), godi.Svc(NewStore)).Build()
	require.NoError(t, err)

	g, err := extract.From(c.(*di.Container))
	require.NoError(t, err)
	return g
}

// The encoder writes the interchange format, which the model reads back: one
// file, whichever way it was written.
func TestTheEncoderWritesWhatReadJSONReads(t *testing.T) {
	t.Parallel()

	want := testGraph(t)

	var buf bytes.Buffer
	require.NoError(t, want.Encode(&buf, json.New(json.Indent("  "))))
	require.Contains(t, buf.String(), "\n  \"metadata\": {", "the indent reaches the output")

	got, _, err := graph.ReadJSON(&buf)
	require.NoError(t, err)
	require.Equal(t, want.Nodes, got.Nodes)
}

func TestTheEncoderNamesItsFormat(t *testing.T) {
	t.Parallel()

	require.Equal(t,
		graph.Format{Name: "json", Ext: "json", MediaType: "application/json"},
		json.New().Format())
}
