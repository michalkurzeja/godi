package di_test

import (
	"reflect"
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

func extract(t *testing.T, c godi.Container, opts ...graph.Option) *graph.Graph {
	t.Helper()

	g, err := graph.Extract(c, opts...)
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

func TestGraphReachability(t *testing.T) {
	t.Parallel()

	t.Run("an unreferenced lazy service is not reachable", func(t *testing.T) {
		t.Parallel()

		c, err := godi.New().Services(
			godi.Svc(NewStore),
			godi.Svc(NewEnGreeter),
		).Build()
		require.NoError(t, err)

		g := extract(t, c)
		require.Empty(t, g.Roots, "nothing is eager and there are no functions")

		store := nodeOf(t, g, "v2_test.(*Store)")
		require.False(t, store.ReachableFromRoots)
		require.False(t, store.Instantiated)
		require.Contains(t, g.Unreachable(), store)
	})

	t.Run("a service an eager service needs is reachable and built", func(t *testing.T) {
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

		require.Contains(t, g.Roots, server.ID)
		require.True(t, server.ReachableFromRoots)
		require.True(t, store.ReachableFromRoots, "reached through the eager server")

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

func TestExtractRejectsASourceWithNoGraph(t *testing.T) {
	t.Parallel()

	_, err := graph.Extract(nilSource{})
	require.ErrorContains(t, err, "produced no graph")
}

// nilSource stands in for a mock container: it satisfies Source but has no
// wiring to describe.
type nilSource struct{}

func (nilSource) Graph(graph.Config) *graph.Graph { return nil }
