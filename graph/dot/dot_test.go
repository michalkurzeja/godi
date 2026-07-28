package dot_test

import (
	"bytes"
	"os/exec"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/michalkurzeja/godi/v2/graph"
	"github.com/michalkurzeja/godi/v2/graph/dot"
)

// encode is the whole pipeline under test: a model in, DOT out.
func encode(t *testing.T, g *graph.Graph, opts ...dot.Option) string {
	t.Helper()

	var buf bytes.Buffer
	require.NoError(t, g.Encode(&buf, dot.New(opts...)))
	return buf.String()
}

// modelWith builds a one-service graph whose single argument carries the given
// provenance, which is all most of these tests need.
func modelWith(edge *graph.Edge, params ...*graph.Param) *graph.Graph {
	consumer := &graph.Node{
		ID: "root/svc:app.(*Consumer)", Kind: graph.NodeService, Scope: "root",
		Type: "github.com/acme/app.(*Consumer)", Name: "github.com/acme/app.NewConsumer",
		Shared: true, Lazy: true, Autowired: true, Params: params,
	}
	dep := &graph.Node{
		ID: "root/svc:app.(*Dep)", Kind: graph.NodeService, Scope: "root",
		Type: "github.com/acme/app.(*Dep)", Name: "github.com/acme/app.NewDep",
		Shared: true, Lazy: true, Autowired: true}

	g := &graph.Graph{
		Schema: graph.Schema,
		Scopes: []*graph.Scope{{ID: "root", Name: "root"}},
		Nodes:  []*graph.Node{consumer, dep},
	}
	if edge != nil {
		g.Edges = []*graph.Edge{edge}
	}
	return g
}

func param(origin graph.ArgOrigin, pass string) *graph.Param {
	return &graph.Param{
		ID: "root/svc:app.(*Consumer)#f:0", Node: "root/svc:app.(*Consumer)",
		Kind: graph.InjectFactoryArg, Index: 0,
		Type: "github.com/acme/app.(*Dep)", Origin: origin, OriginPass: pass,
		Arg: graph.ArgKindType, EdgeCount: 1,
	}
}

func edge(origin graph.ArgOrigin, pass string, hops ...graph.BindingHop) *graph.Edge {
	return &graph.Edge{
		From: "root/svc:app.(*Consumer)", To: "root/svc:app.(*Dep)",
		Param: "root/svc:app.(*Consumer)#f:0", Kind: graph.InjectFactoryArg,
		Origin: origin, OriginPass: pass, Resolution: graph.ResolutionByType,
		Bindings: hops, ParamType: "github.com/acme/app.(*Dep)",
	}
}

// edgeLine returns the single dependency edge line, ignoring the preamble.
func edgeLine(t *testing.T, out string) string {
	t.Helper()

	for line := range strings.SplitSeq(out, "\n") {
		if strings.Contains(line, "->") {
			return line
		}
	}
	t.Fatal("no edge in output")
	return ""
}

