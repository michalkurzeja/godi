// Package text encodes a dependency graph as plain text.
//
// It is the format for reading a container in a terminal, in a log, or in a
// test failure: an indented outline of the scopes, what is registered in each,
// and where every argument comes from. Nothing here needs a tool to look at.
//
// Where the other encoders draw the wiring, this one states it. Each argument
// row says what was declared, what it resolved to, and who decided - so the
// same three questions the picture answers are answerable without one.
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
	p.write(g)
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

// linef writes one indented line. Indentation is the whole of the structure
// here, so it goes through one place.
func (p *printer) linef(depth int, format string, args ...any) {
	p.printf("%s%s\n", strings.Repeat("  ", depth), fmt.Sprintf(format, args...))
}

func (p *printer) write(g *graph.Graph) {
	p.linef(0, "%s", p.summary(g))
	p.snapshot(g)
	if p.cfg.locations && g.SourceRoot != "" {
		p.linef(0, "under %s", g.SourceRoot)
	}

	for _, scope := range g.Scopes {
		if scope.Parent == "" {
			p.scope(g, scope, 0)
		}
	}

	p.diagnostics(g)
}

// summary is the first line: enough to tell at a glance whether this is the
// container you meant.
func (p *printer) summary(g *graph.Graph) string {
	var services, functions, roots int
	for _, n := range g.Nodes {
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
		count(len(g.Edges), "dependency", "dependencies"),
		count(roots, "root", "roots"),
	}, ", ")
}

// snapshot says the container was still being built when this was taken. It
// goes directly under the counts, because those counts are the first thing a
// half-wired graph misleads you about.
func (p *printer) snapshot(g *graph.Graph) {
	if !g.Partial() {
		return
	}

	p.linef(0, "snapshot: %s", g.Snapshot.Label())
	if len(g.Snapshot.Done) > 0 {
		p.linef(1, "passes run: %s", strings.Join(g.Snapshot.Done, ", "))
	}
}

func count(n int, one, many string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, one)
	}
	return fmt.Sprintf("%d %s", n, many)
}

func (p *printer) scope(g *graph.Graph, scope *graph.Scope, depth int) {
	p.printf("\n")
	p.linef(depth, "scope %s", scope.Label())

	p.bindings(g, scope, depth+1)

	var services, functions []*graph.Node
	for _, n := range g.ScopeNodes(scope.ID) {
		if n.Kind == graph.NodeFunction {
			functions = append(functions, n)
		} else {
			services = append(services, n)
		}
	}

	p.nodes(g, "services", services, depth+1)
	p.nodes(g, "functions", functions, depth+1)

	// Nested, because a child scope is only reachable through the definition
	// that declared it, and the indentation is what says so.
	for _, child := range g.ChildScopes(scope.ID) {
		p.scope(g, child, depth+1)
	}
}

func (p *printer) bindings(g *graph.Graph, scope *graph.Scope, depth int) {
	var declared []*graph.Binding
	for _, b := range g.Bindings {
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
			render.Short(b.Interface), render.Short(b.BoundTo), bindingOrigin(b.Origin, b.OriginPass), unused)
	}
}

func (p *printer) nodes(g *graph.Graph, heading string, nodes []*graph.Node, depth int) {
	if len(nodes) == 0 {
		return
	}

	p.linef(depth, "%s:", heading)
	for _, n := range nodes {
		p.node(g, n, depth+1)
	}
}

func (p *printer) node(g *graph.Graph, n *graph.Node, depth int) {
	title := n.Title()

	p.linef(depth, "%s%s", title, bracketed(nodeFlags(n)))
	if label, what := implementation(n); what != "" && what != title {
		p.linef(depth+1, "%s: %s", label, what)
	}
	if p.cfg.locations {
		if !n.Registered.IsZero() {
			p.linef(depth+1, "registered: %s", n.Registered)
		}
		if !n.Defined.IsZero() {
			p.linef(depth+1, "defined: %s", n.Defined)
		}
	}

	p.params(g, n, depth+1)

	if n.Elided > 0 {
		p.linef(depth+1, "... %d neighbours were filtered out", n.Elided)
	}
}

