package html_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/michalkurzeja/godi/v2/graph"
	"github.com/michalkurzeja/godi/v2/graph/html"
)

func encode(t *testing.T, g *graph.Graph, opts ...html.Option) string {
	t.Helper()

	var buf bytes.Buffer
	require.NoError(t, g.Encode(&buf, html.New(opts...)))
	return buf.String()
}

// model is a two-service graph: one dependency, hand-wired, plus a service
// nothing reaches.
func model() *graph.Graph {
	param := &graph.Param{
		ID: "root/svc:app.(*Consumer)#f:0", Node: "root/svc:app.(*Consumer)",
		Kind: graph.InjectFactoryArg, Index: 0,
		Type: "github.com/acme/app.(*Dep)", Origin: graph.ArgOriginManual,
		Arg: graph.ArgKindRef, EdgeCount: 1,
	}
	consumer := &graph.Node{
		ID: "root/svc:app.(*Consumer)", Kind: graph.NodeService, Scope: "root",
		Type: "github.com/acme/app.(*Consumer)", Name: "github.com/acme/app.NewConsumer",
		Shared: true, Lazy: true, Autowired: true,
		Params: []*graph.Param{param}, OutDegree: 1,
	}
	dep := &graph.Node{
		ID: "root/svc:app.(*Dep)", Kind: graph.NodeService, Scope: "root",
		Type: "github.com/acme/app.(*Dep)", Name: "github.com/acme/app.NewDep",
		Shared: true, Lazy: true, Autowired: true, InDegree: 1,
	}
	orphan := &graph.Node{
		ID: "root/svc:app.(*Orphan)", Kind: graph.NodeService, Scope: "root",
		Type: "github.com/acme/app.(*Orphan)", Name: "github.com/acme/app.NewOrphan",
		Shared: true, Lazy: true, Autowired: true,
	}

	return &graph.Graph{
		Schema: graph.Schema,
		Scopes: []*graph.Scope{{ID: "root", Name: "root"}},
		Nodes:  []*graph.Node{consumer, dep, orphan},
		Edges: []*graph.Edge{{
			ID: graph.NewEdgeID(param.ID, 0), From: consumer.ID, To: dep.ID, Param: param.ID,
			Kind: graph.InjectFactoryArg, Origin: graph.ArgOriginManual,
			Resolution: graph.ResolutionRef, ParamType: dep.Type,
		}},
	}
}

func hash(content string) string {
	sum := sha256.Sum256([]byte(content))
	return "sha256-" + base64.StdEncoding.EncodeToString(sum[:])
}

var (
	cspRe    = regexp.MustCompile(`content="(default-src[^"]*)"`)
	dataRe   = regexp.MustCompile(`(?s)<script type="application/json" id="godi-data">(.*?)</script>`)
	scriptRe = regexp.MustCompile(`(?s)<script>(.*?)</script>`)
)

func capture(t *testing.T, re *regexp.Regexp, page string) string {
	t.Helper()

	m := re.FindStringSubmatch(page)
	require.NotNil(t, m, "no match for %s", re)
	return m[1]
}

// The policy names the hash of every script the page carries. If one ever
// drifts, the browser silently refuses to run it and the page looks merely
// broken, so this is the test that has to hold.
func TestPolicyNamesEveryScript(t *testing.T) {
	for _, engine := range []html.LayoutEngine{html.Graphviz, html.Dagre} {
		t.Run(string(engine), func(t *testing.T) {
			page := encode(t, model(), html.Layout(engine))
			csp := capture(t, cspRe, page)

			scripts := scriptRe.FindAllStringSubmatch(page, -1)
			require.NotEmpty(t, scripts)
			for _, script := range scripts {
				require.Contains(t, csp, hash(script[1]), "a script is not named by the policy")
			}
			require.Contains(t, csp, hash(capture(t, dataRe, page)), "the data block is not named")

			require.Contains(t, csp, "default-src 'none'")
			require.NotContains(t, csp, "script-src 'unsafe-inline'")
			require.NotContains(t, csp, "'unsafe-eval'") // Only 'wasm-unsafe-eval' is acceptable.
		})
	}
}

// Instantiating WebAssembly needs an explicit source, and nothing else should
// ask for one.
func TestWasmIsAllowedOnlyWhenGraphvizIsThere(t *testing.T) {
	require.Contains(t, capture(t, cspRe, encode(t, model())), "'wasm-unsafe-eval'")
	require.NotContains(t, capture(t, cspRe, encode(t, model(), html.Layout(html.Dagre))), "wasm-unsafe-eval")
}

func TestPageIsSelfContained(t *testing.T) {
	page := encode(t, model())

	for _, remote := range []string{"src=\"http", "<link", "@import"} {
		require.NotContains(t, page, remote)
	}
}

