package html

import (
	"github.com/michalkurzeja/godi/v2/graph"
	"github.com/michalkurzeja/godi/v2/graph/internal/render"
)

// The payload is the model as the page's script wants it: flat, pre-shortened,
// and with the derived fields the script would otherwise have to recompute on
// every keystroke. It is a private contract between this package and its own
// JavaScript, not a published serialisation of the model.
type payload struct {
	Schema  string      `json:"schema"`
	Title   string      `json:"title"`
	Scopes  []viewScope `json:"scopes"`
	Nodes   []viewNode  `json:"nodes"`
	Edges   []viewEdge  `json:"edges"`
	Credits []credit    `json:"credits"`

	// Snapshot is set when the graph was taken while the container was still
	// being built. Nothing else on the page would give that away.
	Snapshot *viewSnapshot `json:"snapshot,omitzero"`

	// Diagnostics carry the ids they are about, not just their text: the panel
	// links to the node, which is the half the text and DOT notices leave out.
	// Some name nothing at all - a scope belonging to no definition, a schema
	// this build does not know - and those have nowhere else on the page to go.
	Diagnostics []viewDiagnostic `json:"diagnostics,omitzero"`

	// SourceRoot is the directory the file paths hang off, and SourceLink the
	// template that turns one into something clickable. Empty means the page
	// shows locations as plain text.
	SourceRoot string `json:"sourceRoot,omitzero"`
	SourceLink string `json:"sourceLink,omitzero"`
}

// viewSnapshot carries the wording rather than the parts: what a partial graph
// has to tell the reader is the same in every format, so it is written once, in
// the model.
type viewSnapshot struct {
	Label string   `json:"label"`
	Done  []string `json:"done,omitzero"`
}

type viewDiagnostic struct {
	Severity string        `json:"severity"`
	Message  string        `json:"message"`
	Node     graph.NodeID  `json:"node,omitzero"`
	Param    graph.ParamID `json:"param,omitzero"`
	Scope    graph.ScopeID `json:"scope,omitzero"`
}

// viewLocation is a place in the source, pre-rendered for display and kept in
// parts so that the page can still build a link out of it.
type viewLocation struct {
	Text string `json:"text"`
	File string `json:"file"`
	Line int    `json:"line"`
}

func newViewLocation(loc graph.Location) *viewLocation {
	if loc.IsZero() {
		return nil
	}
	return &viewLocation{Text: loc.String(), File: loc.File, Line: loc.Line}
}

type viewScope struct {
	ID     graph.ScopeID `json:"id"`
	Parent graph.ScopeID `json:"parent,omitzero"`
	Depth  int           `json:"depth"`
	Label  string        `json:"label"`
	// Owner is what declared the scope, on its own. The label says "children of
	// X", which reads well on a box but not repeated at every level of a path.
	Owner string `json:"owner,omitzero"`
}

type viewNode struct {
	ID    graph.NodeID   `json:"id"`
	Kind  graph.NodeKind `json:"kind"`
	Scope graph.ScopeID  `json:"scope"`

	Type      string `json:"type"`
	Short     string `json:"short"`
	Name      string `json:"name"`
	Package   string `json:"package,omitzero"`
	Signature string `json:"signature,omitzero"`

	Labels []string `json:"labels,omitzero"`

	Anonymous    bool `json:"anonymous,omitzero"`
	FromValue    bool `json:"fromValue,omitzero"`
	Lazy         bool `json:"lazy"`
	Shared       bool `json:"shared"`
	Autowired    bool `json:"autowired"`
	Instantiated bool `json:"instantiated"`
	Root         bool `json:"root"`
	// Incomplete is what the red border and the warning mark are drawn from:
	// something this node needs is not there.
	Incomplete bool `json:"incomplete,omitzero"`
	// Elided is how many neighbours a filter cut off, so the page can say the
	// graph carries on where it stops.
	Elided int `json:"elided,omitzero"`

	Params []viewParam `json:"params,omitzero"`

	Registered *viewLocation `json:"registered,omitzero"`
	Defined    *viewLocation `json:"defined,omitzero"`
}

type viewParam struct {
	ID     graph.ParamID       `json:"id"`
	Kind   graph.InjectionKind `json:"kind"`
	Method string              `json:"method,omitzero"`
	Index  int                 `json:"index"`

	Type  string `json:"type"`
	Short string `json:"short"`

	Origin     graph.ArgOrigin `json:"origin"`
	OriginPass string          `json:"originPass,omitzero"`
	Label      string          `json:"label,omitzero"`
	Literals   []string        `json:"literals,omitzero"`

	Unresolved bool   `json:"unresolved,omitzero"`
	Note       string `json:"note,omitzero"`
}

