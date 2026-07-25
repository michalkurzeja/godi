package text_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/michalkurzeja/godi/v2/graph"
	"github.com/michalkurzeja/godi/v2/graph/text"
)

const pkg = "github.com/acme/app"

func encode(t *testing.T, g *graph.Graph, opts ...text.Option) string {
	t.Helper()

	var sb strings.Builder
	require.NoError(t, g.Encode(&sb, text.New(opts...)))
	return sb.String()
}

// lineFor returns the first line containing want, with its indentation intact,
// so a test can assert on structure rather than on the whole document.
func lineFor(t *testing.T, out, want string) string {
	t.Helper()

	for line := range strings.SplitSeq(out, "\n") {
		if strings.Contains(line, want) {
			return line
		}
	}
	t.Fatalf("no line containing %q in:\n%s", want, out)
	return ""
}

func model() *graph.Graph {
	serverRouter := &graph.Param{
		ID: "root/svc:app.(*Server)#f:0", Node: "root/svc:app.(*Server)",
		Kind: graph.InjectFactoryArg, Type: pkg + ".Router", Origin: graph.ArgOriginAutowiring,
		Arg: graph.ArgKindType, EdgeCount: 1,
	}
	serverAddr := &graph.Param{
		ID: "root/svc:app.(*Server)#f:1", Node: "root/svc:app.(*Server)", Index: 1,
		Kind: graph.InjectFactoryArg, Type: "string", Origin: graph.ArgOriginCompilerPass,
		OriginPass: "override arg", Arg: graph.ArgKindLiteral,
		Literals: []graph.Literal{{Type: "string", Value: "127.0.0.1:9090"}},
	}
	setLogger := &graph.Param{
		ID: "root/svc:app.(*Server)#m:SetLogger:1", Node: "root/svc:app.(*Server)", Index: 1,
		Kind: graph.InjectMethodArg, Method: "SetLogger", Type: pkg + ".Logger",
		Origin: graph.ArgOriginAutowiring, Arg: graph.ArgKindType, EdgeCount: 1,
	}
	plugins := &graph.Param{
		ID: "root/svc:app.(*Kernel)#f:0", Node: "root/svc:app.(*Kernel)",
		Kind: graph.InjectFactoryArg, Type: "[]" + pkg + ".Plugin", Slice: true, Variadic: true,
		Origin: graph.ArgOriginAutowiring, Arg: graph.ArgKindFlexibleSlice,
	}

	return &graph.Graph{
		Schema:     graph.Schema,
		SourceRoot: "/home/me/app",
		Scopes: []*graph.Scope{
			{ID: "root", Name: "root"},
			{ID: "root/svc:app.(*Server)", Parent: "root", Depth: 1, Name: "uuid-1", Owner: "root/svc:app.(*Server)"},
		},
		Nodes: []*graph.Node{
			{
				ID: "root/svc:app.(*Server)", Kind: graph.NodeService, Scope: "root",
				Type: pkg + ".(*Server)", Name: pkg + ".NewServer",
				Lazy: false, Shared: true, Autowired: true, Root: true, InDegree: 0, OutDegree: 2,
				Registered: graph.Location{File: "wiring.go", Line: 42},
				Defined:    graph.Location{File: "http/server.go", Line: 118},
				Params:     []*graph.Param{serverRouter, serverAddr, setLogger},
			},
			{
				ID: "root/svc:app.(*Kernel)", Kind: graph.NodeService, Scope: "root",
				Type: pkg + ".(*Kernel)", Name: pkg + ".NewKernel",
				Lazy: true, Shared: true, Autowired: true, Root: true,
				Params: []*graph.Param{plugins},
			},
			{
				ID: "root/svc:app.Router", Kind: graph.NodeService, Scope: "root",
				Type: pkg + ".Router", Name: pkg + ".NewRouter",
				Lazy: true, Shared: false, Autowired: true, InDegree: 1, Labels: []string{"http"},
			},
			{
				ID: "root/svc:app.ConsoleLogger", Kind: graph.NodeService, Scope: "root",
				Type: pkg + ".ConsoleLogger", Name: pkg + ".NewConsoleLogger",
				Lazy: true, Shared: true, Autowired: true, InDegree: 1,
			},
			{
				ID: "root/fun:app.migrate", Kind: graph.NodeFunction, Scope: "root",
				Type: "func() error", Name: pkg + ".migrate",
				Lazy: true, Autowired: true, Root: true, Labels: []string{"startup"},
			},
			{
				ID: "root/svc:app.(*Server)/svc:app.(*Conn)", Kind: graph.NodeService,
				Scope: "root/svc:app.(*Server)", Type: pkg + ".(*Conn)", Name: pkg + ".NewConn",
				Lazy: true, Shared: true, Autowired: true, Root: true,
			},
		},
		Edges: []*graph.Edge{
			{
				ID: "e0", From: "root/svc:app.(*Server)", To: "root/svc:app.Router",
				Param: serverRouter.ID, Kind: graph.InjectFactoryArg,
				Origin: graph.ArgOriginAutowiring, Resolution: graph.ResolutionByType,
				ParamType: pkg + ".Router",
			},
			{
				ID: "e1", From: "root/svc:app.(*Server)", To: "root/svc:app.ConsoleLogger",
				Param: setLogger.ID, Kind: graph.InjectMethodArg,
				Origin: graph.ArgOriginAutowiring, Resolution: graph.ResolutionByType,
				Bindings: []graph.BindingHop{{
					Interface: pkg + ".Logger", Scope: "root", Origin: graph.BindOriginAutobinding,
				}},
				ParamType: pkg + ".Logger",
			},
		},
		Bindings: []*graph.Binding{{
			Scope: "root", Interface: pkg + ".Logger", Origin: graph.BindOriginAutobinding,
			BoundTo: pkg + ".ConsoleLogger", Targets: []graph.NodeID{"root/svc:app.ConsoleLogger"},
			EdgeCount: 1,
		}},
	}
}

