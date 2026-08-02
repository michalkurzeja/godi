// Package dot encodes a dependency graph as Graphviz DOT.
//
// Scopes become nested clusters, each service becomes a table with one row per
// constructor argument, and every edge lands on the row it feeds. What is passed
// where is then visible without reading a legend.
//
// Provenance uses two independent channels. The arrowhead says how the dependency
// was matched: a point for an exact type, a diamond for anything that went
// through an interface binding.
//
// The colour says who decided on it: you, godi, or a compiler pass. For a binding
// that means whoever created the binding. The line style repeats who wired the
// argument itself.
package dot

import (
	"bufio"
	"fmt"
	"html"
	"io"
	"strings"

	"github.com/michalkurzeja/godi/v2/graph"
	"github.com/michalkurzeja/godi/v2/graph/internal/render"
)

// Encoder writes a graph as Graphviz DOT.
type Encoder struct {
	cfg config
}

// New returns a DOT encoder.
func New(opts ...Option) *Encoder {
	cfg := config{
		rankDir:  LR,
		theme:    Light,
		ports:    PortsAuto,
		legend:   true,
		maxLabel: 44,
	}
	for _, opt := range opts {
		opt(&cfg)
	}
	return &Encoder{cfg: cfg}
}

func (e *Encoder) Format() graph.Format {
	return graph.Format{Name: "dot", Ext: "dot", MediaType: "text/vnd.graphviz"}
}

func (e *Encoder) Encode(g *graph.Graph, w io.Writer) error {
	buf := bufio.NewWriter(w)
	p := &printer{w: buf, cfg: e.cfg, palette: e.cfg.theme.palette(), graph: g}
	p.write()
	return buf.Flush()
}

type printer struct {
	w       *bufio.Writer
	cfg     config
	palette palette
	graph   *graph.Graph
	ports   bool
}

func (p *printer) write() {
	p.ports = p.cfg.ports.enabled(len(p.graph.Nodes))

	p.printf("digraph godi {\n")
	p.preamble()

	for _, scope := range p.graph.Scopes {
		if scope.Parent == "" {
			p.scope(scope)
		}
	}

	for _, edge := range p.graph.Edges {
		p.edge(edge)
	}

	if p.cfg.legend {
		p.legend()
	}
	p.notices()
	p.printf("}\n")
}

func (p *printer) preamble() {
	p.printf("\tgraph [rankdir=%s, newrank=true, compound=true, splines=spline, pad=0.3,\n", p.cfg.rankDir)
	p.printf("\t       nodesep=0.30, ranksep=0.85, bgcolor=%s,\n", quote(p.palette.background))
	// Cluster labels inherit this one. Without it they fall back to black,
	// which is unreadable on a dark background.
	p.printf("\t       fontcolor=%s, fontname=%s, fontsize=10];\n",
		quote(p.palette.text), quote(fontName))
	p.printf("\tnode [shape=box, style=\"rounded,filled\", fillcolor=%s, color=%s,\n",
		quote(p.palette.nodeFill), quote(p.palette.nodeBorder))
	p.printf("\t      fontcolor=%s, fontname=%s, fontsize=10, penwidth=1.2, margin=\"0.10,0.05\"];\n",
		quote(p.palette.text), quote(fontName))
	p.printf("\tedge [color=%s, fontcolor=%s, penwidth=1.2, arrowsize=0.75,\n",
		quote(p.palette.nodeBorder), quote(p.palette.muted))
	p.printf("\t      fontname=%s, fontsize=8];\n\n", quote(fontName))
}