func TestProvenanceIsDrawnWithoutRelyingOnColour(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		edge     *graph.Edge
		style    string // Who wired it.
		arrow    string // How it resolved.
		wantText string
	}{
		{
			name:  "the user wired it",
			edge:  edge(graph.ArgOriginManual, ""),
			style: "solid", arrow: "normal",
		},
		{
			name:  "autowired by type",
			edge:  edge(graph.ArgOriginAutowiring, "autowiring"),
			style: "dashed", arrow: "normal",
		},
		{
			name: "autowired through a binding godi created",
			edge: edge(graph.ArgOriginAutowiring, "autowiring", graph.BindingHop{
				Interface: "github.com/acme/app.Iface", Scope: "root",
				Origin: graph.BindOriginAutobinding, OriginPass: "interface binding",
			}),
			style: "dashed", arrow: "odiamond",
		},
		{
			name: "resolved through a binding the user declared",
			edge: edge(graph.ArgOriginManual, "", graph.BindingHop{
				Interface: "github.com/acme/app.Iface", Scope: "root",
				Origin: graph.BindOriginManual,
			}),
			style: "solid", arrow: "odiamond",
		},
		{
			name:     "a compiler pass wired it",
			edge:     edge(graph.ArgOriginCompilerPass, "override arg"),
			style:    "dashed",
			arrow:    "normal",
			wantText: "override arg", // Named on the edge, so it reads off the picture.
		},
		{
			name: "a binding a compiler pass created",
			edge: edge(graph.ArgOriginAutowiring, "autowiring", graph.BindingHop{
				Interface: "github.com/acme/app.Iface", Scope: "root",
				Origin: graph.BindOriginCompilerPass, OriginPass: "my pass",
			}),
			style: "dashed", arrow: "odiamond",
			wantText: `label="my pass"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			line := edgeLine(t, encode(t, modelWith(tt.edge, param(tt.edge.Origin, tt.edge.OriginPass))))

			require.Contains(t, line, `style="`+tt.style+`"`)
			require.Contains(t, line, "arrowhead="+tt.arrow)
			if tt.wantText != "" {
				require.Contains(t, line, tt.wantText)
			}
		})
	}
}

// The head says how the dependency was matched. The colour says who chose it.
// Keeping them independent is what lets a reader tell "godi picked the
// dependency" from "you did" at a glance.
func TestHeadSaysHowAndColourSaysWho(t *testing.T) {
	t.Parallel()

	const (
		white  = `color="#1f2328"`
		teal   = `color="#0f766e"`
		purple = `color="#8250df"`
	)

	tests := []struct {
		name         string
		edge         *graph.Edge
		arrow, color string
	}{
		{
			name:  "exact type, you chose it",
			edge:  edge(graph.ArgOriginManual, ""),
			arrow: "normal", color: white,
		},
		{
			name:  "exact type, godi chose it",
			edge:  edge(graph.ArgOriginAutowiring, "autowiring"),
			arrow: "normal", color: teal,
		},
		{
			name:  "exact type, a pass chose it",
			edge:  edge(graph.ArgOriginCompilerPass, "override arg"),
			arrow: "normal", color: purple,
		},
		{
			// The argument was autowired, but the target came from the binding
			// the user declared, so the colour follows the binding.
			name: "interface binding you declared",
			edge: edge(graph.ArgOriginAutowiring, "autowiring",
				graph.BindingHop{Origin: graph.BindOriginManual}),
			arrow: "odiamond", color: white,
		},
		{
			name: "interface binding godi created",
			edge: edge(graph.ArgOriginAutowiring, "autowiring",
				graph.BindingHop{Origin: graph.BindOriginAutobinding}),
			arrow: "odiamond", color: teal,
		},
		{
			name: "interface binding a pass created",
			edge: edge(graph.ArgOriginAutowiring, "autowiring",
				graph.BindingHop{Origin: graph.BindOriginCompilerPass, OriginPass: "p"}),
			arrow: "odiamond", color: purple,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			line := edgeLine(t, encode(t, modelWith(tt.edge, param(tt.edge.Origin, tt.edge.OriginPass))))
			require.Contains(t, line, "arrowhead="+tt.arrow)
			require.Contains(t, line, tt.color)
		})
	}
}

func TestEveryProvenanceGetsADistinctAppearance(t *testing.T) {
	t.Parallel()

	// The six combinations must not collapse onto each other, or the picture
	// would claim two different things were wired the same way.
	combos := []*graph.Edge{
		edge(graph.ArgOriginManual, ""),
		edge(graph.ArgOriginAutowiring, "autowiring"),
		edge(graph.ArgOriginCompilerPass, "p"),
		edge(graph.ArgOriginManual, "", graph.BindingHop{Origin: graph.BindOriginManual}),
		edge(graph.ArgOriginAutowiring, "autowiring", graph.BindingHop{Origin: graph.BindOriginAutobinding}),
		edge(graph.ArgOriginAutowiring, "autowiring", graph.BindingHop{Origin: graph.BindOriginCompilerPass, OriginPass: "p"}),
	}

	seen := make(map[string]bool, len(combos))
	for _, e := range combos {
		line := edgeLine(t, encode(t, modelWith(e, param(e.Origin, e.OriginPass))))

		// All four channels together, since no single one separates every case:
		// the head says how it was matched, the colour who chose it.
		key := strings.Join([]string{
			between(line, `style="`, `"`),
			between(line, "arrowhead=", ","),
			between(line, `color="`, `"`),
			between(line, "penwidth=", ","),
		}, "/")

		require.False(t, seen[key], "two provenances look identical: %s", key)
		seen[key] = true
	}
}

func between(s, start, end string) string {
	_, after, ok := strings.Cut(s, start)
	if !ok {
		return ""
	}
	before, _, _ := strings.Cut(after, end)
	return before
}

func TestEscaping(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		typ     string
		literal string
		absent  string
		present string
	}{
		{
			name:    "a directional channel is not read as markup",
			typ:     "chan<- int",
			present: "chan&lt;- int",
		},
		{
			name:    "a generic instantiation survives",
			typ:     "github.com/acme/app.Cache[string,int]",
			present: "Cache[string,int]",
		},
		{
			name:    "an empty interface survives",
			typ:     "interface {}",
			present: "interface {}",
		},
		{
			// Graphviz substitutes \N with the node name inside a label, so a
			// backslash must be escaped before anything else.
			name:    `a literal containing \N is not substituted`,
			typ:     "string",
			literal: `C:\Notes`,
			present: `C:\\Notes`,
		},
		{
			name:    "a literal containing a quote does not end the string",
			typ:     "string",
			literal: `say "hi"`,
			present: `&#34;hi&#34;`, // The form html.EscapeString emits; Graphviz takes either.
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			p := &graph.Param{
				ID: "root/svc:app.(*Consumer)#f:0", Node: "root/svc:app.(*Consumer)",
				Kind: graph.InjectFactoryArg, Index: 0, Type: tt.typ, Origin: graph.ArgOriginManual,
				Arg: graph.ArgKindType,
			}
			if tt.literal != "" {
				p.Arg = graph.ArgKindLiteral
				p.Literals = []graph.Literal{{Type: "string", Value: tt.literal}}
			}

			out := encode(t, modelWith(nil, p))
			require.Contains(t, out, tt.present)
			requireBalancedQuotes(t, out)
		})
	}
}

