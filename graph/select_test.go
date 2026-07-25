package graph_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/michalkurzeja/godi/v2/graph"
)

const (
	server  = "root/svc:app.(*Server)"
	router  = "root/svc:app.(*Router)"
	repo    = "root/svc:app.(*Repo)"
	config  = "root/svc:app.(*Config)"
	logger  = "root/svc:app.ConsoleLogger"
	metrics = "root/svc:app.(*Metrics)"
	conn    = "root/svc:app.(*Server)/svc:app.(*Conn)"

	childScope = graph.ScopeID(server)
)

// model is a container shaped to make the filters answerable: two services
// share a dependency, one service owns a child scope, one is wired to nothing,
// and one argument arrives through a method call.
//
//	Server ─┬─▶ Router ──▶ Config ◀── Repo
//	        ├─▶ Conn (child scope)
//	        └─▶ ConsoleLogger (method call, through a binding)
//	Metrics (wired to nothing)
func model() *graph.Graph {
	const pkg = "github.com/acme/app"

	node := func(id, typ, file string, params ...*graph.Param) *graph.Node {
		return &graph.Node{
			ID: graph.NodeID(id), Kind: graph.NodeService, Scope: "root",
			Type: pkg + "." + typ, Name: pkg + ".New" + strings.Trim(typ, "(*)"),
			Shared: true, Lazy: true, Autowired: true,
			Registered: graph.Location{File: file, Line: 12},
			Params:     params,
		}
	}

	param := func(node, id, typ string, kind graph.InjectionKind) *graph.Param {
		return &graph.Param{
			ID: graph.ParamID(node + "#" + id), Node: graph.NodeID(node),
			Kind: kind, Type: pkg + "." + typ, Origin: graph.ArgOriginAutowiring,
		}
	}

	serverRouter := param(server, "f:0", "(*Router)", graph.InjectFactoryArg)
	serverConn := param(server, "f:1", "(*Conn)", graph.InjectFactoryArg)
	serverLogger := param(server, "m:SetLogger:1", "Logger", graph.InjectMethodArg)
	serverLogger.Method, serverLogger.Index = "SetLogger", 1
	routerConfig := param(router, "f:0", "(*Config)", graph.InjectFactoryArg)
	repoConfig := param(repo, "f:0", "(*Config)", graph.InjectFactoryArg)

	nodes := []*graph.Node{
		node(config, "(*Config)", "wiring.go"),
		node(metrics, "(*Metrics)", "wiring.go"),
		node(repo, "(*Repo)", "internal/data/wiring.go", repoConfig),
		node(router, "(*Router)", "wiring.go", routerConfig),
		node(server, "(*Server)", "wiring.go", serverRouter, serverConn, serverLogger),
		node(conn, "(*Conn)", "wiring.go"),
		node(logger, "ConsoleLogger", "wiring.go"),
	}
	nodes[2].Labels = []string{"data"}
	nodes[5].Scope = childScope
	byID := make(map[graph.NodeID]*graph.Node, len(nodes))
	for _, n := range nodes {
		byID[n.ID] = n
	}
	byID[graph.NodeID(server)].ChildScope = childScope

	binding := graph.BindingHop{Interface: pkg + ".Logger", Scope: "root", Origin: graph.BindOriginAutobinding}

	edge := func(p *graph.Param, to string, hops ...graph.BindingHop) *graph.Edge {
		return &graph.Edge{
			ID: graph.NewEdgeID(p.ID, 0), From: p.Node, To: graph.NodeID(to), Param: p.ID,
			Kind: p.Kind, Origin: p.Origin, Resolution: graph.ResolutionByType,
			Bindings: hops, ParamType: p.Type,
		}
	}

	g := &graph.Graph{
		Schema: graph.Schema,
		Scopes: []*graph.Scope{
			{ID: "root", Name: "root"},
			{ID: childScope, Parent: "root", Depth: 1, Name: "uuid-1", Owner: server, OwnerName: pkg + ".(*Server)"},
		},
		Nodes: nodes,
		Edges: []*graph.Edge{
			edge(repoConfig, config),
			edge(routerConfig, config),
			edge(serverRouter, router),
			edge(serverConn, conn),
			edge(serverLogger, logger, binding),
		},
		Bindings: []*graph.Binding{{
			Scope: "root", Interface: pkg + ".Logger", Origin: graph.BindOriginAutobinding,
			BoundTo: pkg + ".ConsoleLogger", Targets: []graph.NodeID{logger},
		}},
	}

	// Derived rather than written down, so the fixture cannot drift from itself
	// as it grows. This is the same arithmetic the extractor does.
	for _, e := range g.Edges {
		byID[e.From].OutDegree++
		byID[e.To].InDegree++
		for _, n := range nodes {
			for _, p := range n.Params {
				if p.ID == e.Param {
					p.EdgeCount++
				}
			}
		}
	}
	for _, n := range nodes {
		n.Root = n.InDegree == 0
	}
	return g
}

