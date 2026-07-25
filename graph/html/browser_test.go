package html_test

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
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
// failing a build that was never going to have them.
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

	script := filepath.Join("testdata", "viewer_test.mjs")
	profile := filepath.Join(dir, "profile")

	//nolint:gosec // G204: the binaries are resolved above and the arguments are ours.
	cmd := exec.Command(node, script, page, chrome, profile, snapshot)
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

func toolOrSkip(t *testing.T, name string) string {
	t.Helper()

	path, err := exec.LookPath(name)
	if err != nil {
		t.Skipf("%s is not installed, so the viewer cannot be driven", name)
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

	t.Skip("no Chrome found; set CHROME_PATH to run the viewer regression tests")
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
	g.Snapshot = &graph.Snapshot{
		Stage: "automation", Pass: "autowiring",
		Done: []string{"interface binding"},
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
	}}
	// Set by the extractor from the argument above; written down here because
	// this fixture is built by hand.
	metrics.Incomplete = true

	return g
}
