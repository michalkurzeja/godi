package di_test

import (
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	godi "github.com/michalkurzeja/godi/v2"
	"github.com/michalkurzeja/godi/v2/di"
	"github.com/michalkurzeja/godi/v2/extras"
	"github.com/michalkurzeja/godi/v2/graph"
)

// The wiring fixtures. Kept deliberately small: each test builds the container
// that exercises exactly the case it is pinning.

type Greeter interface{ Greet() string }

type EnGreeter struct{}

func (EnGreeter) Greet() string { return "hello" }

type PlGreeter struct{}

func (PlGreeter) Greet() string { return "czesc" }

type Store struct{}

type Server struct {
	greeter Greeter
	store   *Store
	addr    string
}

func NewEnGreeter() EnGreeter { return EnGreeter{} }
func NewPlGreeter() PlGreeter { return PlGreeter{} }
func NewStore() *Store        { return &Store{} }

func NewServer(g Greeter, s *Store, addr string) *Server {
	return &Server{greeter: g, store: s, addr: addr}
}

func (s *Server) SetAddr(addr string) { s.addr = addr }

type Collector struct{ greeters []Greeter }

func NewCollector(greeters ...Greeter) *Collector { return &Collector{greeters: greeters} }

func extract(t *testing.T, src graph.Source, opts ...graph.Option) *graph.Graph {
	t.Helper()

	g, err := graph.Extract(src, opts...)
	require.NoError(t, err)
	return g
}

// paramOf returns the single param of the node with the given type suffix at the
// given slot index.
func paramOf(t *testing.T, g *graph.Graph, nodeType string, index int) *graph.Param {
	t.Helper()

	node := nodeOf(t, g, nodeType)
	for _, p := range node.Params {
		if p.Index == index {
			return p
		}
	}
	t.Fatalf("node %s has no param at index %d", node.ID, index)
	return nil
}

func nodeOf(t *testing.T, g *graph.Graph, typeShort string) *graph.Node {
	t.Helper()

	for _, n := range g.Nodes {
		if n.TypeShort() == typeShort {
			return n
		}
	}
	t.Fatalf("no node of type %s in graph (have %s)", typeShort, nodeTypes(g))
	return nil
}

func nodeTypes(g *graph.Graph) []string {
	out := make([]string, 0, len(g.Nodes))
	for _, n := range g.Nodes {
		out = append(out, n.TypeShort())
	}
	return out
}

func edgesOf(g *graph.Graph, p *graph.Param) []*graph.Edge {
	var out []*graph.Edge
	for _, e := range g.Edges {
		if e.Param == p.ID {
			out = append(out, e)
		}
	}
	return out
}

