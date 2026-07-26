package html_test

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/michalkurzeja/godi/v2/graph"
	"github.com/michalkurzeja/godi/v2/graph/html"
)

// The viewer is where most of this package's behaviour lives, and none of it is
// reachable from Go: filters, labels, the detail panel and the hop limit are all
// decided in the browser. So the regression suite runs there, driven from here.
//
// It needs node and a Chrome, and skips when either is missing rather than
// failing a build that was never going to have them. Set
// GODI_REQUIRE_VIEWER_TESTS to turn those skips into failures, which is how CI
// asserts that the suite ran at all.
func TestViewerRegressions(t *testing.T) {
	node := toolOrSkip(t, "node")
	chrome := chromeOrSkip(t)

	dir := t.TempDir()
	page := filepath.Join(dir, "graph.html")

	var buf bytes.Buffer
	require.NoError(t, regressionModel().Encode(&buf, html.New(
		html.Title("regression"),
		html.SourceLink("test://open?file={file}&rel={rel}&line={line}"),
	)))
	require.NoError(t, os.WriteFile(page, buf.Bytes(), 0o600))

	// A second page, because a snapshot of a container still being built is a
	// different document, not a state the first page can be put into.
	snapshot := filepath.Join(dir, "snapshot.html")
	var snapBuf bytes.Buffer
	require.NoError(t, snapshotModel().Encode(&snapBuf, html.New(html.Title("snapshot"))))
	require.NoError(t, os.WriteFile(snapshot, snapBuf.Bytes(), 0o600))

	// A third, because the fixture above has one scope, and what a filtered
	// graph does to the scopes holding what it hid needs several.
	scopes := filepath.Join(dir, "scopes.html")
	var scopeBuf bytes.Buffer
	require.NoError(t, nestedScopesModel().Encode(&scopeBuf, html.New(html.Title("scopes"))))
	require.NoError(t, os.WriteFile(scopes, scopeBuf.Bytes(), 0o600))

	script := filepath.Join("testdata", "viewer_test.mjs")

	// Not under t.TempDir: Chrome goes on writing to its profile for a moment
	// after it is killed, and t.TempDir fails the test when what it removes is
	// not empty - so a suite that passed reported a red build. Cleaning it up is
	// ours to do, and best effort, because a stray profile is not worth that.
	profile, err := os.MkdirTemp("", "godi-viewer-profile-")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(profile) })

	//nolint:gosec // G204: the binaries are resolved above and the arguments are ours.
	cmd := exec.Command(node, script, page, chrome, profile, snapshot, scopes)
	cmd.Env = append(os.Environ(), "NODE_OPTIONS=--no-warnings")

	out, err := runWithTimeout(t, cmd, 3*time.Minute)
	results, notes := parseResults(out)

	if len(results) == 0 {
		t.Fatalf("the viewer suite produced no results: %v\n%s", err, out)
	}
	for _, r := range results {
		t.Run(r.Name, func(t *testing.T) {
			if !r.OK {
				t.Fatalf("%s\n\nsuite output:\n%s", r.Detail, notes)
			}
		})
	}
	require.NoError(t, err, "the viewer suite exited badly\n%s", out)
}

type result struct {
	Name   string `json:"name"`
	OK     bool   `json:"ok"`
	Detail string `json:"detail"`
}

// parseResults splits the suite's output into results and everything else,
// which is kept to report alongside a failure.
func parseResults(out string) ([]result, string) {
	var (
		results []result
		notes   strings.Builder
	)
	for line := range strings.SplitSeq(out, "\n") {
		var r result
		if strings.HasPrefix(line, "{") && json.Unmarshal([]byte(line), &r) == nil {
			results = append(results, r)
			continue
		}
		if strings.TrimSpace(line) != "" {
			notes.WriteString(line + "\n")
		}
	}
	return results, notes.String()
}

func runWithTimeout(t *testing.T, cmd *exec.Cmd, limit time.Duration) (string, error) {
	t.Helper()

	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out

	require.NoError(t, cmd.Start())

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	select {
	case err := <-done:
		return out.String(), err
	case <-time.After(limit):
		_ = cmd.Process.Kill()
		<-done
		return out.String(), nil
	}
}