func (p *printer) scope(scope *graph.Scope) {
	// A child scope's label already reads as a sentence. A bare scope name does
	// not, so it is labelled as a scope.
	label := scope.Label()
	if scope.Owner == "" {
		label = "scope: " + label
	}

	// Cluster names must start with "cluster" for Graphviz to draw the box.
	p.printf("\tsubgraph %s {\n", quote("cluster_"+string(scope.ID)))
	p.printf("\t\tgraph [label=%s, labeljust=l, style=\"rounded,filled\", fillcolor=%s, color=%s,\n",
		quote(label), quote(p.palette.clusterFill(scope.Depth)), quote(p.palette.clusterBorder))
	// A child scope is named after the definition that owns it, so the id needs
	// the prefix: without it the cluster and its owner collide in the SVG.
	p.printf("\t\t       fontcolor=%s, class=%s, id=%s];\n",
		quote(p.palette.text), quote(fmt.Sprintf("scope depth%d", scope.Depth)), quote("scope:"+string(scope.ID)))

	for _, node := range p.graph.ScopeNodes(scope.ID) {
		p.node(node)
	}
	for _, child := range p.graph.ChildScopes(scope.ID) {
		p.scope(child)
	}

	p.printf("\t}\n")
}

func (p *printer) node(node *graph.Node) {
	classes := []string{string(node.Kind)}
	if node.Root {
		classes = append(classes, "root")
	}
	if node.Instantiated {
		classes = append(classes, "instantiated")
	}

	style := "rounded,filled"
	if !node.Shared && node.Kind == graph.NodeService {
		style += ",dashed" // A fresh instance every time it is injected.
	}
	penWidth := "1.2"
	if !node.Lazy {
		penWidth = "2.4" // Built during Build, not on demand.
	}

	// Nothing injects a root, so it is the top of a tree. The fill says so,
	// because border width and style are already taken by lazy and shared.
	fill := p.palette.nodeFill
	if node.Root {
		fill = p.palette.rootFill
	}

	p.printf("\t\t%s [id=%s, class=%s, style=%s, color=%s, fillcolor=%s, penwidth=%s, tooltip=%s, label=%s];\n",
		quote(string(node.ID)),
		quote(string(node.ID)),
		quote(strings.Join(classes, " ")),
		quote(style),
		quote(p.palette.nodeBorder),
		quote(fill),
		penWidth,
		quote(p.nodeTooltip(node)),
		p.nodeLabel(node),
	)
}

func (p *printer) nodeTooltip(node *graph.Node) string {
	var sb strings.Builder
	sb.WriteString(node.Type)
	if node.Name != "" {
		sb.WriteString("\n")
		sb.WriteString(node.Name)
	}
	if node.Signature != "" {
		sb.WriteString("\n")
		sb.WriteString(node.Signature)
	}
	sb.WriteString("\nscope: ")
	sb.WriteString(string(node.Scope))
	if len(node.Labels) > 0 {
		sb.WriteString("\nlabels: ")
		sb.WriteString(strings.Join(node.Labels, ", "))
	}
	if !node.Registered.IsZero() {
		sb.WriteString("\nregistered: ")
		sb.WriteString(node.Registered.String())
	}
	if !node.Declared.IsZero() {
		sb.WriteString("\ndeclared: ")
		sb.WriteString(node.Declared.String())
	}
	if node.Root {
		sb.WriteString("\nnothing injects this: it is the top of a tree")
	}
	if node.Elided > 0 {
		fmt.Fprintf(&sb, "\n%d neighbours were filtered out", node.Elided)
	}
	return sb.String()
}

// nodeLabel builds an HTML-like table. Record shapes are not an option, because
// their structure characters collide with Go type syntax ("interface {}",
// "chan<- T").
func (p *printer) nodeLabel(node *graph.Node) string {
	var sb strings.Builder
	sb.WriteString(`<<TABLE BORDER="0" CELLBORDER="0" CELLSPACING="0" CELLPADDING="2">`)

	marker := ""
	if node.Root {
		marker = "▲ " // Nothing injects it: the top of a tree.
	}

	if node.Kind == graph.NodeFunction {
		marker += "ƒ "
	}

	title, subtitle := node.Title(), node.Subtitle()

	p.labelRow(&sb, "", fmt.Sprintf("<B>%s</B>", esc(marker+title)))
	if subtitle != "" && subtitle != title {
		p.labelRow(&sb, "", small(8, p.palette.muted, esc(p.clip(subtitle))))
	}
	if badges := p.nodeBadges(node); badges != "" {
		p.labelRow(&sb, "", small(7, p.palette.muted, esc(badges)))
	}

	if p.ports {
		for _, param := range node.Params {
			p.labelRow(&sb, p.portName(param), small(8, "", esc(p.paramText(param))))
		}
	}

	// A filter stopped here rather than the wiring. A picture that does not say
	// so reads as the whole story.
	if node.Elided > 0 {
		p.labelRow(&sb, "", small(8, p.palette.muted, esc(fmt.Sprintf("⋯ +%d more", node.Elided))))
	}

	sb.WriteString("</TABLE>>")
	return sb.String()
}