func TestGraphProvenance(t *testing.T) {
	t.Parallel()

	t.Run("an argument the user wrote is manual", func(t *testing.T) {
		t.Parallel()

		c, err := godi.New().Services(
			godi.Svc(NewServer, "localhost:8080"),
			godi.Svc(NewEnGreeter),
			godi.Svc(NewStore),
		).Build()
		require.NoError(t, err)

		p := paramOf(t, extract(t, c), "v2_test.(*Server)", 2)
		require.Equal(t, graph.ArgOriginManual, p.Origin)
		require.Empty(t, p.OriginPass)
		require.Equal(t, graph.ArgKindLiteral, p.Arg)
	})

	// The case no heuristic could get right: a hand-written godi.Type[T]() on a
	// slot of exactly type T is byte-identical to what autowiring produces.
	t.Run("a hand-written type argument is manual, not autowired", func(t *testing.T) {
		t.Parallel()

		c, err := godi.New().Services(
			godi.Svc(NewServer, "localhost:8080", godi.Type[*Store]()),
			godi.Svc(NewEnGreeter),
			godi.Svc(NewStore),
		).Build()
		require.NoError(t, err)

		p := paramOf(t, extract(t, c), "v2_test.(*Server)", 1)
		require.Equal(t, graph.ArgOriginManual, p.Origin)
		require.Equal(t, graph.ArgKindType, p.Arg)
	})

	t.Run("an omitted argument is autowired by type", func(t *testing.T) {
		t.Parallel()

		c, err := godi.New().Services(
			godi.Svc(NewServer, "localhost:8080"),
			godi.Svc(NewEnGreeter),
			godi.Svc(NewStore),
		).Build()
		require.NoError(t, err)

		g := extract(t, c)
		p := paramOf(t, g, "v2_test.(*Server)", 1)
		require.Equal(t, graph.ArgOriginAutowiring, p.Origin)
		require.Equal(t, "autowiring", p.OriginPass)

		edges := edgesOf(g, p)
		require.Len(t, edges, 1)
		require.Equal(t, graph.ResolutionByType, edges[0].Resolution)
		require.Empty(t, edges[0].Bindings, "a concrete type needs no binding")
	})

	t.Run("an interface godi bound itself is autowired via an autobinding", func(t *testing.T) {
		t.Parallel()

		c, err := godi.New().Services(
			godi.Svc(NewServer, "localhost:8080"),
			godi.Svc(NewEnGreeter),
			godi.Svc(NewStore),
		).Build()
		require.NoError(t, err)

		g := extract(t, c)
		p := paramOf(t, g, "v2_test.(*Server)", 0)
		require.Equal(t, graph.ArgOriginAutowiring, p.Origin)

		edges := edgesOf(g, p)
		require.Len(t, edges, 1)

		hop, ok := edges[0].Binding()
		require.True(t, ok, "resolving an interface goes through a binding")
		require.Equal(t, graph.BindOriginAutobinding, hop.Origin)
		require.Equal(t, "interface binding", hop.OriginPass)
		require.Equal(t, "github.com/michalkurzeja/godi/v2_test.Greeter", hop.Interface)
	})

	t.Run("a binding the user declared is manual", func(t *testing.T) {
		t.Parallel()

		c, err := godi.New().
			Services(
				godi.Svc(NewServer, "localhost:8080"),
				godi.Svc(NewEnGreeter),
				godi.Svc(NewPlGreeter),
				godi.Svc(NewStore),
			).
			Bindings(godi.BindType[Greeter, EnGreeter]()).
			Build()
		require.NoError(t, err)

		g := extract(t, c)
		p := paramOf(t, g, "v2_test.(*Server)", 0)
		require.Equal(t, graph.ArgOriginAutowiring, p.Origin, "the argument itself was still autowired")

		hop, ok := edgesOf(g, p)[0].Binding()
		require.True(t, ok)
		require.Equal(t, graph.BindOriginManual, hop.Origin, "the user declared this binding")
		require.Empty(t, hop.OriginPass)
	})

	t.Run("a compiler pass is credited by name", func(t *testing.T) {
		t.Parallel()

		var ref godi.SvcReference
		c, err := godi.New().
			Services(
				godi.Svc(NewServer, "localhost:8080").Bind(&ref),
				godi.Svc(NewEnGreeter),
				godi.Svc(NewStore),
			).
			CompilerPasses(extras.OverrideSvcArg(ref, 2, "0.0.0.0:9090")).
			Build()
		require.NoError(t, err)

		p := paramOf(t, extract(t, c), "v2_test.(*Server)", 2)
		require.Equal(t, graph.ArgOriginCompilerPass, p.Origin)
		require.Equal(t, "override arg", p.OriginPass,
			"an overridden argument must not read as something the user typed")
	})

	// The invariant that lets each built-in pass claim one origin and leave the
	// other alone: the interface binding pass only creates bindings.
	t.Run("the interface binding pass fills no slots", func(t *testing.T) {
		t.Parallel()

		c, err := godi.New().Services(
			godi.Svc(NewServer, "localhost:8080"),
			godi.Svc(NewEnGreeter),
			godi.Svc(NewStore),
		).Build()
		require.NoError(t, err)

		g := extract(t, c)
		for _, n := range g.Nodes {
			for _, p := range n.Params {
				require.NotEqual(t, graph.ArgOriginCompilerPass, p.Origin,
					"no base pass other than autowiring should fill a slot")
			}
		}
	})
}

