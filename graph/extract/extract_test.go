package extract_test

import (
	"errors"
	"reflect"
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

	first, err := src.Graph(graph.NewConfig())
	require.NoError(t, err)
	second, err := src.Graph(graph.NewConfig())
	require.NoError(t, err)

	require.NotSame(t, first, second)
	require.Equal(t, first.Nodes, second.Nodes)
}

type CA struct{ b *CB }

func NewCA(b *CB) *CA { return &CA{b: b} }

type CB struct{ a *CA }

func NewCB() *CB { return &CB{} }

func (b *CB) SetA(a *CA) { b.a = a }

// A pass diagnostic naming an argument replaces extraction's own guess about
// it, on the theory that the pass saw more. That only holds when the pass
// diagnostic is itself an error: an unrelated info-severity note on the same
// slot must not erase the real fault extraction already recorded there.
func TestANonErrorPassDiagnosticDoesNotEraseAnExtractionFault(t *testing.T) {
	t.Parallel()

	b := di.NewContainerBuilder(di.NewConfig())

	// *CB is never registered, so this argument traces to ArgFaultNoServicesOfType.
	arg := di.NewTypeArg(reflect.TypeFor[*CB](), false)
	f, err := di.NewFactory(NewCA, arg)
	require.NoError(t, err)

	def := di.NewServiceDefinition(f).SetScope(b.RootScope())
	b.RootScope().AddServiceDefinitions(def)

	// The real argument-validation pass runs at the Validation stage and would
	// itself report an error for this slot, masking the bug: it would replace
	// extraction's guess with an equivalent error rather than an info note. So a
	// pass at PreValidation reports the info note, and a second one right after
	// it stops compilation before Validation ever runs — reproducing the "build
	// stopped early" case CLAUDE.md calls the main use of a partial graph.
	b.Compiler().AddPass(di.NewCompilerPass("note", di.PreValidation, di.CompilerOpFunc(func(cb *di.ContainerBuilder) error {
		slot := def.Factory().Args().Slots()[0]
		cb.Report(di.Diagnostic{Severity: di.SeverityInfo, Site: di.AtServiceArg(def, slot), Message: "unrelated note"})
		return nil
	})))
	b.Compiler().AddPass(di.NewCompilerPass("stop", di.PreValidation, di.CompilerOpFunc(func(*di.ContainerBuilder) error {
		return errors.New("no further")
	})))

	_, err = b.Build()
	require.Error(t, err)

	g, err := extract.FromBuilder(b)
	require.NoError(t, err)
	require.True(t, g.Partial())

	require.Len(t, g.Nodes, 1)
	require.Len(t, g.Nodes[0].Params, 1)
	p := g.Nodes[0].Params[0]

	require.True(t, p.Faulty(), "an argument with no matching service must still read as faulty")

	var hasInfo, hasError bool
	for _, d := range p.Diagnostics {
		hasInfo = hasInfo || d.Severity == graph.SeverityInfo
		hasError = hasError || d.Severity == graph.SeverityError
	}
	require.True(t, hasInfo, "the pass's own note must still be recorded")
	require.True(t, hasError, "the unresolved-argument error must survive an unrelated info diagnostic on the same slot")
}

// A method call runs after its receiver is built, so a loop closed only
// through a method-call argument is legal setter injection, not an
// instantiation cycle — the container builds it with no error. The graph must
// not paint that edge as a cycle just because the underlying algorithm cannot
// tell the two apart without help.
func TestAMethodCallLoopIsNotMarkedAsACycle(t *testing.T) {
	t.Parallel()

	c, err := godi.New().Services(
		godi.Svc(NewCA),
		godi.Svc(NewCB).MethodCall((*CB).SetA),
	).Build()
	require.NoError(t, err)

	engine, ok := c.(*di.Container)
	require.True(t, ok)

	g, err := extract.From(engine)
	require.NoError(t, err)

	for _, e := range g.Edges {
		require.False(t, e.Cycle, "edge %s -> %s (%s) must not be marked as a cycle", e.From, e.To, e.Kind)
	}
}
