package graph_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/michalkurzeja/godi/v2/graph"
)

// The model carries a handful of decisions that every encoder relies on giving
// the same answer. They are pinned here rather than three times over.

func TestALiteralIsRenderedTheSameWayForEveryFormat(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		given graph.Literal
		want  string
	}{
		{"a value", graph.Literal{Type: "string", Value: "localhost"}, "localhost"},
		{"a truncated value", graph.Literal{Type: "string", Value: "postgres://us", Truncated: true}, "postgres://us…"},
		{"no value, because none was asked for", graph.Literal{Type: "string"}, "‹literal›"},
		{"a redactor's replacement", graph.Literal{Type: "string", Value: "***", Redacted: true}, "***"},
		{"redacted to nothing at all", graph.Literal{Type: "string", Redacted: true}, "‹redacted›"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, test.want, test.given.String())
		})
	}
}

func TestLiteralsTextRunsThemTogether(t *testing.T) {
	t.Parallel()

	p := &graph.Param{Literals: []graph.Literal{
		{Type: "string", Value: "a"},
		{Type: "int"},
	}}

	require.Equal(t, "a, ‹literal›", p.LiteralsText())
	require.Empty(t, (&graph.Param{}).LiteralsText())
}

// A pass can be responsible in either of two ways, and the answer is the same
// either way - so no encoder has to know which happened.
func TestPassCreditNamesWhicheverPassWasResponsible(t *testing.T) {
	t.Parallel()

	hop := func(origin graph.BindOrigin, pass string) []graph.BindingHop {
		return []graph.BindingHop{{Interface: "app.Logger", Origin: origin, OriginPass: pass}}
	}

	tests := []struct {
		name string
		edge graph.Edge
		want string
	}{
		{"nobody", graph.Edge{Origin: graph.ArgOriginAutowiring}, ""},
		{
			"a pass wired the argument",
			graph.Edge{Origin: graph.ArgOriginCompilerPass, OriginPass: "override arg"},
			"override arg",
		},
		{
			"a pass created the binding it resolved through",
			graph.Edge{Origin: graph.ArgOriginAutowiring, Bindings: hop(graph.BindOriginCompilerPass, "bind reporter")},
			"bind reporter",
		},
		{
			"the same pass did both",
			graph.Edge{
				Origin: graph.ArgOriginCompilerPass, OriginPass: "rewire",
				Bindings: hop(graph.BindOriginCompilerPass, "rewire"),
			},
			"rewire",
		},
		{
			"two different passes each had a hand in it",
			graph.Edge{
				Origin: graph.ArgOriginCompilerPass, OriginPass: "override arg",
				Bindings: hop(graph.BindOriginCompilerPass, "bind reporter"),
			},
			"arg: override arg, bind: bind reporter",
		},
		{
			"godi's own automation is not worth naming",
			graph.Edge{Origin: graph.ArgOriginAutowiring, Bindings: hop(graph.BindOriginAutobinding, "")},
			"",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, test.want, test.edge.PassCredit())
		})
	}
}

func TestBindingReturnsTheHopAppliedToTheDeclaredType(t *testing.T) {
	t.Parallel()

	_, ok := (&graph.Edge{}).Binding()
	require.False(t, ok)

	e := &graph.Edge{Bindings: []graph.BindingHop{{Interface: "app.A"}, {Interface: "app.B"}}}
	hop, ok := e.Binding()
	require.True(t, ok)
	require.Equal(t, "app.A", hop.Interface, "the first hop, not the last")
}

// A child scope's own name is the uuid of the definition that declared it,
// which says nothing to a reader.
func TestAScopeIsNamedAfterWhateverDeclaredIt(t *testing.T) {
	t.Parallel()

	root := &graph.Scope{ID: "root", Name: "root"}
	require.Equal(t, "root", root.Label())

	child := &graph.Scope{
		ID: "root/svc:app.(*Server)", Name: "0d3f-uuid",
		Owner:     "root/svc:github.com/acme/app.(*Server)",
		OwnerName: "github.com/acme/app.(*Server)",
	}
	require.Equal(t, "children of app.(*Server)", child.Label())
}