func TestGraphSliceSemantics(t *testing.T) {
	t.Parallel()

	t.Run("a variadic interface collects every implementation in order", func(t *testing.T) {
		t.Parallel()

		c, err := godi.New().Services(
			godi.Svc(NewCollector),
			godi.Svc(NewEnGreeter),
			godi.Svc(NewPlGreeter),
		).Build()
		require.NoError(t, err)

		g := extract(t, c)
		p := paramOf(t, g, "v2_test.(*Collector)", 0)
		require.True(t, p.Slice)
		require.True(t, p.Variadic)

		edges := edgesOf(g, p)
		require.Len(t, edges, 2)
		for i, e := range edges {
			require.Equal(t, i, e.Ordinal, "ordinal is the position in the injected slice")
			require.True(t, e.OfMany)

			// godi binds a slice-of-interface slot to an explicit list of the
			// implementations, so each target is named rather than searched for.
			require.Equal(t, graph.ResolutionRef, e.Resolution)
			hop, ok := e.Binding()
			require.True(t, ok)
			require.Equal(t, graph.BindOriginAutobinding, hop.Origin)
		}
	})

	t.Run("a variadic concrete type collects by element type", func(t *testing.T) {
		t.Parallel()

		c, err := godi.New().Services(
			godi.Svc(NewStoreGroup),
			godi.Svc(NewStore),
		).Build()
		require.NoError(t, err)

		g := extract(t, c)
		edges := edgesOf(g, paramOf(t, g, "v2_test.(*StoreGroup)", 0))

		require.Len(t, edges, 1)
		require.Equal(t, graph.ResolutionByElemType, edges[0].Resolution)
		require.Empty(t, edges[0].Bindings, "a concrete type needs no binding")
	})

	// A service of the slice type wins outright: the element services are never
	// looked at. Pins the branch order the container resolves by.
	t.Run("a slice service beats the individual implementations", func(t *testing.T) {
		t.Parallel()

		c, err := godi.New().Services(
			godi.Svc(NewGreeterSlice),
			godi.Svc(NewGreeterList),
			godi.Svc(NewEnGreeter),
			godi.Svc(NewPlGreeter),
		).Build()
		require.NoError(t, err)

		g := extract(t, c)
		edges := edgesOf(g, paramOf(t, g, "v2_test.(*GreeterList)", 0))

		require.Len(t, edges, 1, "the []Greeter service satisfies the slot on its own")
		require.Equal(t, graph.ResolutionBySliceType, edges[0].Resolution)
	})
}

type GreeterList struct{}

func NewGreeterSlice() []Greeter { return nil }

func NewGreeterList([]Greeter) *GreeterList { return &GreeterList{} }

type StoreGroup struct{}

func NewStoreGroup(...*Store) *StoreGroup { return &StoreGroup{} }

func TestGraphStructure(t *testing.T) {
	t.Parallel()

	t.Run("a method call gets its own params and no receiver self-edge", func(t *testing.T) {
		t.Parallel()

		c, err := godi.New().Services(
			godi.Svc(NewServer, "localhost:8080").MethodCall((*Server).SetAddr, "0.0.0.0:9090"),
			godi.Svc(NewEnGreeter),
			godi.Svc(NewStore),
		).Build()
		require.NoError(t, err)

		g := extract(t, c)
		node := nodeOf(t, g, "v2_test.(*Server)")

		var methodParams []*graph.Param
		for _, p := range node.Params {
			if p.Kind == graph.InjectMethodArg {
				methodParams = append(methodParams, p)
			}
			require.NotEqual(t, graph.InjectMethodReceiver, p.Kind,
				"the receiver is godi's own doing, not a dependency")
		}

		require.Len(t, methodParams, 1)
		require.Equal(t, "SetAddr", methodParams[0].Method)
		require.Equal(t, 1, methodParams[0].Index, "method arguments start after the receiver")

		for _, e := range g.Edges {
			require.NotEqual(t, e.From, e.To, "no node depends on itself")
		}
	})

	t.Run("a child scope is named after the definition that declared it", func(t *testing.T) {
		t.Parallel()

		c, err := godi.New().Services(
			godi.Svc(NewServer, "localhost:8080").Children(godi.Svc(NewStore)),
			godi.Svc(NewEnGreeter),
		).Build()
		require.NoError(t, err)

		g := extract(t, c)
		owner := nodeOf(t, g, "v2_test.(*Server)")
		require.NotEmpty(t, owner.ChildScope)

		child, ok := g.Scope(owner.ChildScope)
		require.True(t, ok)
		require.Equal(t, graph.ScopeID(owner.ID), child.ID,
			"a child scope is keyed by its owner, not by the uuid the container uses")
		require.Equal(t, owner.ID, child.Owner)
		require.Equal(t, graph.ScopeID("root"), child.Parent)

		// The private service is visible in the graph, and wired into its owner.
		store := nodeOf(t, g, "v2_test.(*Store)")
		require.Equal(t, child.ID, store.Scope)
		require.Equal(t, 1, store.InDegree)
	})
}

