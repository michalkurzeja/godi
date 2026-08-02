package graph_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/michalkurzeja/godi/v2/graph"
)

// What is wrong with the wiring and what is odd about the graph are two
// different pieces of news, and only the first is anybody's fault.
func TestWiringFaultsAreReadOffTheParameters(t *testing.T) {
	t.Parallel()

	g := &graph.Graph{
		Nodes: []*graph.Node{{
			ID: "root/svc:app.(*Server)",
			Params: []*graph.Param{
				{ID: "p0", Node: "root/svc:app.(*Server)", Unresolved: true, Note: "no services of type app.(*Store)"},
				{ID: "p1", Node: "root/svc:app.(*Server)", Origin: graph.ArgOriginManual},
				{ID: "p2", Node: "root/svc:app.(*Server)", Origin: graph.ArgOriginNone, Note: "argument 2 is not set"},
			},
		}},
		GraphDiagnostics: []*graph.Diagnostic{
			{Severity: graph.SeverityInfo, Scope: "root/plugins", Message: `scope "plugins" belongs to no definition`},
		},
	}

	faults := g.WiringDiagnostics()
	require.Len(t, faults, 2)
	require.Equal(t, "no services of type app.(*Store)", faults[0].Message)
	require.Equal(t, graph.ParamID("p0"), faults[0].Param)
	require.Equal(t, "argument 2 is not set", faults[1].Message)
	require.Equal(t, graph.SeverityWarning, faults[1].Severity)

	all := g.AllDiagnostics()
	require.Len(t, all, 3)
	require.Equal(t, faults[0], all[0], "the wiring comes first: it is what someone has to fix")
	require.Equal(t, graph.SeverityInfo, all[2].Severity)
}

// Before autowiring has run every argument is unwired, so an unfilled one is work
// outstanding rather than a fault. It is the same gate that decides whether a node
// is drawn as incomplete.
func TestAnUnwiredArgumentIsNoFaultUntilAutowiringHasRun(t *testing.T) {
	t.Parallel()

	unwired := func(snapshot *graph.Snapshot) *graph.Graph {
		return &graph.Graph{
			Snapshot: snapshot,
			Nodes: []*graph.Node{{
				ID:     "root/svc:app.(*Server)",
				Params: []*graph.Param{{ID: "p0", Origin: graph.ArgOriginNone, Note: "argument 0 is not set"}},
			}},
		}
	}

	require.Empty(t, unwired(&graph.Snapshot{Pass: "autowiring"}).WiringDiagnostics())
	require.Len(t, unwired(&graph.Snapshot{Pass: "validation", Autowired: true}).WiringDiagnostics(), 1)
	require.Len(t, unwired(nil).WiringDiagnostics(), 1, "a built container is past autowiring by definition")

	g := unwired(nil)
	g.Nodes[0].Params[0].Variadic = true
	require.Empty(t, g.WiringDiagnostics(), "a variadic slot takes no arguments just as happily")
}