func TestFormat(t *testing.T) {
	t.Parallel()

	f := text.New().Format()

	require.Equal(t, "text", f.Name)
	require.Equal(t, "txt", f.Ext)
	require.Equal(t, "text/plain; charset=utf-8", f.MediaType)
}

func TestItOpensWithWhatIsInTheContainer(t *testing.T) {
	t.Parallel()

	out := encode(t, model())

	require.True(t, strings.HasPrefix(out, "5 services, 1 function, 2 dependencies, 4 roots\n"), out)
	require.Contains(t, out, "under /home/me/app")
}

func TestScopesNest(t *testing.T) {
	t.Parallel()

	out := encode(t, model())

	require.Equal(t, "scope root", lineFor(t, out, "scope root"))
	require.Equal(t, "  scope children of app.(*Server)", lineFor(t, out, "children of"),
		"a child scope is indented under the one that owns it")
	require.Equal(t, "      app.(*Conn)  [root]", lineFor(t, out, "(*Conn)"))
}

func TestAServiceIsKnownByItsTypeAndAFunctionByItsName(t *testing.T) {
	t.Parallel()

	out := encode(t, model())

	require.Contains(t, out, "app.(*Server)  [root, eager]")
	require.Contains(t, out, "factory: app.NewServer")
	require.Contains(t, out, "app.migrate  [root, startup]")
	require.Contains(t, out, "signature: func() error")
}

// Only the surprising half of each pair is worth printing: a lazy shared
// service is the default and says nothing about anyone's intent.
func TestOnlyTheFlagsSomeoneChoseArePrinted(t *testing.T) {
	t.Parallel()

	out := encode(t, model())

	require.Equal(t, "    app.Router  [not shared, http]", lineFor(t, out, "app.Router  [not"))
	require.Contains(t, out, "app.ConsoleLogger\n", "nothing was chosen, so nothing is said")
}

func TestAnArgumentSaysWhatArrivedAndWhoDecided(t *testing.T) {
	t.Parallel()

	out := encode(t, model())

	require.Contains(t, out, "0 <- app.Router  [autowiring]")
	require.Contains(t, out, "-> app.Router  [by-type]")
}

// A binding is the difference between "it asked for a Logger" and "it got the
// console one", so the row has to name it, and say who created it.
func TestABindingIsNamedOnTheRowThatWentThroughIt(t *testing.T) {
	t.Parallel()

	out := encode(t, model())

	require.Contains(t, out, "-> app.ConsoleLogger  [by-type, binding on app.Logger (autobinding)]")
	require.Contains(t, out, "app.Logger -> app.ConsoleLogger  [autobinding]")
}

