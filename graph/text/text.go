// Package text encodes a dependency graph as plain text.
//
// It is the format for reading a container in a terminal, in a log, or in a test
// failure: an indented outline of the scopes, what is registered in each, and
// where every argument comes from. Nothing here needs a tool to look at.
//
// Where the other encoders draw the wiring, this one states it. Each argument row
// says what was declared, what it resolved to, and who decided, so the picture's
// three questions are answerable without the picture.
package text

import (
	"bufio"
	"fmt"
	"io"
	"strings"

	"github.com/michalkurzeja/godi/v2/graph"
	"github.com/michalkurzeja/godi/v2/graph/internal/render"
)

// Encoder writes a graph as plain text.
type Encoder struct {
	cfg config
}

// New returns a text encoder.
func New(opts ...Option) *Encoder {
	cfg := config{locations: true, maxType: 48}
	for _, opt := range opts {
		opt(&cfg)
	}
	return &Encoder{cfg: cfg}
}

func (e *Encoder) Format() graph.Format {
	return graph.Format{Name: "text", Ext: "txt", MediaType: "text/plain; charset=utf-8"}
}

func (e *Encoder) Encode(g *graph.Graph, w io.Writer) error {
	buf := bufio.NewWriter(w)
	p := &printer{w: buf, cfg: e.cfg, graph: g}
	p.write()
	return buf.Flush()
}

type printer struct {
	w     *bufio.Writer
	cfg   config
	graph *graph.Graph
}

func (p *printer) printf(format string, args ...any) {
	_, _ = fmt.Fprintf(p.w, format, args...)
}

// linef writes one indented line. Indentation is the whole of the structure here,
// so it goes through one place.
func (p *printer) linef(depth int, format string, args ...any) {
	p.printf("%s%s\n", strings.Repeat("  ", depth), fmt.Sprintf(format, args...))
}

func (p *printer) write() {
	p.linef(0, "%s", p.summary())
	p.snapshot()
	if p.cfg.locations && p.graph.SourceRoot != "" {
		p.linef(0, "under %s", p.graph.SourceRoot)
	}

	for _, scope := range p.graph.Scopes {
		if scope.Parent == "" {
			p.scope(scope, 0)
		}
	}

	p.diagnostics()
}

// summary is the first line: enough to tell at a glance whether this is the
// container you meant.
func (p *printer) summary() string {
	var services, functions, roots int
	for _, n := range p.graph.Nodes {
		if n.Kind == graph.NodeFunction {
			functions++
		} else {
			services++
		}
		if n.Root {
			roots++
		}
	}

	return strings.Join([]string{
		count(services, "service", "services"),
		count(functions, "function", "functions"),
		count(len(p.graph.Edges), "dependency", "dependencies"),
		count(roots, "root", "roots"),
	}, ", ")
}

// snapshot says the container was still being built when this was taken. It goes
// directly under the counts, which are the first thing a half-wired graph
// misleads you about.
func (p *printer) snapshot() {
	if !p.graph.Partial() {
		return
	}

	p.linef(0, "snapshot: %s", p.graph.Snapshot.Label())
	if len(p.graph.Snapshot.Done) > 0 {
		p.linef(1, "passes run: %s", strings.Join(p.graph.Snapshot.Done, ", "))
	}
}

func count(n int, one, many string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, one)
	}
	return fmt.Sprintf("%d %s", n, many)
}

func (p *printer) scope(scope *graph.Scope, depth int) {
	p.printf("\n")
	p.linef(depth, "scope %s", scope.Label())

	p.bindings(scope, depth+1)

	var services, functions []*graph.Node
	for _, n := range p.graph.ScopeNodes(scope.ID) {
		if n.Kind == graph.NodeFunction {
			functions = append(functions, n)
		} else {
			services = append(services, n)
		}
	}

	p.nodes("services", services, depth+1)
	p.nodes("functions", functions, depth+1)

	// Nested, because a child scope is only reachable through the definition
	// that declared it. The indentation says so.
	for _, child := range p.graph.ChildScopes(scope.ID) {
		p.scope(child, depth+1)
	}
}

func (p *printer) bindings(scope *graph.Scope, depth int) {
	var declared []*graph.Binding
	for _, b := range p.graph.Bindings {
		if b.Scope == scope.ID {
			declared = append(declared, b)
		}
	}
	if len(declared) == 0 {
		return
	}

	p.linef(depth, "bindings:")
	for _, b := range declared {
		unused := ""
		if b.EdgeCount == 0 {
			unused = "  (nothing uses it)"
		}
		p.linef(depth+1, "%s -> %s  [%s]%s",
			render.Short(b.Interface), render.Short(b.BoundTo), p.bindingOrigin(b.Origin, b.OriginPass), unused)
	}
}

func (p *printer) nodes(heading string, nodes []*graph.Node, depth int) {
	if len(nodes) == 0 {
		return
	}

	p.linef(depth, "%s:", heading)
	for _, n := range nodes {
		p.node(n, depth+1)
	}
}

func (p *printer) node(n *graph.Node, depth int) {
	title := n.Title()

	p.linef(depth, "%s%s", title, bracketed(p.nodeFlags(n)))
	if label, what := p.subtitle(n); what != "" && what != title {
		p.linef(depth+1, "%s: %s", label, what)
	}
	if p.cfg.locations {
		if !n.Registered.IsZero() {
			p.linef(depth+1, "registered: %s", n.Registered)
		}
		if !n.Declared.IsZero() {
			p.linef(depth+1, "declared: %s", n.Declared)
		}
	}

	p.params(n, depth+1)

	if n.Elided > 0 {
		p.linef(depth+1, "... %d neighbours were filtered out", n.Elided)
	}
}