// skipOrFailf skips, unless GODI_REQUIRE_VIEWER_TESTS says the suite was meant
// to run. Skipping quietly is right on a machine without a browser and wrong in
// CI, where a suite that did not run reads as one that passed.
func skipOrFailf(t *testing.T, format string, args ...any) {
	t.Helper()

	if on, _ := strconv.ParseBool(os.Getenv(envRequireViewerTests)); on {
		t.Fatalf(format, args...)
	}
	t.Skipf(format, args...)
}

const envRequireViewerTests = "GODI_REQUIRE_VIEWER_TESTS"

func toolOrSkip(t *testing.T, name string) string {
	t.Helper()

	path, err := exec.LookPath(name)
	if err != nil {
		skipOrFailf(t, "%s is not installed, so the viewer cannot be driven", name)
	}
	return path
}

func chromeOrSkip(t *testing.T) string {
	t.Helper()

	if path := os.Getenv("CHROME_PATH"); path != "" {
		return path
	}
	for _, path := range []string{
		"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
		"/Applications/Chromium.app/Contents/MacOS/Chromium",
	} {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	for _, name := range []string{"google-chrome", "chromium", "chromium-browser"} {
		if path, err := exec.LookPath(name); err == nil {
			return path
		}
	}

	skipOrFailf(t, "no Chrome found; set CHROME_PATH to run the viewer regression tests")
	return ""
}

// regressionModel is the fixture the viewer suite asserts against. Every case
// that has been reported broken needs something here to break again:
//
//   - a method call carrying a dependency, and another carrying a constant,
//     so hiding method calls has both an edge and rows to take away
//   - a constant, so its row can be checked for a doubled type
//   - a dependency autowired through a binding a compiler pass created, whose
//     colour and filter used to disagree
//   - a chain two hops deep, so the hop limit has something to cut
//   - a service nothing wires at all, which no filter may hide by accident,
//     and which is a root because nothing injects it
//
// The ids below are what testdata/viewer_test.mjs addresses, so they and the
// counts in it move together.
func regressionModel() *graph.Graph {
	const pkg = "github.com/acme/app"

	svc := func(name string, params ...*graph.Param) *graph.Node {
		return &graph.Node{
			ID: graph.NodeID("root/svc:app." + name), Kind: graph.NodeService, Scope: "root",
			Type: pkg + "." + name, Name: pkg + ".New" + strings.Trim(name, "(*)"),
			Shared: true, Lazy: true, Autowired: true,
			// Several qualified names, one of them a generic with a qualified
			// type argument: the shape that makes a full signature unreadable.
			Signature: "func(" + pkg + ".Handler[" + pkg + ".Request], app.Logger) " + pkg + "." + name,
			Params:    params,
		}
	}

	arg := func(node, id, typ string, origin graph.ArgOrigin) *graph.Param {
		return &graph.Param{
			ID: graph.ParamID(node + "#" + id), Node: graph.NodeID(node),
			Kind: graph.InjectFactoryArg, Type: typ, Origin: origin, Arg: graph.ArgKindType,
		}
	}

	const server = "root/svc:app.(*Server)"

	serverRouter := arg(server, "f:0", pkg+".(*Router)", graph.ArgOriginAutowiring)

	serverAddr := arg(server, "f:1", "string", graph.ArgOriginCompilerPass)
	serverAddr.Index = 1
	serverAddr.OriginPass = "override arg"
	serverAddr.Arg = graph.ArgKindLiteral
	serverAddr.Literals = []graph.Literal{{Type: "string", Value: "127.0.0.1:9090"}}

	setLogger := arg(server, "m:SetLogger:1", pkg+".Logger", graph.ArgOriginAutowiring)
	setLogger.Kind, setLogger.Method, setLogger.Index = graph.InjectMethodArg, "SetLogger", 1

	setTimeout := arg(server, "m:SetTimeout:1", "time.Duration", graph.ArgOriginManual)
	setTimeout.Kind, setTimeout.Method, setTimeout.Index = graph.InjectMethodArg, "SetTimeout", 1
	setTimeout.Arg = graph.ArgKindLiteral
	setTimeout.Literals = []graph.Literal{{Type: "time.Duration", Value: "30s"}}

	routerConfig := arg("root/svc:app.(*Router)", "f:0", pkg+".(*Config)", graph.ArgOriginAutowiring)
	auditorReporter := arg("root/svc:app.(*Auditor)", "f:0", pkg+".Reporter", graph.ArgOriginAutowiring)
	repoConfig := arg("root/svc:app.(*Repo)", "f:0", pkg+".(*Config)", graph.ArgOriginManual)
	repoConfig.Arg = graph.ArgKindRef

	edge := func(p *graph.Param, to string, origin graph.ArgOrigin, hops ...graph.BindingHop) *graph.Edge {
		p.EdgeCount++
		return &graph.Edge{
			ID: graph.NewEdgeID(p.ID, 0), From: p.Node, To: graph.NodeID(to), Param: p.ID,
			Kind: p.Kind, Origin: origin, OriginPass: p.OriginPass,
			Resolution: graph.ResolutionByType, Bindings: hops, ParamType: p.Type,
		}
	}

	nodes := []*graph.Node{
		svc("(*Server)", serverRouter, serverAddr, setLogger, setTimeout),
		svc("(*Router)", routerConfig),
		svc("(*Config)"),
		svc("ConsoleLogger"),
		svc("(*Auditor)", auditorReporter),
		svc("JSONReporter"),
		svc("(*Repo)", repoConfig),
		svc("(*Metrics)"), // Wired to nothing, reached by nothing.
	}
	nodes[0].Lazy = false // Eager, so it is a root.
	// A label of the reader's own, which is a different thing from the flags
	// godi puts on a definition and has to read as one.
	nodes[6].Labels = []string{"data"}
	nodes[0].Registered = graph.Location{File: "wiring.go", Line: 42, Func: "app.wire"}
	nodes[0].Defined = graph.Location{File: "http/server.go", Line: 118}

	g := &graph.Graph{
		Schema: graph.Schema,
		Scopes: []*graph.Scope{{ID: "root", Name: "root"}},
		Nodes:  nodes,
		Edges: []*graph.Edge{
			edge(serverRouter, "root/svc:app.(*Router)", graph.ArgOriginAutowiring),
			edge(setLogger, "root/svc:app.ConsoleLogger", graph.ArgOriginAutowiring,
				graph.BindingHop{Interface: pkg + ".Logger", Origin: graph.BindOriginAutobinding}),
			edge(routerConfig, "root/svc:app.(*Config)", graph.ArgOriginAutowiring),
			edge(auditorReporter, "root/svc:app.JSONReporter", graph.ArgOriginAutowiring,
				graph.BindingHop{Interface: pkg + ".Reporter", Origin: graph.BindOriginCompilerPass, OriginPass: "bind reporter"}),
			edge(repoConfig, "root/svc:app.(*Config)", graph.ArgOriginManual),
		},
		SourceRoot: "/home/me/app",
	}

	// What the extractor would work out: a root is a node nothing injects. Read
	// off the edges rather than written down, so it stays true as the fixture
	// grows.
	injected := make(map[graph.NodeID]bool, len(g.Edges))
	for _, edge := range g.Edges {
		injected[edge.To] = true
	}
	for _, node := range g.Nodes {
		node.Root = !injected[node.ID]
	}

	return g
}

// snapshotModel is the same container as it looks partway through compilation:
// one argument nobody has wired yet, and a graph that says why.
func snapshotModel() *graph.Graph {
	g := regressionModel()
	// Taken while validation runs: autowiring is done, so an argument nothing
	// filled is a fault rather than work outstanding.
	g.Snapshot = &graph.Snapshot{
		Stage: "validation", Pass: "argument validation",
		Done:      []string{"interface binding", "autowiring"},
		Autowired: true,
	}

	metrics, ok := g.Node("root/svc:app.(*Metrics)")
	if !ok {
		panic("the fixture no longer has the node this snapshot leaves unwired")
	}
	metrics.Params = []*graph.Param{{
		ID:   graph.ParamID(metrics.ID + "#f:0"),
		Node: metrics.ID,
		Kind: graph.InjectFactoryArg,
		Type: "github.com/acme/app.(*Config)",
		// What an unfilled slot looks like: nothing has decided anything yet.
		Origin: graph.ArgOriginNone,
		Arg:    graph.ArgKindNone,
		Note:   "argument is not wired",
	}}
	// Both set by the extractor from the argument above; written down here
	// because this fixture is built by hand. The notice the page lists for it is
	// not: the graph reads that off the parameter, which is what keeps the count
	// and the red borders from disagreeing.
	metrics.Incomplete = true

	// About the graph rather than the container, with no node to be marked on -
	// and not a fault, since a compiler pass is entitled to make a scope. Only
	// the page's own list carries this kind.
	g.GraphDiagnostics = []*graph.Diagnostic{
		{Severity: graph.SeverityInfo, Scope: "root/jobs", Message: `scope "root/jobs" belongs to no definition`},
	}

	return g
}

// nestedScopesModel is a container whose services sit in scopes inside scopes.
//
// What it is for: isolating the selection below leaves whole scopes with nothing
// on show. Cytoscape stops drawing such a scope, so it looks gone, but it goes
// on counting towards the box of the scope holding it - from wherever the layout
// of the whole container left it. On a real graph that stretched the root box
// down over a screenful of empty space.
//
// Selecting app.(*Worker) reaches app.(*Queue) and nothing else, so both child
// scopes empty out and the root box has to close up around the two survivors.
func nestedScopesModel() *graph.Graph {
	const pkg = "github.com/acme/app"

	const (
		outer = "root/svc:app.(*Server)"
		inner = "root/svc:app.(*Server)/svc:app.(*Router)"
	)

	svc := func(id, name, scope string, params ...*graph.Param) *graph.Node {
		return &graph.Node{
			ID: graph.NodeID(id), Kind: graph.NodeService, Scope: graph.ScopeID(scope),
			Type: pkg + "." + name, Name: pkg + ".New" + strings.Trim(name, "(*)"),
			Shared: true, Lazy: true, Autowired: true, Params: params,
		}
	}

	arg := func(node, id, typ string) *graph.Param {
		return &graph.Param{
			ID: graph.ParamID(node + "#" + id), Node: graph.NodeID(node),
			Kind: graph.InjectFactoryArg, Type: typ, Origin: graph.ArgOriginAutowiring,
			Arg: graph.ArgKindType,
		}
	}

	workerQueue := arg("root/svc:app.(*Worker)", "f:0", pkg+".(*Queue)")
	serverRouter := arg(outer, "f:0", pkg+".(*Router)")
	handlerStore := arg(inner+"/svc:app.(*Handler)", "f:0", pkg+".(*Store)")

	edge := func(p *graph.Param, to string) *graph.Edge {
		p.EdgeCount++
		return &graph.Edge{
			ID: graph.NewEdgeID(p.ID, 0), From: p.Node, To: graph.NodeID(to), Param: p.ID,
			Kind: p.Kind, Origin: p.Origin, Resolution: graph.ResolutionByType, ParamType: p.Type,
		}
	}

	g := &graph.Graph{
		Schema: graph.Schema,
		Scopes: []*graph.Scope{
			{ID: "root", Name: "root"},
			{ID: outer, Parent: "root", Depth: 1, Name: "outer",
				Owner: outer, OwnerName: pkg + ".(*Server)"},
			{ID: inner, Parent: outer, Depth: 2, Name: "inner",
				Owner: graph.NodeID(outer + "/svc:app.(*Router)"), OwnerName: pkg + ".(*Router)"},
		},
		Nodes: []*graph.Node{
			// The pair the selection keeps, both in the root scope.
			svc("root/svc:app.(*Worker)", "(*Worker)", "root", workerQueue),
			svc("root/svc:app.(*Queue)", "(*Queue)", "root"),
			// Everything below is in a nested scope and drops out of an isolated
			// selection, emptying both of them.
			svc(outer, "(*Server)", "root", serverRouter),
			svc(outer+"/svc:app.(*Router)", "(*Router)", outer),
			svc(inner+"/svc:app.(*Handler)", "(*Handler)", inner, handlerStore),
			svc(inner+"/svc:app.(*Store)", "(*Store)", inner),
		},
	}
	g.Edges = []*graph.Edge{
		edge(workerQueue, "root/svc:app.(*Queue)"),
		edge(serverRouter, outer+"/svc:app.(*Router)"),
		edge(handlerStore, inner+"/svc:app.(*Store)"),
	}

	injected := make(map[graph.NodeID]bool, len(g.Edges))
	for _, e := range g.Edges {
		injected[e.To] = true
	}
	for _, node := range g.Nodes {
		node.Root = !injected[node.ID]
	}

	return g
}