// requireBalancedQuotes is a cheap structural check: an unescaped quote leaking
// out of a label would break every line after it.
func requireBalancedQuotes(t *testing.T, out string) {
	t.Helper()

	for line := range strings.SplitSeq(out, "\n") {
		var count int
		for i := 0; i < len(line); i++ {
			switch line[i] {
			case '\\':
				i++ // Skip whatever it escapes.
			case '"':
				count++
			}
		}
		require.Zero(t, count%2, "unbalanced quotes in: %s", line)
	}
}

func TestScopesBecomeNestedClusters(t *testing.T) {
	t.Parallel()

	g := &graph.Graph{
		Schema: graph.Schema,
		Scopes: []*graph.Scope{
			{ID: "root", Name: "root"},
			{
				ID: "root/svc:app.(*Server)", Parent: "root", Depth: 1, Name: "uuid-here",
				Owner: "root/svc:app.(*Server)", OwnerName: "github.com/acme/app.(*Server)",
			},
		},
		Nodes: []*graph.Node{
			{ID: "root/svc:app.(*Server)", Kind: graph.NodeService, Scope: "root",
				Type: "github.com/acme/app.(*Server)", ChildScope: "root/svc:app.(*Server)"},
			{ID: "root/svc:app.(*Server)/svc:app.(*Conn)", Kind: graph.NodeService, Scope: "root/svc:app.(*Server)",
				Type: "github.com/acme/app.(*Conn)"},
		},
	}

	out := encode(t, g)

	require.Contains(t, out, `subgraph "cluster_root"`)
	require.Contains(t, out, `subgraph "cluster_root/svc:app.(*Server)"`)
	// Named after what declared it, not after the uuid the container uses.
	require.Contains(t, out, `label="children of app.(*Server)"`)
	require.NotContains(t, out, "uuid-here")

	inner := strings.Index(out, `cluster_root/svc:app.(*Server)`)
	conn := strings.Index(out, "app.(*Conn)")
	require.Less(t, inner, conn, "the private service sits inside its owner's cluster")
}