// implementation describes what provides a node, for the line under its
// heading: the factory that builds a service, the value it was registered as,
// or the signature of a function.
//
// Whichever of the name and the signature the heading did not take. A name the
// runtime made up describes nothing, so where the heading is a signature it is
// still worth printing - it is the only thing that says which literal this is.
func implementation(n *graph.Node) (label, what string) {
	if n.Kind == graph.NodeFunction {
		return "signature", n.TypeShort()
	}

	label = "factory"
	if n.FromValue {
		label = "value"
	}
	// Either the name is the heading, and the signature is what is left to say,
	// or the runtime made the name up and the signature is the only thing that
	// describes it. A value's signature is its type, so where that is already the
	// heading the caller drops this line.
	if n.KnownByName() || n.Anonymous() {
		return label, render.Short(n.Signature)
	}
	return label, n.NameShort()
}

// nodeFlags are the properties worth naming. Only the surprising half of each
// pair is printed - a lazy shared service is the default and says nothing - so
// what is left is what someone chose.
func nodeFlags(n *graph.Node) []string {
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

func (p *printer) params(g *graph.Graph, n *graph.Node, depth int) {
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
			p.param(g, param, depth+1)
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
		p.param(g, param, depth+2)
	}
}

// param is one argument: what was asked for, what arrived, and who decided.
func (p *printer) param(g *graph.Graph, param *graph.Param, depth int) {
	head := fmt.Sprintf("%d <- %s", param.Index, render.Ellipsis(param.TypeShort(), p.cfg.maxType))

	edges := paramEdges(g, param)
	switch {
	case len(param.Literals) > 0:
		p.linef(depth, "%s = %s%s", head, param.LiteralsText(), bracketed(argOrigin(param.Origin, param.OriginPass)))
	case len(edges) == 0:
		p.linef(depth, "%s%s%s", head, unresolved(param), bracketed(argOrigin(param.Origin, param.OriginPass)))
	default:
		p.linef(depth, "%s%s", head, bracketed(argOrigin(param.Origin, param.OriginPass)))
	}

	for _, e := range edges {
		p.linef(depth+1, "-> %s%s", p.name(e.To), bracketed(resolution(e)))
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

func paramEdges(g *graph.Graph, param *graph.Param) []*graph.Edge {
	var out []*graph.Edge
	for _, e := range g.OutEdges(param.Node) {
		if e.Param == param.ID {
			out = append(out, e)
		}
	}
	return out
}

func unresolved(param *graph.Param) string {
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
func resolution(e *graph.Edge) []string {
	out := []string{string(e.Resolution)}
	if hop, ok := e.Binding(); ok {
		// Parenthesised rather than another colon: a pass-created binding would
		// otherwise read "binding on X: compiler-pass: name".
		out = append(out, fmt.Sprintf("binding on %s (%s)",
			render.Short(hop.Interface), bindingOrigin(hop.Origin, hop.OriginPass)))
	}
	if e.Cycle {
		out = append(out, "cycle")
	}
	return out
}

// Only an extension is worth naming: godi's own automation runs under a
// compiler pass too, but "autowiring (autowiring)" tells nobody anything.
func argOrigin(origin graph.ArgOrigin, pass string) []string {
	if origin == graph.ArgOriginCompilerPass && pass != "" {
		return []string{string(origin) + ": " + pass}
	}
	return []string{string(origin)}
}

func bindingOrigin(origin graph.BindOrigin, pass string) string {
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

func (p *printer) diagnostics(g *graph.Graph) {
	if len(g.Diagnostics) == 0 {
		return
	}

	p.printf("\n")
	p.linef(0, "notices:")
	for _, d := range g.Diagnostics {
		p.linef(1, "%s: %s", d.Severity, d.Message)
	}
}