// Only the engine that was asked for is inlined. That is what keeps a dagre
// page a fraction of the size, and what keeps Graphviz's EPL-2.0 object code
// out of it entirely.
func TestOnlyTheChosenEngineIsEmbedded(t *testing.T) {
	graphviz := encode(t, model(), html.Layout(html.Graphviz))
	require.Contains(t, graphviz, "Viz.js")
	require.NotContains(t, graphviz, "Chris Pettitt")

	dagre := encode(t, model(), html.Layout(html.Dagre))
	require.NotContains(t, dagre, "Viz.js")
	require.NotContains(t, dagre, "graphvizVersion")
	require.Contains(t, dagre, "cytoscape-dagre")

	require.Less(t, len(dagre), len(graphviz)/2)
}

// Inlining a bundle redistributes it. Dagre is the one that ships without a
// header of its own, so the page has to carry its notice.
func TestDagreNoticeTravelsWithTheBundle(t *testing.T) {
	page := encode(t, model(), html.Layout(html.Dagre))

	require.Contains(t, page, "/*! dagre 3.0.0 | (c) 2012-2014 Chris Pettitt | MIT")
}

func TestCreditsMatchTheBundles(t *testing.T) {
	tests := []struct {
		engine html.LayoutEngine
		want   []string
	}{
		{html.Graphviz, []string{"cytoscape", "viz.js", "graphviz"}},
		{html.Dagre, []string{"cytoscape", "dagre", "cytoscape-dagre"}},
	}

	for _, tt := range tests {
		t.Run(string(tt.engine), func(t *testing.T) {
			var data struct {
				Credits []struct {
					Name    string `json:"name"`
					Licence string `json:"licence"`
				} `json:"credits"`
			}
			require.NoError(t, json.Unmarshal([]byte(capture(t, dataRe, encode(t, model(), html.Layout(tt.engine)))), &data))

			var names []string
			for _, c := range data.Credits {
				names = append(names, c.Name)
				require.NotEmpty(t, c.Licence)
			}
			require.Equal(t, tt.want, names)
		})
	}
}

// The page is nothing but the model plus a renderer, so everything the viewer
// can show has to be in the payload.
func TestPayloadCarriesTheWholeGraph(t *testing.T) {
	g := model()

	var data struct {
		Nodes []struct {
			ID     string `json:"id"`
			Params []struct {
				Origin string `json:"origin"`
			} `json:"params"`
		} `json:"nodes"`
		Edges []struct {
			ID         string `json:"id"`
			Origin     string `json:"origin"`
			Resolution string `json:"resolution"`
		} `json:"edges"`
		Scopes []struct {
			ID string `json:"id"`
		} `json:"scopes"`
	}
	require.NoError(t, json.Unmarshal([]byte(capture(t, dataRe, encode(t, g))), &data))

	require.Len(t, data.Nodes, len(g.Nodes))
	require.Len(t, data.Edges, len(g.Edges))
	require.Len(t, data.Scopes, len(g.Scopes))

	require.Equal(t, string(g.Edges[0].ID), data.Edges[0].ID)
	require.Equal(t, "manual", data.Edges[0].Origin)
	require.Equal(t, "ref", data.Edges[0].Resolution)
	require.Equal(t, "manual", data.Nodes[0].Params[0].Origin)
}

// An argument godi autowired, through a binding a compiler pass created, was
// being drawn in the pass's colour but filtered as autowiring. Both now ask the
// same field.
func TestWhoDecidedFollowsTheBinding(t *testing.T) {
	tests := []struct {
		name string
		arg  graph.ArgOrigin
		bind []graph.BindingHop
		want string
	}{
		{"no binding, hand-wired", graph.ArgOriginManual, nil, "manual"},
		{"no binding, autowired", graph.ArgOriginAutowiring, nil, "autowiring"},
		{
			"autowired through a binding a pass created", graph.ArgOriginAutowiring,
			[]graph.BindingHop{{Interface: "app.Reporter", Origin: graph.BindOriginCompilerPass, OriginPass: "bind reporter"}},
			"compiler-pass",
		},
		{
			"autowired through godi's own binding", graph.ArgOriginAutowiring,
			[]graph.BindingHop{{Interface: "app.Logger", Origin: graph.BindOriginAutobinding}},
			"autowiring",
		},
		{
			"autowired through a binding you declared", graph.ArgOriginAutowiring,
			[]graph.BindingHop{{Interface: "app.Clock", Origin: graph.BindOriginManual}},
			"manual",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := model()
			g.Edges[0].Origin = tt.arg
			g.Edges[0].Bindings = tt.bind

			var data struct {
				Edges []struct {
					DecidedBy string `json:"decidedBy"`
				} `json:"edges"`
			}
			require.NoError(t, json.Unmarshal([]byte(capture(t, dataRe, encode(t, g))), &data))
			require.Equal(t, tt.want, data.Edges[0].DecidedBy)
		})
	}
}