func (p *printer) labelRow(sb *strings.Builder, port, content string) {
	sb.WriteString(`<TR><TD ALIGN="LEFT"`)
	if port != "" {
		fmt.Fprintf(sb, ` PORT="%s"`, port)
	}
	sb.WriteString(">")
	sb.WriteString(content)
	sb.WriteString("</TD></TR>")
}

// paramText is the row for one argument: its position, its declared type, and
// the literal it carries if it carries one.
func (p *printer) paramText(param *graph.Param) string {
	var sb strings.Builder
	if param.Method != "" {
		sb.WriteString(param.Method + " ")
	}
	fmt.Fprintf(&sb, "%d ◂ ", param.Index)
	sb.WriteString(p.clip(param.TypeShort()))

	switch {
	case len(param.Literals) > 0:
		sb.WriteString(" = ")
		sb.WriteString(param.LiteralsText())
	case param.Unwired():
		sb.WriteString(" (not wired)")
	case param.Faulty():
		sb.WriteString(" (unresolved)")
	}

	// A constant produces no edge, so the row is the only place its provenance
	// can show. Without it, an argument a pass substituted looks hand-written.
	if param.EdgeCount == 0 && param.Origin == graph.ArgOriginCompilerPass {
		sb.WriteString(" ← " + param.OriginPass)
	}

	// An argument can resolve to something and still be wrong, so what the
	// compiler objected to goes on the row rather than only in the notices.
	for _, d := range param.Diagnostics {
		if d.Message == "" {
			continue
		}
		sb.WriteString(" ⚠ " + p.clip(strings.ReplaceAll(d.Message, "\n", "; ")))
	}

	return sb.String()
}

func (p *printer) nodeBadges(node *graph.Node) string {
	var badges []string
	if node.Kind == graph.NodeService {
		if node.Shared {
			badges = append(badges, "shared")
		} else {
			badges = append(badges, "not shared")
		}
	}
	if node.Lazy {
		badges = append(badges, "lazy")
	} else {
		badges = append(badges, "eager")
	}
	if !node.Autowired {
		badges = append(badges, "not autowired")
	}
	badges = append(badges, node.Labels...)
	return strings.Join(badges, " · ")
}

func (p *printer) edge(edge *graph.Edge) {
	attrs := []string{
		fmt.Sprintf("style=%s", quote(p.lineStyle(edge))),
		fmt.Sprintf("arrowhead=%s", p.arrowHead(edge)),
		fmt.Sprintf("color=%s", quote(p.edgeColour(edge))),
		fmt.Sprintf("class=%s", quote(p.edgeClasses(edge))),
	}
	if edge.ID != "" {
		// Graphviz copies id and class into the SVG. That is what lets the HTML
		// viewer join the drawing back to the model it was drawn from.
		attrs = append(attrs, fmt.Sprintf("id=%s", quote(string(edge.ID))))
	}
	if edge.Origin == graph.ArgOriginCompilerPass {
		attrs = append(attrs, "penwidth=2")
	}
	if label := p.edgeLabel(edge); label != "" {
		attrs = append(attrs, fmt.Sprintf("label=%s", quote(label)))
	}
	if edge.Cycle {
		// Without this, dot ranks a cyclic graph absurdly.
		attrs = append(attrs, "constraint=false")
	}
	attrs = append(attrs, fmt.Sprintf("tooltip=%s", quote(p.edgeTooltip(edge))))

	p.printf("\t%s -> %s [%s];\n", p.tail(edge), quote(string(edge.To)), strings.Join(attrs, ", "))
}

// tail anchors the edge on the argument row it feeds, when rows are drawn.
func (p *printer) tail(edge *graph.Edge) string {
	node := quote(string(edge.From))
	if !p.ports {
		return node
	}
	param, ok := p.graph.Param(edge.Param)
	if !ok {
		return node
	}
	compass := "w"
	if p.cfg.rankDir == TB {
		compass = "s"
	}
	return fmt.Sprintf("%s:%s:%s", node, p.portName(param), compass)
}