// The name is recorded at extraction rather than read back out of the ID. An ID
// is a path built for uniqueness: it accumulates every scope above it, import
// paths and all, and shortening that gives the mess this replaced.
func TestAScopeNameDoesNotGrowWithItsDepth(t *testing.T) {
	t.Parallel()

	const deep = "root" +
		"/svc:github.com/acme/app/temporalx.(*Worker)" +
		"/svc:github.com/acme/app/indexing.(*IndexProfilesActivities)[temporal:activity]" +
		"/svc:github.com/acme/app/sources/profile.(*Sources)"

	scope := &graph.Scope{
		ID: graph.ScopeID(deep), Name: "0d3f-uuid", Owner: graph.NodeID(deep),
		OwnerName: "github.com/acme/app/sources/profile.(*Sources)",
	}

	require.Equal(t, "children of profile.(*Sources)", scope.Label())
}

func TestALocationReadsAsAPlace(t *testing.T) {
	t.Parallel()

	require.True(t, graph.Location{}.IsZero())
	require.Empty(t, graph.Location{}.String())

	require.Equal(t, "wiring.go:42", graph.Location{File: "wiring.go", Line: 42}.String())
	require.Equal(t, "wiring.go", graph.Location{File: "wiring.go"}.String(),
		"a file with no line is still worth showing")
	require.False(t, graph.Location{File: "wiring.go"}.IsZero())
}

func TestAnEdgeIDIsTheParamAndThePosition(t *testing.T) {
	t.Parallel()

	require.Equal(t, graph.EdgeID("root/svc:app.(*Router)#f:0@2"),
		graph.NewEdgeID("root/svc:app.(*Router)#f:0", 2))
}

func TestEncodingANilGraphIsAnErrorRatherThanAPanic(t *testing.T) {
	t.Parallel()

	var g *graph.Graph

	require.ErrorContains(t, g.Encode(nil, nil), "nil graph")
}

func TestTheLookupsFindWhatIsThere(t *testing.T) {
	t.Parallel()

	g := model()

	n, ok := g.Node(graph.NodeID(server))
	require.True(t, ok)
	require.Equal(t, graph.NodeID(server), n.ID)

	_, ok = g.Node("root/svc:app.(*Nope)")
	require.False(t, ok)

	p, ok := g.Param(graph.ParamID(server + "#f:0"))
	require.True(t, ok)
	require.Equal(t, graph.NodeID(server), p.Node)

	s, ok := g.Scope("root")
	require.True(t, ok)
	require.Equal(t, "root", s.Name)

	require.Len(t, g.OutEdges(graph.NodeID(server)), 3)
	require.Len(t, g.InEdges(graph.NodeID(config)), 2)
	require.Len(t, g.ChildScopes("root"), 1)
	require.Len(t, g.ScopeNodes(childScope), 1)
}

// The qualified type is what the detail panel used to show, and a generic makes
// it unreadable. The package is the part of it worth keeping.
func TestANodeKnowsWhichPackageItBelongsTo(t *testing.T) {
	t.Parallel()

	const pkg = "github.com/acme/app"

	tests := []struct {
		name string
		node graph.Node
		want string
	}{
		{
			"a service, from its type",
			graph.Node{Kind: graph.NodeService, Type: pkg + ".(*Server)", Name: pkg + ".NewServer"},
			pkg,
		},
		{
			"a generic service, from the type rather than its arguments",
			graph.Node{Kind: graph.NodeService, Type: pkg + ".Cache[github.com/other/x.Key]", Name: pkg + ".NewCache"},
			pkg,
		},
		{
			// A service can be of a builtin type; the factory that made it
			// still lives somewhere.
			"a service of no package, from its factory",
			graph.Node{Kind: graph.NodeService, Type: "string", Name: pkg + ".NewGreeting"},
			pkg,
		},
		{
			// A function's type is only a signature, so it never has one.
			"a function, from its name",
			graph.Node{Kind: graph.NodeFunction, Type: "func() error", Name: pkg + ".migrate"},
			pkg,
		},
		{"nothing to go on", graph.Node{Kind: graph.NodeService, Type: "string", Name: "closure"}, ""},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, test.want, test.node.Package())
		})
	}
}