// subtitle is the line under a node's heading: the factory that builds a
// service, the value it was registered as, or the signature of a function.
//
// What to say is the model's answer, the same one the picture puts under a node
// box. What to call it is this format's own: a line of text has room to say
// which of the three it is, and a box does not.
func (p *printer) subtitle(n *graph.Node) (label, what string) {
	switch {
	case n.Kind == graph.NodeFunction:
		label = "signature"
	case n.FromValue:
		label = "value"
	default:
		label = "factory"
	}
	return label, n.Subtitle()
}

// nodeFlags are the properties worth naming. Only the surprising half of each
// pair is printed - a lazy shared service is the default and says nothing - so
// what is left is what someone chose.
func (p *printer) nodeFlags(n *graph.Node) []string {
	var flags []string
	if n.Root {
		flags = append(flags, "root")
	}
	if !n.Lazy {
		flags = append(flags, "eager")
	}
	if n.Kind == graph.NodeService && !n.Shared {
		flags = append(flags, "not shared")
	}
	if !n.Autowired {
		flags = append(flags, "not autowired")
	}
	if n.Instantiated {
		flags = append(flags, "instantiated")
	}
	return append(flags, n.Labels...)
}

func (p *printer) params(n *graph.Node, depth int) {
	var args, calls []*graph.Param
	for _, param := range n.Params {
		if param.Kind == graph.InjectMethodArg || param.Kind == graph.InjectMethodReceiver {
			calls = append(calls, param)
		} else {
			args = append(args, param)
		}
	}

	if len(args) > 0 {
		p.linef(depth, "args:")
		for _, param := range args {
			p.param(param, depth+1)
		}
	}

	// Grouped by method, because a method call is one act of wiring however
	// many arguments it takes.
	var method string
	for i, param := range calls {
		if i == 0 {
			p.linef(depth, "method calls:")
		}
		if param.Method != method {
			method = param.Method
			p.linef(depth+1, "%s():", method)
		}
		p.param(param, depth+2)
	}
}

// param is one argument: what was asked for, what arrived, and who decided.
func (p *printer) param(param *graph.Param, depth int) {
	head := fmt.Sprintf("%d <- %s", param.Index, render.Ellipsis(param.TypeShort(), p.cfg.maxType))
	origin := bracketed(p.argOrigin(param))

	edges := p.paramEdges(param)
	switch {
	case len(param.Literals) > 0:
		p.linef(depth, "%s = %s%s", head, param.LiteralsText(), origin)
	case len(edges) == 0:
		p.linef(depth, "%s%s%s", head, p.unresolved(param), origin)
	default:
		p.linef(depth, "%s%s", head, origin)
	}

	for _, e := range edges {
		p.linef(depth+1, "-> %s%s", p.name(e.To), bracketed(p.resolution(e)))
	}
}

// name is what to call a node in a row that points at it. A node ID is a path
// built to be unique rather than to be read, so it is only the fallback.
func (p *printer) name(id graph.NodeID) string {
	n, ok := p.graph.Node(id)
	if !ok {
		return string(id)
	}
	return n.Title()
}

func (p *printer) paramEdges(param *graph.Param) []*graph.Edge {
	var out []*graph.Edge
	for _, e := range p.graph.OutEdges(param.Node) {
		if e.Param == param.ID {
			out = append(out, e)
		}
	}
	return out
}

func (p *printer) unresolved(param *graph.Param) string {
	switch {
	case param.Origin == graph.ArgOriginNone:
		return "  (not wired)"
	case param.Unresolved:
		return "  (unresolved)"
	case param.Label != "":
		return "  (label: " + param.Label + ")"
	default:
		// A variadic slot nobody supplies, which is what an optional dependency
		// looks like when nothing provides one.
		return "  (nothing)"
	}
}

// resolution says how a dependency was matched, and names the binding it went
// through when it went through one.
func (p *printer) resolution(e *graph.Edge) []string {
	out := []string{string(e.Resolution)}
	if hop, ok := e.Binding(); ok {
		// Parenthesised rather than another colon: a pass-created binding would
		// otherwise read "binding on X: compiler-pass: name".
		out = append(out, fmt.Sprintf("binding on %s (%s)",
			render.Short(hop.Interface), p.bindingOrigin(hop.Origin, hop.OriginPass)))
	}
	if e.Cycle {
		out = append(out, "cycle")
	}
	return out
}

// Only an extension is worth naming: godi's own automation runs under a
// compiler pass too, but "autowiring (autowiring)" tells nobody anything.
func (p *printer) argOrigin(param *graph.Param) []string {
	if param.Origin == graph.ArgOriginCompilerPass && param.OriginPass != "" {
		return []string{string(param.Origin) + ": " + param.OriginPass}
	}
	return []string{string(param.Origin)}
}

func (p *printer) bindingOrigin(origin graph.BindOrigin, pass string) string {
	if origin == graph.BindOriginCompilerPass && pass != "" {
		return string(origin) + ": " + pass
	}
	return string(origin)
}

func bracketed(parts []string) string {
	if len(parts) == 0 {
		return ""
	}
	return "  [" + strings.Join(parts, ", ") + "]"
}

func (p *printer) diagnostics() {
	notices := p.graph.AllDiagnostics()
	if len(notices) == 0 {
		return
	}

	p.printf("\n")
	p.linef(0, "notices:")
	for _, d := range notices {
		p.linef(1, "%s: %s", d.Severity, d.Message)
	}
}