func TestGraphIsDeterministic(t *testing.T) {
	t.Parallel()

	build := func() godi.Container {
		c, err := godi.New().Services(
			godi.Svc(NewServer, "localhost:8080").MethodCall((*Server).SetAddr, "0.0.0.0:9090"),
			godi.Svc(NewEnGreeter),
			godi.Svc(NewStore),
			godi.Svc(NewCollector),
		).Build()
		require.NoError(t, err)
		return c
	}

	// Definition uuids differ between the two builds, so anything keyed by them
	// would show up here.
	require.Equal(t, stripUUIDs(extract(t, build())), stripUUIDs(extract(t, build())))
}

func stripUUIDs(g *graph.Graph) *graph.Graph {
	for _, n := range g.Nodes {
		n.UUID = ""
	}
	for _, s := range g.Scopes {
		s.Name = ""
	}
	return g
}

func TestGraphLiterals(t *testing.T) {
	t.Parallel()

	newContainer := func(t *testing.T) godi.Container {
		t.Helper()

		c, err := godi.New().Services(
			godi.Svc(NewServer, "postgres://user:hunter2@db/app"),
			godi.Svc(NewEnGreeter),
			godi.Svc(NewStore),
		).Build()
		require.NoError(t, err)
		return c
	}

	t.Run("values are left out by default", func(t *testing.T) {
		t.Parallel()

		p := paramOf(t, extract(t, newContainer(t)), "v2_test.(*Server)", 2)
		require.Len(t, p.Literals, 1)
		require.Equal(t, "string", p.Literals[0].Type)
		require.Empty(t, p.Literals[0].Value, "a literal is often a secret")
	})

	t.Run("values are included and truncated on request", func(t *testing.T) {
		t.Parallel()

		g := extract(t, newContainer(t), graph.WithLiteralValues(8))
		p := paramOf(t, g, "v2_test.(*Server)", 2)
		require.Equal(t, "postgres", p.Literals[0].Value)
		require.True(t, p.Literals[0].Truncated)
	})

	t.Run("a redactor can filter them", func(t *testing.T) {
		t.Parallel()

		g := extract(t, newContainer(t),
			graph.WithLiteralValues(0),
			graph.WithRedactor(func(_ reflect.Type, v any) (string, bool) {
				if s, ok := v.(string); ok && strings.Contains(s, "@") {
					return "<redacted>", true
				}
				return "", false
			}),
		)

		p := paramOf(t, g, "v2_test.(*Server)", 2)
		require.Equal(t, "<redacted>", p.Literals[0].Value)
		require.True(t, p.Literals[0].Redacted)
	})

	t.Run("they can be dropped entirely", func(t *testing.T) {
		t.Parallel()

		p := paramOf(t, extract(t, newContainer(t), graph.WithoutLiterals()), "v2_test.(*Server)", 2)
		require.Empty(t, p.Literals)
	})

	// A func or a channel formats as a machine address: it says nothing the type
	// does not, and it differs on every run, so two graphs of the same wiring
	// would stop comparing equal.
	t.Run("a value that formats as an address is left out", func(t *testing.T) {
		t.Parallel()

		c, err := godi.New().Services(
			godi.Svc(NewOddments,
				func(int) string { return "" },
				make(chan int),
				godi.Val(new(int)),
			),
		).Build()
		require.NoError(t, err)

		g := extract(t, c, graph.WithLiteralValues(0))
		for i, wantType := range []string{"func(int) string", "chan int", "*int"} {
			p := paramOf(t, g, "v2_test.(*Oddments)", i)
			require.Len(t, p.Literals, 1)
			require.Equal(t, wantType, p.Literals[0].Type, "the type is still reported")
			require.Empty(t, p.Literals[0].Value, "but an address is not a value worth showing")
		}
	})

	t.Run("a type that formats itself is still shown", func(t *testing.T) {
		t.Parallel()

		c, err := godi.New().Services(godi.Svc(NewSelfDescribing, SelfDescribing{})).Build()
		require.NoError(t, err)

		g := extract(t, c, graph.WithLiteralValues(0))
		p := paramOf(t, g, "v2_test.(*Described)", 0)
		require.Equal(t, "I describe myself", p.Literals[0].Value)
	})
}

type Oddments struct{}

func NewOddments(func(int) string, chan int, *int) *Oddments { return &Oddments{} }

type SelfDescribing struct{}

func (SelfDescribing) String() string { return "I describe myself" }

type Described struct{}

func NewSelfDescribing(SelfDescribing) *Described { return &Described{} }