func TestMethodCallsAreGroupedByMethod(t *testing.T) {
	t.Parallel()

	out := encode(t, model())

	require.Contains(t, out, "method calls:")
	require.Contains(t, out, "SetLogger():")
	require.Equal(t, "          1 <- app.Logger  [autowiring]", lineFor(t, out, "1 <- app.Logger"),
		"a method argument sits under its own method")
}

// A constant draws no edge, so its row is the only place the pass that
// substituted it can be credited.
func TestALiteralCarriesItsValueAndItsProvenance(t *testing.T) {
	t.Parallel()

	out := encode(t, model())

	require.Contains(t, out, "1 <- string = 127.0.0.1:9090  [compiler-pass: override arg]")
}

func TestAValueLessLiteralSaysOnlyThatThereIsOne(t *testing.T) {
	t.Parallel()

	g := model()
	g.Nodes[0].Params[1].Literals = []graph.Literal{{Type: "string"}}

	require.Contains(t, encode(t, g), "1 <- string = <literal string>")
}

func TestARedactedLiteralSaysSo(t *testing.T) {
	t.Parallel()

	g := model()
	g.Nodes[0].Params[1].Literals = []graph.Literal{{Type: "string", Redacted: true}}

	require.Contains(t, encode(t, g), "= <redacted>")
}

// An empty variadic slot is what an optional dependency looks like when nothing
// supplies one. It is not an error, and it is not the same as unresolved.
func TestASlotNothingFilledSaysWhichKindOfNothing(t *testing.T) {
	t.Parallel()

	g := model()
	require.Contains(t, encode(t, g), "0 <- []app.Plugin  (nothing)  [autowiring]")

	g.Nodes[1].Params[0].Origin = graph.ArgOriginNone
	require.Contains(t, encode(t, g), "(not wired)")

	g = model()
	g.Nodes[1].Params[0].Unresolved = true
	require.Contains(t, encode(t, g), "(unresolved)")
}

func TestAnUnusedBindingSaysSo(t *testing.T) {
	t.Parallel()

	g := model()
	g.Bindings[0].EdgeCount = 0

	require.Contains(t, encode(t, g), "(nothing uses it)")
}

func TestAFilteredNodeSaysWhatWasCutOff(t *testing.T) {
	t.Parallel()

	g := model()
	g.Nodes[0].Elided = 3

	require.Contains(t, encode(t, g), "... 3 neighbours were filtered out")
}

func TestNoticesAreReported(t *testing.T) {
	t.Parallel()

	g := model()
	g.Diagnostics = []*graph.Diagnostic{{Severity: "warning", Message: "scope \"orphan\" belongs to no definition"}}

	out := encode(t, g)

	require.Contains(t, out, "notices:")
	require.Contains(t, out, "warning: scope \"orphan\" belongs to no definition")
}

// Paths depend on the machine the binary was built on, so a comparison of the
// output needs a way to leave them out.
func TestLocationsCanBeLeftOut(t *testing.T) {
	t.Parallel()

	with := encode(t, model())
	require.Contains(t, with, "registered: wiring.go:42")
	require.Contains(t, with, "defined: http/server.go:118")

	without := encode(t, model(), text.WithoutLocations())
	require.NotContains(t, without, "wiring.go")
	require.NotContains(t, without, "under /home/me/app", "the root they hang off goes with them")
}

func TestLongTypesAreTruncated(t *testing.T) {
	t.Parallel()

	g := model()
	g.Nodes[0].Params[0].Type = pkg + ".Handler[github.com/acme/app.Request, github.com/acme/app.Response]"

	require.Contains(t, encode(t, g, text.MaxType(20)), "0 <- app.Handler[app.Req…")
	require.Contains(t, encode(t, g, text.MaxType(0)), "0 <- app.Handler[app.Request, app.Response]")
}

func TestAnEmptyGraphIsStillReadable(t *testing.T) {
	t.Parallel()

	out := encode(t, &graph.Graph{Schema: graph.Schema})

	require.Equal(t, "0 services, 0 functions, 0 dependencies, 0 roots\n", out)
}