func ids(g *graph.Graph) []string {
	out := make([]string, 0, len(g.Nodes))
	for _, n := range g.Nodes {
		out = append(out, string(n.ID))
	}
	return out
}

// only keeps exactly what the matcher accepts, so a test of what a matcher
// matches is not also a test of what Focus reaches from there.
func only(match graph.Matcher) graph.Filter { return graph.Exclude(graph.Not(match)) }

func node(t *testing.T, g *graph.Graph, id string) *graph.Node {
	t.Helper()
	n, ok := g.Node(graph.NodeID(id))
	require.True(t, ok, "node %s is missing from the graph", id)
	return n
}

// ---------------------------------------------------------------- matchers ---

func TestAPatternMatchesTheWholeNameOrItsShortForm(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		pattern string
		want    []string
	}{
		{"the qualified name", "github.com/acme/app.(*Config)", []string{config}},
		{"the short form", "app.(*Config)", []string{config}},
		{"a trailing star", "github.com/acme/*", []string{config, metrics, repo, router, server, conn, logger}},
		{"a leading star", "*ConsoleLogger", []string{logger}},
		{"a star in the middle", "app.(*C*)", []string{config, conn}},
		{"a lone star", "*", []string{config, metrics, repo, router, server, conn, logger}},
		{"no wildcard is not a substring", "Config", nil},
		{"nothing matches nothing", "app.(*Nope)", nil},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got := model().Select(only(graph.ByType(test.pattern)))
			if test.want == nil {
				require.Empty(t, ids(got))
				return
			}
			require.ElementsMatch(t, test.want, ids(got))
		})
	}
}

func TestMatchersReachEveryWayANodeIsNamed(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		match graph.Matcher
		want  []string
	}{
		{"by label", graph.ByLabel("data"), []string{repo}},
		{"by graph id", graph.ByID("*ConsoleLogger"), []string{logger}},
		{"by file", graph.ByFile("internal/*"), []string{repo}},
		{"by name", graph.ByName("*NewMetrics"), []string{metrics}},
		{"any of several", graph.Any(graph.ByLabel("data"), graph.ByType("*Metrics)")), []string{repo, metrics}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got := model().Select(only(test.match))
			require.ElementsMatch(t, test.want, ids(got))
		})
	}
}

func TestAllNarrowsWhereAnyWidens(t *testing.T) {
	t.Parallel()

	kind, labelled := graph.ByType("*Repo)", "*Metrics)"), graph.ByLabel("data")

	require.ElementsMatch(t, []string{repo, metrics}, ids(model().Select(only(graph.Any(kind, labelled)))))
	require.ElementsMatch(t, []string{repo}, ids(model().Select(only(graph.All(kind, labelled)))))
}

func TestAllOfNothingMatchesEverything(t *testing.T) {
	t.Parallel()

	got := model().Select(only(graph.All()))

	require.ElementsMatch(t, ids(model()), ids(got))
}

func TestNotInvertsAMatcher(t *testing.T) {
	t.Parallel()

	got := model().Select(graph.Exclude(graph.Not(graph.ByType("*Config)", "*Repo)"))))

	require.ElementsMatch(t, []string{config, repo}, ids(got))
}

// ------------------------------------------------------------------- focus ---

func TestFocusFollowsBothDirectionsAsFarAsTheyGo(t *testing.T) {
	t.Parallel()

	got := model().Select(graph.Focus(graph.ByType("*Router)")))

	// Up to the Server and down to the Config, but not across to the Repo: it
	// is reached only by turning round at the Config.
	require.ElementsMatch(t, []string{server, router, config}, ids(got))
}

// The same rule the viewer's hop slider follows. A sibling shares a dependency
// and nothing else; it is not on any path through the selection.
func TestAHopNeverTurnsAround(t *testing.T) {
	t.Parallel()

	got := model().Select(graph.Focus(graph.ByType("*Repo)"), graph.Dependencies(5), graph.Consumers(5)))

	require.ElementsMatch(t, []string{repo, config}, ids(got),
		"the Router turned up, and it only shares the Config")
}

func TestNamingOneDirectionTakesTheOtherOut(t *testing.T) {
	t.Parallel()

	deps := model().Select(graph.Focus(graph.ByType("*Router)"), graph.Dependencies(1)))
	require.ElementsMatch(t, []string{router, config}, ids(deps), "following dependencies reached a consumer")

	consumers := model().Select(graph.Focus(graph.ByType("*Router)"), graph.Consumers(1)))
	require.ElementsMatch(t, []string{router, server}, ids(consumers), "following consumers reached a dependency")
}

