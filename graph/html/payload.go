package html

import (
	"strings"

	"github.com/michalkurzeja/godi/v2/graph"
	"github.com/michalkurzeja/godi/v2/graph/internal/render"
)

// The payload is the model as the page's script wants it: flat, pre-shortened,
// and with the derived fields the script would otherwise have to recompute on
// every keystroke. It is a private contract between this package and its own
// JavaScript, not a published serialisation of the model.
type payload struct {
	Schema  string         `json:"schema"`
	Title   string         `json:"title"`
	Scopes  []viewScope    `json:"scopes"`
	Nodes   []viewNode     `json:"nodes"`
	Edges   []viewEdge     `json:"edges"`
	Roots   []graph.NodeID `json:"roots"`
	Notices []string       `json:"notices,omitzero"`
	Credits []credit       `json:"credits"`

	// SourceRoot is the directory the file paths hang off, and SourceLink the
	// template that turns one into something clickable. Empty means the page
	// shows locations as plain text.
	SourceRoot string `json:"sourceRoot,omitzero"`
	SourceLink string `json:"sourceLink,omitzero"`
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
}

type viewNode struct {
	ID    graph.NodeID   `json:"id"`
	Kind  graph.NodeKind `json:"kind"`
	Scope graph.ScopeID  `json:"scope"`

	Type      string `json:"type"`
	Short     string `json:"short"`
	Name      string `json:"name"`
	Signature string `json:"signature,omitzero"`

	Labels []string `json:"labels,omitzero"`

	Lazy         bool `json:"lazy"`
	Shared       bool `json:"shared"`
	Autowired    bool `json:"autowired"`
	Reachable    bool `json:"reachable"`
	Instantiated bool `json:"instantiated"`

	Params []viewParam `json:"params,omitzero"`

	Registered *viewLocation `json:"registered,omitzero"`
	Defined    *viewLocation `json:"defined,omitzero"`

	// Search is every searchable string of the node, lowercased and joined, so
	// that filtering as the reader types is one substring test per node.
	Search string `json:"search"`
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
		Roots:      g.Roots,
		Credits:    cfg.credits(),
		SourceRoot: g.SourceRoot,
		SourceLink: cfg.sourceLink,
	}

	for _, scope := range g.Scopes {
		p.Scopes = append(p.Scopes, viewScope{
			ID:     scope.ID,
			Parent: scope.Parent,
			Depth:  scope.Depth,
			Label:  scopeLabel(scope),
		})
	}
	for _, node := range g.Nodes {
		p.Nodes = append(p.Nodes, newViewNode(node))
	}
	for _, edge := range g.Edges {
		p.Edges = append(p.Edges, newViewEdge(edge))
	}
	for _, d := range g.Diagnostics {
		p.Notices = append(p.Notices, d.Message)
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
		Signature:    node.Signature,
		Labels:       node.Labels,
		Lazy:         node.Lazy,
		Shared:       node.Shared,
		Autowired:    node.Autowired,
		Reachable:    node.ReachableFromRoots,
		Instantiated: node.Instantiated,
		Registered:   newViewLocation(node.Registered),
		Defined:      newViewLocation(node.Defined),
	}

	for _, param := range node.Params {
		out.Params = append(out.Params, newViewParam(param))
	}

	terms := append([]string{node.Type, node.Name, string(node.Scope)}, node.Labels...)
	out.Search = strings.ToLower(strings.Join(terms, " "))

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
		out.Literals = append(out.Literals, literalText(lit))
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

// scopeLabel names a scope for a reader. A child scope's own name is the uuid
// of the definition that declared it, which says nothing.
func scopeLabel(scope *graph.Scope) string {
	if scope.Owner == "" {
		return string(scope.ID)
	}
	owner := render.Short(string(scope.Owner))
	if _, name, ok := strings.Cut(owner, ":"); ok {
		owner = name
	}
	return "children of " + owner
}