// The argument row already carries the declared type, so a literal that repeats
// it reads as "string = string = production".
func TestLiteralDoesNotRepeatItsType(t *testing.T) {
	g := model()
	g.Nodes[0].Params[0].Literals = []graph.Literal{
		{Type: "string", Value: `"production"`},
		{Type: "time.Duration", Value: "5m0s", Truncated: true},
		{Type: "int"},
	}

	var data struct {
		Nodes []struct {
			Params []struct {
				Literals []string `json:"literals"`
			} `json:"params"`
		} `json:"nodes"`
	}
	require.NoError(t, json.Unmarshal([]byte(capture(t, dataRe, encode(t, g))), &data))

	require.Equal(t, []string{`"production"`, "5m0s…", "‹literal›"}, data.Nodes[0].Params[0].Literals)
}

// A pass can be responsible for an edge by wiring the argument or by creating
// the binding it resolved through. The page named only the first, so a
// dependency a pass chose through a binding was drawn in the pass's colour with
// nothing saying which pass.
func TestThePassIsNamedHoweverItWasResponsible(t *testing.T) {
	tests := []struct {
		name string
		arg  graph.ArgOrigin
		pass string
		bind []graph.BindingHop
		want string
	}{
		{"nobody to name", graph.ArgOriginAutowiring, "", nil, ""},
		{"a pass wired the argument", graph.ArgOriginCompilerPass, "override arg", nil, "override arg"},
		{
			"a pass created the binding", graph.ArgOriginAutowiring, "",
			[]graph.BindingHop{{Origin: graph.BindOriginCompilerPass, OriginPass: "bind reporter"}},
			"bind reporter",
		},
		{
			"the same pass did both", graph.ArgOriginCompilerPass, "rewire",
			[]graph.BindingHop{{Origin: graph.BindOriginCompilerPass, OriginPass: "rewire"}},
			"rewire",
		},
		{
			"two passes, so it has to say which did what", graph.ArgOriginCompilerPass, "override arg",
			[]graph.BindingHop{{Origin: graph.BindOriginCompilerPass, OriginPass: "bind reporter"}},
			"arg: override arg, bind: bind reporter",
		},
		{
			"godi's own binding is not named", graph.ArgOriginAutowiring, "",
			[]graph.BindingHop{{Origin: graph.BindOriginAutobinding, OriginPass: "interface binding"}},
			"",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := model()
			g.Edges[0].Origin = tt.arg
			g.Edges[0].OriginPass = tt.pass
			g.Edges[0].Bindings = tt.bind

			require.Equal(t, tt.want, g.Edges[0].PassCredit())

			var data struct {
				Edges []struct {
					Pass string `json:"pass"`
				} `json:"edges"`
			}
			require.NoError(t, json.Unmarshal([]byte(capture(t, dataRe, encode(t, g))), &data))
			require.Equal(t, tt.want, data.Edges[0].Pass, "the page must say what the model says")
		})
	}
}

func TestThemeSetsTheRootClass(t *testing.T) {
	require.Contains(t, encode(t, model()), `class="theme-auto"`)
	require.Contains(t, encode(t, model(), html.Theme(html.Dark)), `class="theme-dark"`)
	require.Contains(t, encode(t, model(), html.Theme(html.Light)), `class="theme-light"`)
}

func TestLayoutIsAnnouncedToTheViewer(t *testing.T) {
	require.Contains(t, encode(t, model()), `data-layout="graphviz"`)
	require.Contains(t, encode(t, model(), html.Layout(html.Dagre)), `data-layout="dagre"`)
}

func TestTitleIsEscaped(t *testing.T) {
	page := encode(t, model(), html.Title(`</script><img onerror=x>`))

	require.NotContains(t, page, "<img onerror")
	require.Contains(t, page, "&lt;/script&gt;&lt;img onerror=x&gt;")
}

func TestFormat(t *testing.T) {
	require.Equal(t,
		graph.Format{Name: "html", Ext: "html", MediaType: "text/html; charset=utf-8"},
		html.New().Format())
}

// The JSON is inlined into a script element, so a value that closes it would
// break the page open. encoding/json escapes the characters that could.
func TestPayloadCannotBreakOutOfItsScriptBlock(t *testing.T) {
	g := model()
	g.Nodes[0].Labels = []string{`</script><script>alert(1)</script>`}

	page := encode(t, g)

	// The closing tag never appears literally, so the block cannot be ended
	// early - but the value still arrives intact once parsed.
	require.NotContains(t, page, "alert(1)</script>")
	require.Equal(t, 1, strings.Count(page, "</script>\n</body>"))

	var data struct {
		Nodes []struct {
			Labels []string `json:"labels"`
		} `json:"nodes"`
	}
	require.NoError(t, json.Unmarshal([]byte(capture(t, dataRe, page)), &data))
	require.Equal(t, []string{`</script><script>alert(1)</script>`}, data.Nodes[0].Labels)
}