func TestRootsAreDistinguishable(t *testing.T) {
	t.Parallel()

	g := &graph.Graph{
		Schema: graph.Schema,
		Scopes: []*graph.Scope{{ID: "root", Name: "root"}},
		Nodes: []*graph.Node{{
			ID: "root/svc:app.(*Orphan)", Kind: graph.NodeService, Scope: "root",
			Type: "github.com/acme/app.(*Orphan)", Root: true,
		}},
	}

	out := encode(t, g)
	require.Contains(t, out, `class="service root"`)
	require.Contains(t, out, `fillcolor="#dbe9fc"`, "a tint, not a warning colour")
	require.Contains(t, out, "▲ ", "and a marker on the node itself")
	require.NotContains(t, out, "⚠", "a root is not a problem")
	require.Contains(t, out, "nothing injects this: it is the top of a tree")
}

// A filtered graph stops somewhere the wiring does not. Saying so is what keeps
// the picture from reading as the whole story.
func TestAFilteredNodeSaysWhatWasCutOff(t *testing.T) {
	t.Parallel()

	g := &graph.Graph{
		Schema: graph.Schema,
		Scopes: []*graph.Scope{{ID: "root", Name: "root"}},
		Nodes: []*graph.Node{{
			ID: "root/svc:app.(*Server)", Kind: graph.NodeService, Scope: "root",
			Type: "github.com/acme/app.(*Server)", Elided: 7,
		}},
	}

	out := encode(t, g)
	require.Contains(t, out, "⋯ +7 more")
	require.Contains(t, out, "7 neighbours were filtered out")
}

func TestANodeWithNothingCutOffSaysNothing(t *testing.T) {
	t.Parallel()

	out := encode(t, modelWith(nil, param(graph.ArgOriginManual, "")))

	require.NotContains(t, out, "more")
	require.NotContains(t, out, "filtered out")
}

func TestPortsCanBeTurnedOff(t *testing.T) {
	t.Parallel()

	g := modelWith(edge(graph.ArgOriginManual, ""), param(graph.ArgOriginManual, ""))

	require.Contains(t, encode(t, g, dot.Ports(dot.PortsOn)), `PORT="f0"`)
	require.NotContains(t, encode(t, g, dot.Ports(dot.PortsOff)), `PORT="f0"`)
	require.Contains(t, edgeLine(t, encode(t, g, dot.Ports(dot.PortsOn))), ":f0:w")
}

func TestThemesDifferAndBothSetABackground(t *testing.T) {
	t.Parallel()

	g := modelWith(nil, param(graph.ArgOriginManual, ""))

	light := encode(t, g, dot.Theme(dot.Light))
	dark := encode(t, g, dot.Theme(dot.Dark))

	// A single Graphviz SVG cannot suit both backgrounds: the colours are baked
	// in, so each theme must state its own.
	require.Contains(t, light, `bgcolor="#ffffff"`)
	require.Contains(t, dark, `bgcolor="#0d1117"`)
	require.NotEqual(t, light, dark)
}

// Cluster labels take the graph font colour, not the node one. That covers the scope
// names and the key's own caption. Leaving it unset renders them black, which is
// unreadable on a dark background.
func TestClusterLabelsFollowTheTheme(t *testing.T) {
	t.Parallel()

	g := &graph.Graph{
		Schema: graph.Schema,
		Scopes: []*graph.Scope{{ID: "root", Name: "root"}},
		Nodes: []*graph.Node{{
			ID: "root/svc:app.(*A)", Kind: graph.NodeService, Scope: "root",
			Type: "github.com/acme/app.(*A)"}},
	}

	for _, tc := range []struct {
		theme dot.ThemeName
		text  string
	}{
		{dot.Light, "#1f2328"},
		{dot.Dark, "#e6edf3"},
	} {
		out := encode(t, g, dot.Theme(tc.theme))

		// Attribute lists wrap across lines, so scan statements, not lines.
		var checked int
		for stmt := range strings.SplitSeq(out, "];") {
			if strings.Contains(stmt, `label="scope: root"`) || strings.Contains(stmt, `label="legend"`) {
				checked++
				require.Contains(t, stmt, `fontcolor="`+tc.text+`"`,
					"cluster label must state its own colour: %s", stmt)
			}
		}
		require.Equal(t, 2, checked, "expected the scope and the legend cluster")
	}
}