// A root is a node nothing injects: the top of a dependency tree. It is a fact
// about the wiring, so it holds whether or not anything is eager and whatever
// the container is later asked for.
func TestGraphRoots(t *testing.T) {
	t.Parallel()

	t.Run("a service nothing injects is a root", func(t *testing.T) {
		t.Parallel()

		c, err := godi.New().Services(
			godi.Svc(NewServer, "localhost:8080"),
			godi.Svc(NewEnGreeter),
			godi.Svc(NewStore),
		).Build()
		require.NoError(t, err)

		g := extract(t, c)
		server := nodeOf(t, g, "v2_test.(*Server)")
		store := nodeOf(t, g, "v2_test.(*Store)")

		require.True(t, server.Root, "the server is injected nowhere")
		require.Zero(t, server.InDegree)
		require.False(t, store.Root, "the server takes a store")
		require.Equal(t, 1, store.InDegree)
	})

	t.Run("wiring nothing uses is a root too, and cannot be told apart", func(t *testing.T) {
		t.Parallel()

		c, err := godi.New().Services(
			godi.Svc(NewStore),
			godi.Svc(NewEnGreeter),
		).Build()
		require.NoError(t, err)

		g := extract(t, c)
		require.True(t, nodeOf(t, g, "v2_test.(*Store)").Root)
	})

	t.Run("being eager is beside the point", func(t *testing.T) {
		t.Parallel()

		c, err := godi.New().Services(
			godi.Svc(NewServer, "localhost:8080").Eager(),
			godi.Svc(NewEnGreeter),
			godi.Svc(NewStore),
		).Build()
		require.NoError(t, err)

		g := extract(t, c)
		server := nodeOf(t, g, "v2_test.(*Server)")
		store := nodeOf(t, g, "v2_test.(*Store)")

		require.True(t, server.Root)
		require.False(t, store.Root, "still injected, however it was built")

		require.True(t, server.Instantiated, "an eager service is built during Build")
		require.True(t, store.Instantiated, "and so is everything it needs")
	})
}

func TestGraphSurvivesCycles(t *testing.T) {
	t.Parallel()

	c, err := godi.New(godi.SkipCycleValidation()).Services(
		godi.Svc(NewCycleA),
		godi.Svc(NewCycleB),
	).Build()
	require.NoError(t, err)

	g := extract(t, c) // Terminates: the walk must not assume the graph is acyclic.

	var cycles int
	for _, e := range g.Edges {
		if e.Cycle {
			cycles++
		}
	}
	require.Equal(t, 1, cycles, "exactly one edge closes the loop")
}

type CycleA struct{}
type CycleB struct{}

func NewCycleA(*CycleB) *CycleA { return &CycleA{} }
func NewCycleB(*CycleA) *CycleB { return &CycleB{} }

func TestGraphFromBuilderShowsWiringBeforeAutowiring(t *testing.T) {
	t.Parallel()

	var seen *graph.Graph
	_, err := godi.New().
		Services(
			godi.Svc(NewServer, "localhost:8080"),
			godi.Svc(NewEnGreeter),
			godi.Svc(NewStore),
		).
		CompilerPasses(di.NewCompilerPass("snapshot", di.PreAutomation, di.CompilerOpFunc(
			func(b *di.ContainerBuilder) error {
				var err error
				seen, err = graph.Extract(b)
				return err
			},
		))).
		Build()
	require.NoError(t, err)
	require.NotNil(t, seen)

	// Nothing has wired the interface or the store yet.
	for _, p := range nodeOf(t, seen, "v2_test.(*Server)").Params {
		switch p.Index {
		case 0, 1:
			require.Equal(t, graph.ArgOriginNone, p.Origin)
			require.Equal(t, graph.ArgKindNone, p.Arg)
			require.Empty(t, edgesOf(seen, p))
		case 2:
			require.Equal(t, graph.ArgOriginManual, p.Origin)
		}
	}
}

