package extract_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	godi "github.com/michalkurzeja/godi/v2"
	"github.com/michalkurzeja/godi/v2/di"
	"github.com/michalkurzeja/godi/v2/graph"
	"github.com/michalkurzeja/godi/v2/graph/extract"
)

type Store struct{}

func NewStore() *Store { return &Store{} }

type Server struct{ store *Store }

func NewServer(s *Store) *Server { return &Server{store: s} }

func container(t *testing.T) *di.Container {
	t.Helper()

	c, err := godi.New().Services(godi.Svc(NewServer), godi.Svc(NewStore)).Build()
	require.NoError(t, err)

	container, ok := c.(*di.Container)
	require.True(t, ok)
	return container
}

func TestFromReadsABuiltContainer(t *testing.T) {
	t.Parallel()

	g, err := extract.From(container(t))
	require.NoError(t, err)

	require.Len(t, g.Nodes, 2)
	require.Len(t, g.Edges, 1)
	require.False(t, g.Partial(), "a built container is not a snapshot")
}

func TestFromRejectsNothingToRead(t *testing.T) {
	t.Parallel()

	_, err := extract.From(nil)
	require.ErrorContains(t, err, "no container")

	_, err = extract.FromBuilder(nil)
	require.ErrorContains(t, err, "no builder")
}

// A builder that has been built has handed its container over, so there is
// nothing left to read. Saying so beats an empty graph, which is what an
// unprepared builder and a spent one would otherwise both look like.
func TestFromBuilderSaysWhenTheBuilderHasBeenSpent(t *testing.T) {
	t.Parallel()

	b := di.NewContainerBuilder(di.NewConfig())
	_, err := b.Build()
	require.NoError(t, err)

	_, err = extract.FromBuilder(b)
	require.ErrorContains(t, err, "already been built; extract from the container it returned")
}

// The builder of a build that stopped is the one worth reading: the graph shows
// exactly how far the compiler got.
func TestFromBuilderReadsABuildThatStopped(t *testing.T) {
	t.Parallel()

	b := di.NewContainerBuilder(di.NewConfig())
	b.Compiler().AddPass(di.NewCompilerPass("stop", di.PreValidation, di.CompilerOpFunc(func(*di.ContainerBuilder) error {
		return errors.New("no further")
	})))

	f, err := di.NewFactory(NewStore)
	require.NoError(t, err)
	b.RootScope().AddServiceDefinitions(di.NewServiceDefinition(f).SetScope(b.RootScope()))

	_, err = b.Build()
	require.Error(t, err)

	g, err := extract.FromBuilder(b)
	require.NoError(t, err)

	require.True(t, g.Partial())
	require.Equal(t, "stop", g.Snapshot.Failed)
	require.Len(t, g.Nodes, 1)
}

// Live is what serves a container over HTTP: the wiring can change under it, so
// each request sees what is there now.
func TestLiveReadsAgainEveryTime(t *testing.T) {
	t.Parallel()

	src := extract.Live(container(t))

	first, err := src(graph.NewConfig())
	require.NoError(t, err)
	second, err := src(graph.NewConfig())
	require.NoError(t, err)

	require.NotSame(t, first, second)
	require.Equal(t, first.Nodes, second.Nodes)
}