// The key exists to show the arrowheads. An earlier version described them in
// prose, which leaves the reader guessing what a diamond means.
func TestLegendDrawsRealSampleEdges(t *testing.T) {
	t.Parallel()

	out := encode(t, modelWith(nil, param(graph.ArgOriginManual, "")))

	var samples []string
	for line := range strings.SplitSeq(out, "\n") {
		if strings.Contains(line, "legend_") && strings.Contains(line, "->") &&
			!strings.Contains(line, "style=invis") {
			samples = append(samples, line)
		}
	}
	require.Len(t, samples, 5, "two rows for the head, three for the colour")

	joined := strings.Join(samples, "\n")
	for _, head := range []string{"normal", "odiamond"} {
		require.Contains(t, joined, "arrowhead="+head)
	}
	for _, colour := range []string{"#1f2328", "#0f766e", "#8250df"} {
		require.Contains(t, joined, `color="`+colour+`"`)
	}

	// The samples must be level and the same length, or the key reads as though
	// the differences between rows were meaningful.
	require.Contains(t, joined, "weight=100")
	require.Contains(t, out, "minlen=0")

	// Ordering is not left to the layout: the rows would otherwise come out
	// bottom to top.
	require.Contains(t, out, "{ rank=same; ")
	require.Contains(t, out, "[style=invis, minlen=0, weight=100]; }")

	// The key gets the first columns to itself. Sharing them with the graph
	// stretches every sample arrow to the width of the widest service table.
	require.Contains(t, out, "legend_rank_pad -> ")
	require.Contains(t, out, "[style=invis, minlen=2]")
}

// TestGraphvizAcceptsTheOutput is the only test that needs the real tool, so it
// skips where Graphviz is absent - CI has none.
func TestGraphvizAcceptsTheOutput(t *testing.T) {
	t.Parallel()

	if _, err := exec.LookPath("dot"); err != nil {
		t.Skip("graphviz not installed")
	}

	g := modelWith(
		edge(graph.ArgOriginAutowiring, "autowiring", graph.BindingHop{
			Interface: "github.com/acme/app.Iface", Origin: graph.BindOriginAutobinding,
		}),
		&graph.Param{
			ID: "root/svc:app.(*Consumer)#f:0", Node: "root/svc:app.(*Consumer)",
			Kind: graph.InjectFactoryArg, Index: 0, Type: "chan<- map[string]interface {}",
			Origin: graph.ArgOriginManual, Arg: graph.ArgKindLiteral, EdgeCount: 1,
			Literals: []graph.Literal{{Type: "string", Value: `weird \N "value"`}},
		},
	)
	// Everything the encoder can emit, so Graphviz vets all of it: a filtered
	// node's extra row and the notices cluster included.
	g.Nodes[0].Elided = 4
	g.GraphDiagnostics = []*graph.Diagnostic{
		{Severity: graph.SeverityInfo, Message: `scope "orphan" belongs to no definition`},
		{Severity: graph.SeverityWarning, Message: "chan<- int could not be resolved"},
	}
	g.Snapshot = &graph.Snapshot{Stage: "automation", Pass: "autowiring", Done: []string{"interface binding"}}

	cmd := exec.Command("dot", "-Tsvg")
	cmd.Stdin = strings.NewReader(encode(t, g))
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	out, err := cmd.Output()
	require.NoError(t, err, "graphviz rejected the output: %s", stderr.String())
	require.Empty(t, stderr.String(), "graphviz warned about the output")
	require.Contains(t, string(out), "<svg")
	require.Contains(t, string(out), "notices", "the cluster made it into the drawing")
	require.Contains(t, string(out), "+4 more")
}