// Half-wired is the normal state mid-compilation, so the graph has to say so:
// read without that, it is a container with dependencies mysteriously missing.
func TestASnapshotSaysHowFarCompilationHadGot(t *testing.T) {
	t.Parallel()

	var early, late *graph.Graph
	snapshot := func(name string, stage di.CompilerStage, into **graph.Graph) *di.CompilerPass {
		return di.NewCompilerPass(name, stage, di.CompilerOpFunc(
			func(b *di.ContainerBuilder) error {
				var err error
				*into, err = graph.Extract(b)
				return err
			},
		))
	}

	c, err := godi.New().
		Services(
			godi.Svc(NewServer, "localhost:8080"),
			godi.Svc(NewEnGreeter),
			godi.Svc(NewStore),
		).
		CompilerPasses(
			snapshot("look early", di.PreAutomation, &early),
			snapshot("look late", di.PreValidation, &late),
		).
		Build()
	require.NoError(t, err)

	require.True(t, early.Partial())
	require.Equal(t, "look early", early.Snapshot.Pass)
	require.Equal(t, "pre-automation", early.Snapshot.Stage)
	require.Empty(t, early.Snapshot.Done, "nothing had run yet")

	require.True(t, late.Partial())
	require.Equal(t, "look late", late.Snapshot.Pass)
	require.Equal(t, []string{"look early", "interface binding", "autowiring"}, late.Snapshot.Done,
		"the passes that had run are what says how much of the wiring is here")

	require.False(t, extract(t, c).Partial(), "a built container is not a snapshot")
}

func TestABuilderIsAGraphSourceBeforeItIsBuilt(t *testing.T) {
	t.Parallel()

	b := godi.New().Services(
		godi.Svc(NewServer, "localhost:8080"),
		godi.Svc(NewEnGreeter),
		godi.Svc(NewStore),
	)

	g := extract(t, b)

	require.True(t, g.Partial())
	require.Empty(t, g.Snapshot.Pass, "nothing is compiling yet")
	require.ElementsMatch(t,
		[]string{"v2_test.(*Server)", "v2_test.EnGreeter", "v2_test.(*Store)"}, nodeTypes(g))

	// What the user wrote is there; what godi would have supplied is not.
	require.Equal(t, graph.ArgOriginManual, paramOf(t, g, "v2_test.(*Server)", 2).Origin)
	require.Equal(t, graph.ArgOriginNone, paramOf(t, g, "v2_test.(*Server)", 1).Origin)
	require.Empty(t, g.Edges)
}

// The definitions the graph is read from are the ones Build compiles, so
// reading has to leave them exactly as it found them.
func TestReadingTheGraphDoesNotDisturbTheBuild(t *testing.T) {
	t.Parallel()

	b := godi.New().Services(
		godi.Svc(NewServer, "localhost:8080"),
		godi.Svc(NewEnGreeter),
		godi.Svc(NewStore),
	)

	extract(t, b)
	extract(t, b)

	c, err := b.Build()
	require.NoError(t, err)

	server, err := godi.SvcByType[*Server](c)
	require.NoError(t, err)
	require.Equal(t, "localhost:8080", server.addr)

	g := extract(t, c)
	require.False(t, g.Partial())
	require.Len(t, paramOf(t, g, "v2_test.(*Server)", 2).Literals, 1, "the argument was filled once")
}

func TestExtractRejectsASourceWithNoGraph(t *testing.T) {
	t.Parallel()

	_, err := graph.Extract(nilSource{})
	require.ErrorContains(t, err, "produced no graph")
}

// nilSource stands in for a mock container: it satisfies Source but has no
// wiring to describe.
type nilSource struct{}

func (nilSource) Graph(graph.Config) *graph.Graph { return nil }

// --- source locations -------------------------------------------------------

type locService struct{}

func newLocService() *locService { return &locService{} }

func locFunction(*locService) error { return nil }

// here is the file and line of the call site, so a test can say where it
// expects a registration without writing a line number down and watching it rot
// on the next edit.
func here() (string, int) {
	_, file, line, _ := runtime.Caller(1)
	return file, line
}

func TestLocationsPointAtTheWiringAndTheFactory(t *testing.T) {
	wantFile, wantLine := here()
	c, err := godi.New().
		Services(godi.Svc(newLocService)).
		Functions(godi.Func(locFunction)).
		Build()
	require.NoError(t, err)

	g, err := graph.Extract(c)
	require.NoError(t, err)

	svc := serviceByType(t, g, "locService")
	fn := nodeByKind(t, g, graph.NodeFunction)

	// The registration is the di.Svc line, three below the marker.
	require.Equal(t, wantFile, filepath.Join(g.SourceRoot, svc.Registered.File))
	require.Equal(t, wantLine+2, svc.Registered.Line)
	require.Contains(t, svc.Registered.Func, "TestLocationsPointAtTheWiringAndTheFactory",
		"godi's own frames must not be blamed for the registration")

	require.Equal(t, wantLine+3, fn.Registered.Line)

	// The definition is where the factory itself is written.
	require.Equal(t, wantFile, filepath.Join(g.SourceRoot, svc.Defined.File))
	require.Equal(t, factoryLine(t, "newLocService"), svc.Defined.Line)
	require.Equal(t, factoryLine(t, "locFunction"), fn.Defined.Line)
}

