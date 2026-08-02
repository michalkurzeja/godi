package html

import (
	"fmt"
	"strings"

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

	// Diagnostics carry the ids they are about, not just their text, so the panel
	// can link to the node. The text and DOT notices leave that out.
	//
	// Some name nothing at all: a scope belonging to no definition, or a schema
	// this build does not know. Those have nowhere else on the page to go.
	Diagnostics []viewDiagnostic `json:"diagnostics,omitzero"`

	// SourceRoot is the directory the file paths hang off, and SourceLink the
	// template that turns one into something clickable. Empty means the page
	// shows locations as plain text.
	SourceRoot string `json:"sourceRoot,omitzero"`
	SourceLink string `json:"sourceLink,omitzero"`
}

// viewSnapshot carries the wording rather than the parts. What a partial graph has
// to tell the reader is the same in every format, so the model words it once.
type viewSnapshot struct {
	Label string   `json:"label"`
	Done  []string `json:"done,omitzero"`
}

type viewDiagnostic struct {
	Severity graph.Severity `json:"severity"`
	Message  string         `json:"message"`
	// Where names what the diagnostic is about, worded by the model so that the
	// panel and the other formats say the same thing.
	Where string        `json:"where,omitzero"`
	Pass  string        `json:"pass,omitzero"`
	Node  graph.NodeID  `json:"node,omitzero"`
	Param graph.ParamID `json:"param,omitzero"`
	Scope graph.ScopeID `json:"scope,omitzero"`
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
	// X", which reads well on a box but not at every level of a path.
	Owner string `json:"owner,omitzero"`
}

type viewNode struct {
	ID    graph.NodeID   `json:"id"`
	Kind  graph.NodeKind `json:"kind"`
	Scope graph.ScopeID  `json:"scope"`

	Type string `json:"type"`
	// Title and Subtitle are what the node is headed by and what goes under it.
	// The model works both out, so that every format gives the same answer.
	Title     string `json:"title"`
	Subtitle  string `json:"subtitle,omitzero"`
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
	// Incomplete is what the red border and the warning mark are drawn from.
	// Something this node needs is not there.
	Incomplete bool `json:"incomplete,omitzero"`
	// Elided is how many neighbours a filter cut off, so the page can say the
	// graph carries on where it stops.
	Elided int `json:"elided,omitzero"`

	Params []viewParam `json:"params,omitzero"`

	Registered *viewLocation `json:"registered,omitzero"`
	Declared   *viewLocation `json:"declared,omitzero"`
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

	Unwired    bool   `json:"unwired,omitzero"`
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
	// the filters both key off it, so it is settled once here.
	DecidedBy graph.ArgOrigin `json:"decidedBy"`
	// Pass names the compiler pass responsible, however it was responsible.
	Pass string `json:"pass,omitzero"`

	// Only the first hop, which is the binding applied to the declared type. No
	// view of the page shows the rest.
	BindInterface string           `json:"bindInterface,omitzero"`
	BindOrigin    graph.BindOrigin `json:"bindOrigin,omitzero"`
	BindPass      string           `json:"bindPass,omitzero"`

	Type    string `json:"type"`
	Ordinal int    `json:"ordinal"`
	OfMany  bool   `json:"ofMany,omitzero"`
	Cycle   bool   `json:"cycle,omitzero"`
}

func newPayload(g *graph.Graph, cfg config) payload {
	// Sized rather than grown, and never nil. The page iterates these three
	// without checking, and a nil slice marshals to null rather than to an empty
	// array. A container whose services are all unwired has no edges, and that
	// null stopped the viewer dead at boot.
	p := payload{
		Schema:     g.Schema,
		Title:      cfg.title,
		Credits:    cfg.credits(),
		SourceRoot: g.SourceRoot,
		SourceLink: cfg.sourceLink,
		Scopes:     make([]viewScope, 0, len(g.Scopes)),
		Nodes:      make([]viewNode, 0, len(g.Nodes)),
		Edges:      make([]viewEdge, 0, len(g.Edges)),
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
	for _, d := range g.AllDiagnostics() {
		p.Diagnostics = append(p.Diagnostics, viewDiagnostic{
			Severity: d.Severity,
			Message:  d.Message,
			Where:    d.Where(g),
			Pass:     d.Pass,
			Node:     d.Node,
			Param:    d.Param,
			Scope:    d.Scope,
		})
	}
	// A filter takes a diagnostic away with the thing it was about, and the panel
	// would otherwise read as a container with nothing wrong with it.
	if n := g.ElidedDiagnostics; n > 0 {
		p.Diagnostics = append(p.Diagnostics, viewDiagnostic{
			Severity: graph.SeverityInfo,
			Message:  fmt.Sprintf("%d more about what this view leaves out", n),
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
		Title:        node.Title(),
		Subtitle:     node.Subtitle(),
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
		Incomplete:   node.Faulty(),
		Elided:       node.Elided,
		Registered:   newViewLocation(node.Registered),
		Declared:     newViewLocation(node.Declared),
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
		Unwired:    param.Unwired(),
		Unresolved: param.Faulty(),
		Note:       paramNote(param),
	}
	for _, lit := range param.Literals {
		out.Literals = append(out.Literals, lit.String())
	}
	return out
}

// paramNote is what the page shows against an argument. The panel lists each
// diagnostic on its own; the row beside the argument has space for one line.
func paramNote(param *graph.Param) string {
	messages := make([]string, 0, len(param.Diagnostics))
	for _, d := range param.Diagnostics {
		if d.Message != "" {
			messages = append(messages, strings.ReplaceAll(d.Message, "\n", "; "))
		}
	}
	return strings.Join(messages, "; ")
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
	out.DecidedBy = edge.DecidedBy()
	if hop, ok := edge.Binding(); ok {
		out.BindInterface = render.Short(hop.Interface)
		out.BindOrigin = hop.Origin
		out.BindPass = hop.OriginPass
	}
	return out
}