// lineStyle says who wired the argument.
func (p *printer) lineStyle(edge *graph.Edge) string {
	switch edge.Origin {
	case graph.ArgOriginManual:
		return "solid"
	case graph.ArgOriginAutowiring, graph.ArgOriginCompilerPass:
		return "dashed"
	case graph.ArgOriginNone:
		return "dotted"
	}
	return "solid"
}

// arrowHead says how the dependency was matched: a point for an exact type, a
// diamond for anything that went through an interface binding. Who created that
// binding is the colour's job, not the shape's.
func (p *printer) arrowHead(edge *graph.Edge) string {
	if _, bound := edge.Binding(); bound {
		return "odiamond"
	}
	return "normal"
}

// edgeColour reinforces the arrowhead. It says who decided on this particular
// dependency: whoever created the binding it resolved through, and otherwise
// whoever wired the argument.
//
// Arrowheads are small, so the colour repeats that distinction rather than the
// one the line style already shows.
func (p *printer) edgeColour(edge *graph.Edge) string {
	if edge.Cycle {
		return p.palette.warn
	}

	switch edge.DecidedBy() {
	case graph.ArgOriginManual:
		return p.palette.manual
	case graph.ArgOriginAutowiring:
		return p.palette.autowired
	case graph.ArgOriginCompilerPass:
		return p.palette.pass
	case graph.ArgOriginNone:
		return p.palette.muted
	}
	return p.palette.manual
}

// edgeLabel names the extension responsible, when one is, and flags a cycle.
func (p *printer) edgeLabel(edge *graph.Edge) string {
	var parts []string
	if pass := edge.PassCredit(); pass != "" {
		parts = append(parts, pass)
	}
	if edge.Cycle {
		parts = append(parts, "cycle")
	}
	return strings.Join(parts, ", ")
}

func (p *printer) edgeClasses(edge *graph.Edge) string {
	classes := []string{"edge", "origin-" + string(edge.Origin), string(edge.Kind)}
	if hop, ok := edge.Binding(); ok {
		classes = append(classes, "bind-"+string(hop.Origin))
	}
	if edge.OfMany {
		classes = append(classes, "of-many")
	}
	if edge.Cycle {
		classes = append(classes, "cycle")
	}
	return strings.Join(classes, " ")
}

func (p *printer) edgeTooltip(edge *graph.Edge) string {
	var sb strings.Builder
	sb.WriteString(edge.ParamType)
	sb.WriteString("\nwired: ")
	sb.WriteString(string(edge.Origin))
	if edge.OriginPass != "" {
		sb.WriteString(" (" + edge.OriginPass + ")")
	}
	sb.WriteString("\nresolved: ")
	sb.WriteString(string(edge.Resolution))
	for _, hop := range edge.Bindings {
		sb.WriteString("\nvia binding ")
		sb.WriteString(render.Short(hop.Interface))
		sb.WriteString(" (" + string(hop.Origin) + ")")
	}
	return sb.String()
}

// notices draws everything worth saying about the graph that is not in it: what
// the extractor could not make sense of, and what is odd about the picture
// itself. A drawing that leaves that out is the last place a reader would think
// to look for it.
func (p *printer) notices() {
	// Escaped a line at a time, then joined with a break. Inside an HTML-like
	// label a newline is only whitespace, and <BR/> is the only line ending.
	notices := p.graph.AllDiagnostics()
	lines := make([]string, 0, len(notices)+3)

	// A half-wired picture looks like a finished one with dependencies missing,
	// so the drawing has to say which it is.
	if p.graph.Partial() {
		lines = append(lines, esc("snapshot: "+p.graph.Snapshot.Label()))
		if len(p.graph.Snapshot.Done) > 0 {
			lines = append(lines, esc("passes run: "+strings.Join(p.graph.Snapshot.Done, ", ")))
		}
	}

	for _, d := range notices {
		lines = append(lines, esc(string(d.Severity)+": "+p.notice(d)))
	}
	// A filter takes a diagnostic away with the thing it was about. Saying so
	// stops a narrowed picture reading as a container with nothing wrong with it.
	if n := p.graph.ElidedDiagnostics; n > 0 {
		lines = append(lines, esc(fmt.Sprintf("info: %d more about what this view leaves out", n)))
	}

	if len(lines) == 0 {
		return
	}

	p.printf("\tsubgraph cluster_notices {\n")
	p.printf("\t\tgraph [label=\"notices\", labeljust=l, style=rounded, color=%s, fontcolor=%s, class=%s];\n",
		quote(p.palette.warn), quote(p.palette.warn), quote("notices"))
	p.printf("\t\tnotices [shape=plaintext, style=\"\", label=<%s>];\n",
		small(9, p.palette.warn, strings.Join(lines, `<BR ALIGN="LEFT"/>`)))
	p.printf("\t}\n")
}