// A factory godi synthesised has no source of the user's behind it, so pointing
// at godi's own closure would be worse than saying nothing.
func TestASynthesisedFactoryHasNoDefinitionSite(t *testing.T) {
	c, err := godi.New().Services(godi.SvcVal("hello")).Build()
	require.NoError(t, err)

	g, err := graph.Extract(c)
	require.NoError(t, err)

	node := serviceByType(t, g, "string")
	require.True(t, node.Defined.IsZero(), "got %s", node.Defined)
	require.False(t, node.Registered.IsZero(), "the registration is still the caller's")
	require.Contains(t, node.Registered.Func, "TestASynthesisedFactoryHasNoDefinitionSite")
}

// Paths come out of the runtime absolute and build-machine specific, which is
// long to read and unstable to diff.
func TestPathsAreRelativeToASharedRoot(t *testing.T) {
	c, err := godi.New().Services(godi.Svc(newLocService)).Build()
	require.NoError(t, err)

	g, err := graph.Extract(c)
	require.NoError(t, err)

	require.True(t, filepath.IsAbs(g.SourceRoot), "the root carries the absolute part")
	for _, node := range g.Nodes {
		require.False(t, filepath.IsAbs(node.Registered.File), "%s", node.Registered)
		require.False(t, filepath.IsAbs(node.Defined.File), "%s", node.Defined)
		require.FileExists(t, filepath.Join(g.SourceRoot, node.Registered.File),
			"the root and the path must join back into something real")
	}
}

// A definition a compiler pass creates is registered by that pass, and saying
// so is the point: it pairs with the pass name already on the edge.
func TestAPassIsCreditedWithWhatItRegisters(t *testing.T) {
	var passFile string
	var passLine int
	pass := di.NewCompilerPass("add service", di.PreAutomation, di.CompilerOpFunc(
		func(b *di.ContainerBuilder) error {
			factory, err := di.NewFactory(newLocService)
			if err != nil {
				return err
			}
			passFile, passLine = here()
			def := di.NewServiceDefinition(factory)
			def.SetScope(b.RootScope())
			b.RootScope().AddServiceDefinitions(def)
			return nil
		}))

	c, err := godi.New().CompilerPasses(pass).Build()
	require.NoError(t, err)

	g, err := graph.Extract(c)
	require.NoError(t, err)

	node := serviceByType(t, g, "locService")
	require.Equal(t, passFile, filepath.Join(g.SourceRoot, node.Registered.File))
	require.Equal(t, passLine+1, node.Registered.Line, "the line inside the pass that created it")
	require.Contains(t, node.Registered.Func, "TestAPassIsCreditedWithWhatItRegisters")
}

// serviceByType finds a service by a substring of its type. Services only: a
// function that takes the type has it in its signature too.
func serviceByType(t *testing.T, g *graph.Graph, want string) *graph.Node {
	t.Helper()

	for _, node := range g.Nodes {
		if node.Kind == graph.NodeService && strings.Contains(node.Type, want) {
			return node
		}
	}
	t.Fatalf("no service whose type contains %q", want)
	return nil
}

func nodeByKind(t *testing.T, g *graph.Graph, kind graph.NodeKind) *graph.Node {
	t.Helper()

	for _, node := range g.Nodes {
		if node.Kind == kind {
			return node
		}
	}
	t.Fatalf("no %s node", kind)
	return nil
}

// factoryLine is where a function is declared, read the same way the extractor
// reads it, so the test pins the wiring rather than restating a line number.
func factoryLine(t *testing.T, name string) int {
	t.Helper()

	fns := map[string]any{"newLocService": newLocService, "locFunction": locFunction}
	fn, ok := fns[name]
	require.True(t, ok)

	pc := reflect.ValueOf(fn).Pointer()
	_, line := runtime.FuncForPC(pc).FileLine(pc)
	return line
}