// A function literal has no name of its own, so what the graph carries is what
// the runtime calls it. That identifies it without describing it, which is why
// it is worth telling apart.
func TestAFunctionLiteralIsRecognisedByItsRuntimeName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		node graph.Node
		want bool
	}{
		{
			"a literal",
			graph.Node{Kind: graph.NodeFunction, Name: "main.build.func1"},
			true,
		},
		{
			"one nested in another",
			graph.Node{Kind: graph.NodeFunction, Name: "github.com/acme/app.wire.func2.1"},
			true,
		},
		{
			"one inside a method",
			graph.Node{Kind: graph.NodeFunction, Name: "github.com/acme/app.(*Builder).build.func1"},
			true,
		},
		{
			"a named function",
			graph.Node{Kind: graph.NodeFunction, Name: "github.com/acme/app.migrate"},
			false,
		},
		{
			// Nothing stops someone naming a function this way themselves. What
			// separates them is that a literal is named after whatever encloses
			// it, so it always has a qualifier in front of the counter.
			"a function someone called func1",
			graph.Node{Kind: graph.NodeFunction, Name: "main.func1"},
			false,
		},
		{
			"the same, in a package with a path",
			graph.Node{Kind: graph.NodeFunction, Name: "github.com/acme/app.func1"},
			false,
		},
		{
			"a function whose name merely contains the marker",
			graph.Node{Kind: graph.NodeFunction, Name: "github.com/acme/app.myfunc2"},
			false,
		},
		{
			"a named function whose name only looks like one",
			graph.Node{Kind: graph.NodeFunction, Name: "github.com/acme/app.functional"},
			false,
		},
		{
			"a bare name",
			graph.Node{Kind: graph.NodeFunction, Name: "func1"},
			false,
		},
		{
			// A service can be built by a literal too, or registered as one.
			"a service built by a literal",
			graph.Node{Kind: graph.NodeService, Name: "main.build.func1"},
			true,
		},
		{
			"a service built by a factory of its own",
			graph.Node{Kind: graph.NodeService, Name: "main.NewServer"},
			false,
		},
		{
			"a service registered as a value that is not a function at all",
			graph.Node{Kind: graph.NodeService, Name: ""},
			false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, test.want, test.node.Anonymous())
		})
	}
}

func TestASnapshotSaysWhenItWasTaken(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		given *graph.Snapshot
		want  string
	}{
		{"nothing to say about a finished container", nil, ""},
		{"before anything ran", &graph.Snapshot{}, "taken before the container was compiled"},
		{"between stages", &graph.Snapshot{Stage: "validation"}, "taken before the validation stage"},
		{
			"inside a pass, which is the more precise answer",
			&graph.Snapshot{Stage: "automation", Pass: "autowiring"},
			"taken during the autowiring pass",
		},
		{
			"where it stopped, which beats both",
			&graph.Snapshot{Stage: "validation", Failed: "argument validation"},
			"taken where the argument validation pass failed",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, test.want, test.given.Label())
		})
	}
}

func TestOnlyASnapshotMakesAGraphPartial(t *testing.T) {
	t.Parallel()

	var nilGraph *graph.Graph

	require.False(t, nilGraph.Partial())
	require.False(t, (&graph.Graph{}).Partial())
	require.True(t, (&graph.Graph{Snapshot: &graph.Snapshot{}}).Partial())
}