// notice is one line of the list: what the diagnostic is about, and what it says.
// A message can run to several lines, and this list has one line per notice.
func (p *printer) notice(d graph.LocatedDiagnostic) string {
	message := strings.ReplaceAll(d.Message, "\n", "; ")
	where := d.Where(p.graph)
	switch {
	case where == "":
	case message == "":
		message = where
	default:
		message = where + ": " + message
	}
	// A registration that never became a definition names nothing in the graph,
	// and the file and line are the only way back to it.
	if !d.Location.IsZero() {
		message += " (" + d.Location.String() + ")"
	}
	// Who is telling the reader this. godi's own checks and an extension's read
	// alike, so the pass is the only thing that says which of them found it.
	if d.Pass != "" {
		message += " [" + d.Pass + "]"
	}
	return message
}

// legend draws one real edge per channel rather than describing them. The point of
// a key is to show the mark.
//
// The two channels are independent. The head says how the dependency was matched.
// The colour says who decided on it.
func (p *printer) legend() {
	rows := []struct {
		style, arrow, colour, penWidth, text string
	}{
		{"solid", "normal", p.palette.muted, "1.2", "matched by exact type"},
		{"solid", "odiamond", p.palette.muted, "1.2", "matched through an interface binding"},
		{"solid", "normal", p.palette.manual, "1.2", "you decided"},
		{"dashed", "normal", p.palette.autowired, "1.2", "godi decided"},
		{"dashed", "normal", p.palette.pass, "2", "a compiler pass decided"},
	}

	p.printf("\tsubgraph cluster_legend {\n")
	p.printf("\t\tgraph [label=\"legend\", labeljust=l, style=rounded, color=%s, fontcolor=%s, class=%s];\n",
		quote(p.palette.clusterBorder), quote(p.palette.text), quote("legend"))

	for i, row := range rows {
		from, to := p.legendAnchor(i), p.legendCaptionNode(i)

		// An anchor with no shape, so the sample reads as a free-standing arrow.
		// It matches the caption's height, so both columns span the same distance
		// and every arrow comes out level.
		p.printf("\t\t%s [shape=none, style=\"\", label=\"\", width=0.02, height=%s];\n", from, legendRowHeight)
		// Every caption is boxed to the same width, so every sample arrow comes
		// out the same length. dot centres a node within its rank, so a short
		// caption would otherwise be pushed away from its own arrow.
		p.printf("\t\t%s [shape=plaintext, style=\"\", height=%s, label=%s];\n", to, legendRowHeight, p.legendCaption(row.text))
		// The weight asks dot to keep the sample level rather than let it slope
		// towards a neighbouring row.
		p.printf("\t\t%s -> %s [style=%s, arrowhead=%s, color=%s, penwidth=%s, weight=100];\n",
			from, to, quote(row.style), row.arrow, quote(row.colour), row.penWidth)
	}

	// Pin both columns. Without this the rows come out in whatever order the
	// layout settles on, and a row whose two halves land at different heights
	// gets a sloping arrow. Chaining last to first puts row zero on top.
	p.orderColumn(len(rows), p.legendAnchor)
	p.orderColumn(len(rows), p.legendCaptionNode)

	p.printf("\t}\n")

	p.reserveLegendRanks()
}