// Filters are exercised in depth against a hand-built model in graph/select_test.go.
// This is the seam: that they line up with the graph a real container produces.
func TestSelectNarrowsAnExtractedGraph(t *testing.T) {
	t.Parallel()

	build := func(t *testing.T) godi.Container {
		t.Helper()

		c, err := godi.New().Services(
			godi.Svc(NewServer, "localhost:8080"),
			godi.Svc(NewEnGreeter),
			godi.Svc(NewStore).Labels("storage"),
		).Build()
		require.NoError(t, err)
		return c
	}

	t.Run("a focus narrows the graph to what surrounds the selection", func(t *testing.T) {
		t.Parallel()

		g := extract(t, build(t)).Select(graph.Focus(graph.ByType("*v2_test.(*Store)"), graph.Consumers(1)))

		require.ElementsMatch(t, []string{"v2_test.(*Store)", "v2_test.(*Server)"}, nodeTypes(g))
	})

	t.Run("an exclusion drops what it names", func(t *testing.T) {
		t.Parallel()

		g := extract(t, build(t)).Select(graph.ExcludeLabels("storage"))

		require.NotContains(t, nodeTypes(g), "v2_test.(*Store)")
	})

	t.Run("the graph is whole when nothing is filtered", func(t *testing.T) {
		t.Parallel()

		g := extract(t, build(t)).Select()

		require.Len(t, g.Nodes, 3)
	})
}

// A function is a value like any other, so SvcVal is how one becomes a service.
// The factory holding it is godi's own, and reporting that as the implementation
// would point the reader into godi rather than at the function they registered.

type Validate func(string) error

type Rules struct{}

func (Rules) Check(string) error { return nil }

func validateEmail(string) error { return nil }

type Settings struct{ Addr string }

func TestAServiceRegisteredAsAValueIsDescribedByTheValue(t *testing.T) {
	t.Parallel()

	rules := Rules{}

	c, err := godi.New().Services(
		godi.SvcVal[Validate](validateEmail),
		godi.SvcVal[Greeter](EnGreeter{}),
		godi.SvcVal(Settings{Addr: "localhost"}),
		godi.Svc(NewStore),
	).Build()
	require.NoError(t, err)

	g := extract(t, c)

	t.Run("a named function is named, and points at its own source", func(t *testing.T) {
		n := nodeOf(t, g, "v2_test.Validate")

		require.True(t, n.FromValue)
		require.False(t, n.Anonymous())
		require.Equal(t, "github.com/michalkurzeja/godi/v2_test.validateEmail", n.Name)
		require.Equal(t, "func(string) error", n.Signature,
			"the value's own type is the named one, which says nothing about what it takes")
		require.Equal(t, "graph_test.go", filepath.Base(n.Defined.File))
		require.NotZero(t, n.Defined.Line)
	})

	t.Run("and nothing of godi's is reported as the implementation", func(t *testing.T) {
		for _, n := range g.Nodes {
			require.NotContains(t, n.Name, "SvcVal", "the wrapper godi built to hold the value")
			require.NotContains(t, n.Defined.File, "definition.go")
		}
	})

	t.Run("a value that is no function has nothing to describe it", func(t *testing.T) {
		n := nodeOf(t, g, "v2_test.Settings")

		require.True(t, n.FromValue)
		require.Empty(t, n.Name, "there is no name for a struct someone handed over")
		require.Empty(t, n.Signature)
		require.True(t, n.Defined.IsZero(), "and nowhere to point at for it")
	})

	t.Run("a factory-built service is unaffected", func(t *testing.T) {
		n := nodeOf(t, g, "v2_test.(*Store)")

		require.False(t, n.FromValue)
		require.Equal(t, "github.com/michalkurzeja/godi/v2_test.NewStore", n.Name)
		// reflect names the package rather than its path in a func type, which
		// is how every signature in the graph has always read.
		require.Equal(t, "func() *di_test.Store", n.Signature)
	})

	t.Run("a method value is named after the method", func(t *testing.T) {
		c, err := godi.New().Services(godi.SvcVal[Validate](rules.Check)).Build()
		require.NoError(t, err)

		n := nodeOf(t, extract(t, c), "v2_test.Validate")

		require.Equal(t, "github.com/michalkurzeja/godi/v2_test.Rules.Check", n.Name,
			"the suffix the compiler puts on a method value is not part of the name")
		require.True(t, n.Defined.IsZero(),
			"the wrapper it points at is generated, so there is no source to offer")
	})

	t.Run("an anonymous function has only its signature", func(t *testing.T) {
		c, err := godi.New().Services(
			godi.SvcVal[Validate](func(string) error { return nil }),
		).Build()
		require.NoError(t, err)

		n := nodeOf(t, extract(t, c), "v2_test.Validate")

		require.True(t, n.Anonymous())
		require.Equal(t, "func(string) error", n.Signature)
		require.Equal(t, "graph_test.go", filepath.Base(n.Defined.File),
			"the literal itself is the implementation, and it has a line of its own")
	})
}
