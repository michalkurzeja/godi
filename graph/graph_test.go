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
		ID: "root/svc:app.(*Server)", Name: "0d3f-uuid", Owner: "root/svc:github.com/acme/app.(*Server)",
	}
	require.Equal(t, "children of app.(*Server)", child.Label())
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