// orderColumn fixes the top-to-bottom order of one column of the key.
//
// minlen=0 matters. dot reserves room to route an edge between two nodes of the
// same rank, and at the default it pads the rows a third of an inch apart for
// edges that are never drawn.
func (p *printer) orderColumn(rows int, name func(int) string) {
	p.printf("\t\t{ rank=same; ")
	for i := rows - 1; i >= 0; i-- {
		p.printf("%s", name(i))
		if i > 0 {
			p.printf(" -> ")
		}
	}
	p.printf(" [style=invis, minlen=0, weight=100]; }\n")
}

// reserveLegendRanks keeps the key out of the graph's own columns.
//
// Ranks are global in dot. Without this the legend's two columns end up as far
// apart as the widest service table in the graph, and the sample arrows come out
// several inches long.
//
// Holding every source node back by two ranks leaves the first two columns to the
// legend, which keeps its arrows short.
func (p *printer) reserveLegendRanks() {
	p.printf("\t%s [style=invis, shape=point, width=0.01, height=0.01];\n", legendPad)

	for _, node := range p.graph.Nodes {
		if node.InDegree == 0 {
			p.printf("\t%s -> %s [style=invis, minlen=2];\n", legendPad, quote(string(node.ID)))
		}
	}
}

// legendCaption boxes the text at a fixed width so every row measures the same.
func (p *printer) legendCaption(text string) string {
	return fmt.Sprintf(
		`<<TABLE BORDER="0" CELLBORDER="0" CELLSPACING="0" CELLPADDING="0"><TR><TD WIDTH="185" ALIGN="LEFT">`+
			`<FONT POINT-SIZE="9" COLOR="%s">%s</FONT></TD></TR></TABLE>>`,
		p.palette.text, esc(text))
}

const (
	legendPad       = "legend_rank_pad"
	legendRowHeight = "0.2" // Keeps the rows close together; a plaintext node is half an inch tall by default.
)

func (p *printer) legendAnchor(row int) string { return fmt.Sprintf("legend_%d_from", row) }

func (p *printer) legendCaptionNode(row int) string { return fmt.Sprintf("legend_%d_to", row) }

func (p *printer) printf(format string, args ...any) {
	fmt.Fprintf(p.w, format, args...)
}

// clip keeps a long signature from stretching a node across the page. The full
// text is still in the tooltip.
func (p *printer) clip(s string) string {
	return render.Ellipsis(s, p.cfg.maxLabel)
}

// portName is the row anchor for one argument. Method names are Go identifiers,
// so the result is always a valid DOT port name.
func (p *printer) portName(param *graph.Param) string {
	switch param.Kind {
	case graph.InjectFunctionArg:
		return fmt.Sprintf("c%d", param.Index)
	case graph.InjectMethodArg, graph.InjectMethodReceiver:
		return fmt.Sprintf("m_%s_%d", param.Method, param.Index)
	case graph.InjectFactoryArg:
		return fmt.Sprintf("f%d", param.Index)
	}
	return fmt.Sprintf("f%d", param.Index)
}

func small(size int, colour, content string) string {
	if colour == "" {
		return fmt.Sprintf(`<FONT POINT-SIZE="%d">%s</FONT>`, size, content)
	}
	return fmt.Sprintf(`<FONT POINT-SIZE="%d" COLOR="%s">%s</FONT>`, size, colour, content)
}

// esc escapes text going into an HTML-like label. Go signatures are full of
// characters that would otherwise be read as markup, and "chan<- int" breaks a
// label without it.
//
// The backslash goes first. Graphviz substitutes \N, \G and friends inside
// HTML-like labels just as it does in quoted strings, so a literal such as
// `C:\Notes` would otherwise become the node's own name.
func esc(s string) string {
	return html.EscapeString(strings.ReplaceAll(s, `\`, `\\`))
}

// quote renders a DOT quoted string. The backslash is escaped before the quote,
// or an intentional escape would be doubled. Graphviz would also substitute the
// node's name for a literal "\N".
func quote(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	// Left-justify multi-line text. A trailing \l keeps the last line aligned.
	s = strings.ReplaceAll(s, "\n", `\l`)
	return `"` + s + `"`
}