func TestHopsAreCountedInEdges(t *testing.T) {
	t.Parallel()

	one := model().Select(graph.Focus(graph.ByType("*Server)"), graph.Dependencies(1)))
	require.ElementsMatch(t, []string{server, router, conn, logger}, ids(one))

	two := model().Select(graph.Focus(graph.ByType("*Server)"), graph.Dependencies(2)))
	require.ElementsMatch(t, []string{server, router, conn, logger, config}, ids(two))
}

func TestFocusSaysHowManyNeighboursItCutOff(t *testing.T) {
	t.Parallel()

	got := model().Select(graph.Focus(graph.ByType("*Router)"), graph.Dependencies(0), graph.Consumers(0)))

	require.Equal(t, []string{router}, ids(got))
	require.Equal(t, 2, node(t, got, router).Elided, "the Server above it and the Config below it")
}

// An exclusion is a node the caller named. Its absence is not news, so nothing
// is said about it - unlike a limit, which stops somewhere arbitrary.
func TestAnExclusionSaysNothing(t *testing.T) {
	t.Parallel()

	got := model().Select(graph.ExcludeTypes("*Config)"))

	require.Equal(t, 0, node(t, got, router).Elided)
	require.Equal(t, 0, node(t, got, repo).Elided)
}

// ----------------------------------------------------------------- filters ---

func TestExcludeDropsTheSelectedNodesAndTheirEdges(t *testing.T) {
	t.Parallel()

	got := model().Select(graph.ExcludeTypes("*Config)"))

	require.NotContains(t, ids(got), config)
	require.Len(t, got.Edges, 3, "the two edges into the Config went with it")
	require.Equal(t, 0, node(t, got, router).OutDegree)
}

func TestExcludeLabelsDropsByLabel(t *testing.T) {
	t.Parallel()

	got := model().Select(graph.ExcludeLabels("data"))

	require.NotContains(t, ids(got), repo)
}

func TestOnlyScopeKeepsAScopeAndTheScopesAroundIt(t *testing.T) {
	t.Parallel()

	got := model().Select(graph.OnlyScope("children of app.(*Server)"))

	require.Equal(t, []string{conn}, ids(got))
	require.Len(t, got.Scopes, 2, "the root scope stays, or the child has nowhere to hang")
	require.Equal(t, graph.ScopeID("root"), got.Scopes[0].ID)
}

func TestAScopeWithNothingLeftInItGoes(t *testing.T) {
	t.Parallel()

	got := model().Select(graph.OnlyScope("root"))

	require.NotContains(t, ids(got), conn)
	require.Len(t, got.Scopes, 1, "the child scope is empty and should not be drawn")
}

func TestOnlyRootsKeepsWhatNothingInjects(t *testing.T) {
	t.Parallel()

	got := model().Select(graph.OnlyRoots())

	require.ElementsMatch(t, []string{server, repo, metrics}, ids(got))
	require.Empty(t, got.Edges, "roots have nothing between them")
}

func TestHideMethodCallsDropsTheRowAndTheEdge(t *testing.T) {
	t.Parallel()

	got := model().Select(graph.HideMethodCalls())

	require.Contains(t, ids(got), logger, "the service stays; only that way of reaching it goes")
	require.Equal(t, 0, node(t, got, logger).InDegree)
	require.Len(t, node(t, got, server).Params, 2, "the method argument row went too")
	for _, e := range got.Edges {
		require.NotEqual(t, graph.InjectMethodArg, e.Kind)
	}
}

func TestMaxNodesKeepsTheMostConnected(t *testing.T) {
	t.Parallel()

	got := model().Select(graph.MaxNodes(3))

	// Server has three edges, the Config and the Router two each; the tie goes
	// to the lower ID so that the same graph always yields the same view.
	require.ElementsMatch(t, []string{server, config, router}, ids(got))
	require.Equal(t, 2, node(t, got, server).Elided, "the Conn and the logger")
	require.Equal(t, 1, node(t, got, config).Elided, "the Repo")
}

func TestMaxNodesDoesNothingToASmallGraph(t *testing.T) {
	t.Parallel()

	got := model().Select(graph.MaxNodes(100))

	require.Len(t, ids(got), 7)
	require.Equal(t, 0, node(t, got, server).Elided)
}

func TestFiltersCompose(t *testing.T) {
	t.Parallel()

	got := model().Select(
		graph.Focus(graph.ByType("*Server)"), graph.Dependencies(1)),
		graph.HideMethodCalls(),
	)

	require.ElementsMatch(t, []string{server, router, conn}, ids(got),
		"the logger was only reachable through the method call")
}