func TestLocationsReachThePayload(t *testing.T) {
	g := model()
	g.SourceRoot = "/home/me/app"
	g.Nodes[0].Registered = graph.Location{File: "wiring.go", Line: 42, Func: "app.wire"}
	g.Nodes[0].Defined = graph.Location{File: "http/server.go", Line: 118}

	var data struct {
		SourceRoot string `json:"sourceRoot"`
		Nodes      []struct {
			Registered *struct {
				Text string `json:"text"`
				File string `json:"file"`
				Line int    `json:"line"`
			} `json:"registered"`
			Defined *struct {
				Text string `json:"text"`
			} `json:"defined"`
		} `json:"nodes"`
	}
	require.NoError(t, json.Unmarshal([]byte(capture(t, dataRe, encode(t, g))), &data))

	require.Equal(t, "/home/me/app", data.SourceRoot)
	require.Equal(t, "wiring.go:42", data.Nodes[0].Registered.Text)
	require.Equal(t, "wiring.go", data.Nodes[0].Registered.File)
	require.Equal(t, 42, data.Nodes[0].Registered.Line)
	require.Equal(t, "http/server.go:118", data.Nodes[0].Defined.Text)

	// A node with no location carries none, rather than an empty one.
	require.Nil(t, data.Nodes[1].Registered)
	require.Nil(t, data.Nodes[1].Defined)
}

// A link is only offered when the caller says how to build one: a file:// link
// to a .go file just downloads it, so guessing would be worse than nothing.
func TestSourceLinkIsOptIn(t *testing.T) {
	require.NotContains(t, capture(t, dataRe, encode(t, model())), "sourceLink")

	page := encode(t, model(), html.SourceLink("vscode://file{file}:{line}"))
	require.Contains(t, capture(t, dataRe, page), `"sourceLink":"vscode://file{file}:{line}"`)
}

// Every preset has to carry the placeholders that make it a template, or it
// would link to the same line of the same file forever.
func TestEveryEditorPresetIsAUsableTemplate(t *testing.T) {
	names := html.Editors()
	require.NotEmpty(t, names)
	require.IsIncreasing(t, names, "the list goes in a usage message, so it is sorted")

	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			template, ok := html.EditorLink(name)
			require.True(t, ok)
			require.Contains(t, template, "{line}")
			require.True(t,
				strings.Contains(template, "{file}") || strings.Contains(template, "{rel}"),
				"a template with no path in it: %s", template)
			require.Contains(t, template, "://")
		})
	}
}

func TestEditorLookupIsForgivingButNotInventive(t *testing.T) {
	template, ok := html.EditorLink("  VSCode ")
	require.True(t, ok, "a name should survive the case and spacing of a flag")
	require.Equal(t, html.VSCode, template)

	// An unknown name must be reported, not quietly turned into a dead link:
	// the caller can then treat it as a template of the reader's own.
	_, ok = html.EditorLink("notepad")
	require.False(t, ok)
}

// A query-style template carries an ampersand, which JSON escapes on the way
// into the page. It has to arrive intact.
func TestATemplateWithQueryParametersSurvivesEncoding(t *testing.T) {
	page := encode(t, model(), html.SourceLink(html.GoLand))

	var data struct {
		SourceLink string `json:"sourceLink"`
	}
	require.NoError(t, json.Unmarshal([]byte(capture(t, dataRe, page)), &data))
	require.Equal(t, "goland://open?file={file}&line={line}", data.SourceLink)
	require.NotContains(t, page, "goland://open?file={file}&line=", "the raw & must not reach the markup")
}

// The page has to be able to say the graph carries on past what it draws, so
// the count a filter left behind has to reach it.
func TestElisionReachesThePayload(t *testing.T) {
	g := model()
	g.Nodes[0].Elided = 4

	var data struct {
		Nodes []struct {
			ID     string `json:"id"`
			Elided int    `json:"elided"`
		} `json:"nodes"`
	}
	require.NoError(t, json.Unmarshal([]byte(capture(t, dataRe, encode(t, g))), &data))

	byID := make(map[string]int, len(data.Nodes))
	for _, n := range data.Nodes {
		byID[n.ID] = n.Elided
	}
	require.Equal(t, 4, byID[string(g.Nodes[0].ID)])
	require.Equal(t, 0, byID[string(g.Nodes[1].ID)], "a node nothing was cut from says nothing")
}