// The head already says whether a pass wired the argument or created the
// binding, so naming it twice ("bind: bind reporter") only adds noise.
func TestPassNameIsQualifiedOnlyWhenAmbiguous(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		edge *graph.Edge
		want string
	}{
		{
			name: "a pass wired the argument",
			edge: edge(graph.ArgOriginCompilerPass, "override arg"),
			want: `label="override arg"`,
		},
		{
			name: "a pass created the binding",
			edge: edge(graph.ArgOriginAutowiring, "autowiring",
				graph.BindingHop{Origin: graph.BindOriginCompilerPass, OriginPass: "bind reporter"}),
			want: `label="bind reporter"`,
		},
		{
			name: "the same pass did both",
			edge: edge(graph.ArgOriginCompilerPass, "rewire",
				graph.BindingHop{Origin: graph.BindOriginCompilerPass, OriginPass: "rewire"}),
			want: `label="rewire"`,
		},
		{
			// Only here is there anything to disambiguate.
			name: "two different passes",
			edge: edge(graph.ArgOriginCompilerPass, "override arg",
				graph.BindingHop{Origin: graph.BindOriginCompilerPass, OriginPass: "bind reporter"}),
			want: `label="arg: override arg, bind: bind reporter"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			line := edgeLine(t, encode(t, modelWith(tt.edge, param(tt.edge.Origin, tt.edge.OriginPass))))
			require.Contains(t, line, tt.want)
		})
	}
}

// A location is too long to put in a node box, so it lives in the tooltip where
// it costs nothing to carry.
func TestSourceLocationsAreInTheTooltip(t *testing.T) {
	g := modelWith(nil)
	g.SourceRoot = "/home/me/app"
	g.Nodes[0].Registered = graph.Location{File: "wiring.go", Line: 42, Func: "app.wire"}
	g.Nodes[0].Declared = graph.Location{File: "http/server.go", Line: 118}

	out := encode(t, g)

	require.Contains(t, out, `registered: wiring.go:42`)
	require.Contains(t, out, `declared: http/server.go:118`)
}

// A node godi could find no source for must not grow an empty row.
func TestAnUnknownLocationIsLeftOut(t *testing.T) {
	out := encode(t, modelWith(nil))

	require.NotContains(t, out, "registered:")
	require.NotContains(t, out, "declared:")
}

// Extraction never fails on odd input. It records it, and a drawing that leaves that
// out is the last place a reader would think to look.
func TestNoticesAreDrawn(t *testing.T) {
	t.Parallel()

	g := modelWith(nil, param(graph.ArgOriginManual, ""))
	g.GraphDiagnostics = []*graph.Diagnostic{
		{Severity: graph.SeverityInfo, Message: `scope "orphan" belongs to no definition`},
		{Severity: graph.SeverityWarning, Message: "chan<- int could not be resolved"},
	}

	out := encode(t, g)

	require.Contains(t, out, "cluster_notices")
	require.Contains(t, out, "scope &#34;orphan&#34; belongs to no definition", "quoted, for an HTML-like label")
	require.Contains(t, out, "chan&lt;- int could not be resolved", "and so is the type syntax")
	require.Contains(t, out, `<BR ALIGN="LEFT"/>`, "a newline is only whitespace in an HTML-like label")
}

func TestAGraphWithNothingToReportDrawsNoNotices(t *testing.T) {
	t.Parallel()

	require.NotContains(t, encode(t, modelWith(nil, param(graph.ArgOriginManual, ""))), "cluster_notices")
}

// A drawing of half-finished wiring is indistinguishable from a drawing of
// finished wiring with pieces missing, so it has to say which it is.
func TestASnapshotIsDrawnAmongTheNotices(t *testing.T) {
	t.Parallel()

	g := modelWith(nil, param(graph.ArgOriginManual, ""))
	g.Snapshot = &graph.Snapshot{
		Stage: "pre-validation", Pass: "graph snapshot",
		Done: []string{"interface binding", "autowiring"},
	}

	out := encode(t, g)

	require.Contains(t, out, "cluster_notices")
	require.Contains(t, out, "snapshot: taken during the graph snapshot pass")
	require.Contains(t, out, "passes run: interface binding, autowiring")
}