// Hiding a kind of wiring changes what is reachable, so it has to happen before
// anything follows the wiring. Otherwise the answer would depend on which
// filter the caller wrote down first, which is not something anyone should have
// to think about.
func TestTheOrderFiltersAreGivenInDoesNotMatter(t *testing.T) {
	t.Parallel()

	focus := graph.Focus(graph.ByType("*Server)"), graph.Dependencies(1))

	first := model().Select(graph.HideMethodCalls(), focus)
	last := model().Select(focus, graph.HideMethodCalls())

	require.Equal(t, ids(first), ids(last))
}

// --------------------------------------------------------------- soundness ---

func TestTheCountsDescribeTheGraphYouAreLookingAt(t *testing.T) {
	t.Parallel()

	got := model().Select(graph.ExcludeTypes("*Router)"))

	require.Equal(t, 1, node(t, got, config).InDegree, "only the Repo is left asking for it")
	require.Equal(t, 2, node(t, got, server).OutDegree, "the Router edge went")

	param, ok := got.Param(graph.ParamID(server + "#f:0"))
	require.True(t, ok)
	require.Equal(t, 0, param.EdgeCount, "the row is still there, and now resolves to nothing")
}

// Root is a fact about the container, not about the picture. Recomputing it
// after filtering would call a service an entry point because its only consumer
// was hidden.
func TestRootSurvivesFiltering(t *testing.T) {
	t.Parallel()

	got := model().Select(graph.ExcludeTypes("*Server)"))

	require.False(t, node(t, got, router).Root, "the Server still injects it, filtered out or not")
	require.Equal(t, 0, node(t, got, router).InDegree)
}

func TestBindingsFollowTheFilteredGraph(t *testing.T) {
	t.Parallel()

	kept := model().Select(graph.ExcludeTypes("*Metrics)"))
	require.Len(t, kept.Bindings, 1)
	require.Equal(t, 1, kept.Bindings[0].EdgeCount)

	gone := model().Select(graph.HideMethodCalls())
	require.Len(t, gone.Bindings, 1)
	require.Equal(t, 0, gone.Bindings[0].EdgeCount, "nothing resolves through it any more")

	dropped := model().Select(graph.ExcludeTypes("*ConsoleLogger"))
	require.Empty(t, dropped.Bindings, "its only target went")
}

func TestFilteringLeavesTheOriginalAlone(t *testing.T) {
	t.Parallel()

	g := model()
	before := len(g.Nodes)

	narrowed := g.Select(graph.Focus(graph.ByType("*Router)"), graph.Dependencies(0), graph.Consumers(0)))

	require.Len(t, g.Nodes, before)
	require.Len(t, narrowed.Nodes, 1)
	require.Equal(t, 1, node(t, g, router).InDegree, "the source graph's counts were rewritten")
	require.Equal(t, 0, node(t, g, router).Elided)
}

func TestNoFiltersChangesNothing(t *testing.T) {
	t.Parallel()

	g := model()

	require.Same(t, g, g.Select())
}

// A narrowed snapshot is still a snapshot: whatever the reader is warned about
// has to survive being filtered, or the warning goes missing exactly when the
// picture gets smaller and looks more complete.
func TestFilteringKeepsWhatTheGraphSaysAboutItself(t *testing.T) {
	t.Parallel()

	g := model()
	g.Snapshot = &graph.Snapshot{Stage: "automation", Pass: "autowiring"}

	got := g.Select(graph.OnlyRoots())

	require.True(t, got.Partial())
	require.Equal(t, g.Snapshot, got.Snapshot)
}

// A Filter can only be built by the functions in this package, but the zero
// value is still writable, so it has to mean "no question asked".
func TestAZeroFilterAsksNothing(t *testing.T) {
	t.Parallel()

	got := model().Select(graph.Filter{})

	require.ElementsMatch(t, ids(model()), ids(got))
}

// Encode is defensive about a nil graph, and these are the two things anyone
// does to one.
func TestFilteringNothingIsNotAPanic(t *testing.T) {
	t.Parallel()

	var g *graph.Graph

	require.Nil(t, g.Select())
	require.Nil(t, g.Select(graph.OnlyRoots()))
}

func TestAnEmptyResultIsStillAGraph(t *testing.T) {
	t.Parallel()

	got := model().Select(graph.ExcludeTypes("*"))

	require.Empty(t, got.Nodes)
	require.Empty(t, got.Edges)
	require.Empty(t, got.Scopes)
	require.Equal(t, graph.Schema, got.Schema)
}
