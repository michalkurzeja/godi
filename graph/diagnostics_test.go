package graph_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/michalkurzeja/godi/v2/graph"
)

func diagnosed() *graph.Graph {
	return &graph.Graph{
		Diagnostics: []graph.Diagnostic{
			{Severity: graph.SeverityWarning, Message: "this graph names no schema"},
		},
		Scopes: []*graph.Scope{{
			ID:          "root/plugins",
			Diagnostics: []graph.Diagnostic{{Severity: graph.SeverityInfo, Message: `scope "plugins" belongs to no definition`}},
		}},
		Nodes: []*graph.Node{{
			ID:          "root/svc:app.(*Server)",
			Type:        "github.com/acme/app.(*Server)",
			Scope:       "root",
			Diagnostics: []graph.Diagnostic{{Severity: graph.SeverityError, Message: "connection refused", Pass: "eager initialization"}},
			Params: []*graph.Param{
				{ID: "p0", Node: "root/svc:app.(*Server)", Index: 0,
					Diagnostics: []graph.Diagnostic{{Severity: graph.SeverityError, Message: "no services of type app.(*Store)"}}},
				{ID: "p1", Node: "root/svc:app.(*Server)", Index: 1, Origin: graph.ArgOriginManual},
				{ID: "p2", Node: "root/svc:app.(*Server)", Index: 2, Method: "SetLogger",
					Diagnostics: []graph.Diagnostic{{Severity: graph.SeverityError, Message: "argument 2 of SetLogger is not set"}}},
			},
		}},
	}
}

// Every diagnostic is stored on the element it is about, so listing them is a
// walk rather than a list of its own. That is what stops the count a reader is
// shown disagreeing with what is drawn as broken.
func TestDiagnosticsAreWalkedOffTheElementsTheyAreAbout(t *testing.T) {
	t.Parallel()

	all := diagnosed().AllDiagnostics()
	require.Len(t, all, 5)

	require.Equal(t, "this graph names no schema", all[0].Message)
	require.Empty(t, all[0].Node, "a notice about the file names nothing in the graph")
	require.Empty(t, all[0].Scope)

	require.Equal(t, graph.ScopeID("root/plugins"), all[1].Scope)
	require.Empty(t, all[1].Node)

	require.Equal(t, "connection refused", all[2].Message)
	require.Equal(t, graph.NodeID("root/svc:app.(*Server)"), all[2].Node)
	require.Empty(t, all[2].Param, "what is wrong with the service is not wrong with an argument")
	require.Equal(t, "eager initialization", all[2].Pass)

	require.Equal(t, graph.ParamID("p0"), all[3].Param)
	require.Equal(t, graph.NodeID("root/svc:app.(*Server)"), all[3].Node, "an argument carries its node too")
	require.Equal(t, graph.ParamID("p2"), all[4].Param, "the arguments come in the order the node holds them")
}

func TestADiagnosticNamesWhatItIsAbout(t *testing.T) {
	t.Parallel()

	g := diagnosed()
	all := g.AllDiagnostics()

	require.Empty(t, all[0].Where(g), "one about the file itself is about nothing in the graph")
	require.Equal(t, "root/plugins", all[1].Where(g))
	require.Equal(t, "app.(*Server)", all[2].Where(g))
	require.Equal(t, "app.(*Server) argument 0", all[3].Where(g))
	require.Equal(t, "app.(*Server) argument 2 of SetLogger", all[4].Where(g),
		"the method matters: argument 2 alone means two different slots")
}

// A node is drawn as broken because something on it is broken. Asking rather than
// storing is what keeps the two answers the same.
func TestANodeIsFaultyWhenItOrItsArgumentsAre(t *testing.T) {
	t.Parallel()

	g := diagnosed()
	node := g.Nodes[0]

	require.True(t, node.Faulty())
	require.True(t, node.Params[0].Faulty())
	require.False(t, node.Params[1].Faulty())

	node.Diagnostics = nil
	require.True(t, node.Faulty(), "a broken argument breaks the node it belongs to")

	node.Params[0].Diagnostics = nil
	node.Params[2].Diagnostics = nil
	require.False(t, node.Faulty())

	node.Params[1].Diagnostics = []graph.Diagnostic{{Severity: graph.SeverityInfo, Message: "wired by hand"}}
	require.False(t, node.Faulty(), "something worth knowing is not something wrong")
}