type viewEdge struct {
	ID    graph.EdgeID        `json:"id"`
	From  graph.NodeID        `json:"from"`
	To    graph.NodeID        `json:"to"`
	Param graph.ParamID       `json:"param"`
	Kind  graph.InjectionKind `json:"kind"`

	Origin     graph.ArgOrigin  `json:"origin"`
	OriginPass string           `json:"originPass,omitzero"`
	Resolution graph.Resolution `json:"resolution"`

	// DecidedBy is who chose this dependency: whoever created the binding it
	// resolved through, and otherwise whoever wired the argument. The colour and
	// the filters both key off it, so it is settled once here rather than twice
	// in the page - which is how they came to disagree.
	DecidedBy graph.ArgOrigin `json:"decidedBy"`
	// Pass names the compiler pass responsible, however it was responsible.
	Pass string `json:"pass,omitzero"`

	// Only the first hop, which is the binding applied to the declared type.
	// The rest are a detail no view of the page shows.
	BindInterface string           `json:"bindInterface,omitzero"`
	BindOrigin    graph.BindOrigin `json:"bindOrigin,omitzero"`
	BindPass      string           `json:"bindPass,omitzero"`

	Type    string `json:"type"`
	Ordinal int    `json:"ordinal"`
	OfMany  bool   `json:"ofMany,omitzero"`
	Cycle   bool   `json:"cycle,omitzero"`
}

func newPayload(g *graph.Graph, cfg config) payload {
	p := payload{
		Schema:     g.Schema,
		Title:      cfg.title,
		Credits:    cfg.credits(),
		SourceRoot: g.SourceRoot,
		SourceLink: cfg.sourceLink,
	}

	for _, scope := range g.Scopes {
		p.Scopes = append(p.Scopes, viewScope{
			ID:     scope.ID,
			Parent: scope.Parent,
			Depth:  scope.Depth,
			Label:  scope.Label(),
			Owner:  render.Short(scope.OwnerName),
		})
	}
	for _, node := range g.Nodes {
		p.Nodes = append(p.Nodes, newViewNode(node))
	}
	for _, edge := range g.Edges {
		p.Edges = append(p.Edges, newViewEdge(edge))
	}
	if g.Partial() {
		p.Snapshot = &viewSnapshot{Label: g.Snapshot.Label(), Done: g.Snapshot.Done}
	}
	for _, d := range g.Diagnostics {
		p.Diagnostics = append(p.Diagnostics, viewDiagnostic{
			Severity: d.Severity,
			Message:  d.Message,
			Node:     d.Node,
			Param:    d.Param,
			Scope:    d.Scope,
		})
	}

	return p
}

func newViewNode(node *graph.Node) viewNode {
	out := viewNode{
		ID:           node.ID,
		Kind:         node.Kind,
		Scope:        node.Scope,
		Type:         node.Type,
		Short:        node.TypeShort(),
		Name:         node.Name,
		Package:      node.Package(),
		Signature:    node.Signature,
		Labels:       node.Labels,
		Anonymous:    node.Anonymous(),
		FromValue:    node.FromValue,
		Lazy:         node.Lazy,
		Shared:       node.Shared,
		Autowired:    node.Autowired,
		Instantiated: node.Instantiated,
		Root:         node.Root,
		Incomplete:   node.Incomplete,
		Elided:       node.Elided,
		Registered:   newViewLocation(node.Registered),
		Defined:      newViewLocation(node.Defined),
	}

	for _, param := range node.Params {
		out.Params = append(out.Params, newViewParam(param))
	}

	return out
}

func newViewParam(param *graph.Param) viewParam {
	out := viewParam{
		ID:         param.ID,
		Kind:       param.Kind,
		Method:     param.Method,
		Index:      param.Index,
		Type:       param.Type,
		Short:      param.TypeShort(),
		Origin:     param.Origin,
		OriginPass: param.OriginPass,
		Label:      param.Label,
		Unresolved: param.Unresolved,
		Note:       param.Note,
	}
	for _, lit := range param.Literals {
		out.Literals = append(out.Literals, lit.String())
	}
	return out
}

func newViewEdge(edge *graph.Edge) viewEdge {
	out := viewEdge{
		ID:         edge.ID,
		From:       edge.From,
		To:         edge.To,
		Param:      edge.Param,
		Kind:       edge.Kind,
		Origin:     edge.Origin,
		OriginPass: edge.OriginPass,
		Resolution: edge.Resolution,
		Type:       edge.ParamType,
		Ordinal:    edge.Ordinal,
		OfMany:     edge.OfMany,
		Cycle:      edge.Cycle,
		Pass:       edge.PassCredit(),
	}
	out.DecidedBy = edge.Origin
	if hop, ok := edge.Binding(); ok {
		out.BindInterface = hop.Interface
		out.BindOrigin = hop.Origin
		out.BindPass = hop.OriginPass
		out.DecidedBy = decidedBy(hop.Origin)
	}
	return out
}

// decidedBy translates a binding's origin into the argument vocabulary the page
// filters by. godi creating a binding for you and godi autowiring an argument
// are the same answer to "who decided": godi did.
func decidedBy(origin graph.BindOrigin) graph.ArgOrigin {
	switch origin {
	case graph.BindOriginManual:
		return graph.ArgOriginManual
	case graph.BindOriginAutobinding:
		return graph.ArgOriginAutowiring
	case graph.BindOriginCompilerPass:
		return graph.ArgOriginCompilerPass
	}
	return graph.ArgOriginNone
}

// literalText renders one constant for display. The type is not repeated: it is
// already on the argument row this sits beside. A value is only ever present
// when the caller asked for values.
func literalText(lit graph.Literal) string {
	switch {
	case lit.Value == "":
		return "‹literal›"
	case lit.Truncated:
		return lit.Value + "…"
	default:
		return lit.Value
	}
}
