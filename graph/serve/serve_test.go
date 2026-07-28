package serve_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	godi "github.com/michalkurzeja/godi/v2"
	"github.com/michalkurzeja/godi/v2/di"
	"github.com/michalkurzeja/godi/v2/graph"
	"github.com/michalkurzeja/godi/v2/graph/extract"
	"github.com/michalkurzeja/godi/v2/graph/serve"
	"github.com/michalkurzeja/godi/v2/graph/text"
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

// countingSource stands in for a container that is still being wired: the
// handler is meant to ask it again every time.
type countingSource struct {
	g     *graph.Graph
	calls int
}

func (s *countingSource) source() graph.Source {
	return func(graph.Config) (*graph.Graph, error) {
		s.calls++
		return s.g, nil
	}
}

func get(t *testing.T, h http.Handler, path string) *http.Response {
	t.Helper()

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec.Result()
}

func TestThePageIsServedAsHTML(t *testing.T) {
	t.Parallel()

	res := get(t, serve.Handler(graph.Static(testGraph(t))), "/")
	defer res.Body.Close()

	require.Equal(t, http.StatusOK, res.StatusCode)
	require.Equal(t, "text/html; charset=utf-8", res.Header.Get("Content-Type"))

	body, err := io.ReadAll(res.Body)
	require.NoError(t, err)
	require.Contains(t, string(body), "<!DOCTYPE html>")
}

// The point of taking a Source rather than a graph: a container that is still
// being wired says something different each time it is asked, and a page that
// asked once would be showing the past.
func TestTheGraphIsExtractedOnEveryRequest(t *testing.T) {
	t.Parallel()

	src := &countingSource{g: testGraph(t)}
	h := serve.Handler(src.source(), serve.WithEncoder(text.New()))

	for range 3 {
		get(t, h, "/").Body.Close()
	}

	require.Equal(t, 3, src.calls)
}

// Whatever reads the model, such as another tool or a future live preview, should
// not have to scrape it out of the page.
func TestTheModelIsServedAsJSON(t *testing.T) {
	t.Parallel()

	want := testGraph(t)

	res := get(t, serve.Handler(graph.Static(want)), "/graph.json")
	defer res.Body.Close()

	require.Equal(t, "application/json", res.Header.Get("Content-Type"))

	got, md, err := graph.ReadJSON(res.Body)
	require.NoError(t, err)
	require.Equal(t, graph.Schema, md.Schema)
	require.Equal(t, want.Nodes, got.Nodes)
}

func TestTheEncoderCanBeChosen(t *testing.T) {
	t.Parallel()

	res := get(t, serve.Handler(graph.Static(testGraph(t)), serve.WithEncoder(text.New())), "/")
	defer res.Body.Close()

	require.Equal(t, "text/plain; charset=utf-8", res.Header.Get("Content-Type"))

	body, err := io.ReadAll(res.Body)
	require.NoError(t, err)
	require.Contains(t, string(body), "serve_test.(*Server)")
}

// A source with nothing to describe must not be served as an empty page under a
// 200, which reads as a container with nothing in it.
func TestASourceWithNoGraphIsAnError(t *testing.T) {
	t.Parallel()

	res := get(t, serve.Handler(graph.Static(nil)), "/")
	defer res.Body.Close()

	require.Equal(t, http.StatusInternalServerError, res.StatusCode)

	body, err := io.ReadAll(res.Body)
	require.NoError(t, err)
	require.Contains(t, string(body), "no graph")
}

func TestAnythingElseIsNotFound(t *testing.T) {
	t.Parallel()

	res := get(t, serve.Handler(graph.Static(testGraph(t))), "/nope")
	defer res.Body.Close()

	require.Equal(t, http.StatusNotFound, res.StatusCode)
}

// The port is usually chosen for us, so it has to be known before anything is
// served: it is what gets printed, and what the browser is pointed at.
func TestListenBindsBeforeItServes(t *testing.T) {
	t.Parallel()

	srv, err := serve.Listen("127.0.0.1:0", graph.Static(testGraph(t)))
	require.NoError(t, err)

	require.True(t, strings.HasPrefix(srv.URL(), "http://127.0.0.1:"))
	require.NotContains(t, srv.URL(), ":0", "a real port, not the one we asked for")

	done := make(chan error, 1)
	go func() { done <- srv.Serve() }()

	res, err := http.Get(srv.URL()) //nolint:noctx // A test against our own loopback server.
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, res.StatusCode)
	res.Body.Close()

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	require.NoError(t, srv.Shutdown(ctx))

	require.NoError(t, <-done, "a server told to stop has not failed")
}
