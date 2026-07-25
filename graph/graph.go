package graph

import (
	"fmt"
	"io"
	"strings"

	"github.com/michalkurzeja/godi/v2/graph/internal/render"
	"github.com/michalkurzeja/godi/v2/internal/errorsx"
)

// Schema identifies the shape of the model, for anything that serialises it.
const Schema = "godi.graph/v1"

type (
	// ScopeID is a readable path: "root", or the node ID of the scope's owner.
	ScopeID string
	// NodeID is a readable, build-stable identifier: "root/svc:http.(*Server)".
	NodeID string
	// ParamID identifies an injection point: "<NodeID>#f:2", "<NodeID>#m:SetLogger:1".
	ParamID string
	// EdgeID identifies one dependency injected into one injection point:
	// "<ParamID>@0".
	EdgeID string
)

// NewEdgeID names the ordinal-th dependency injected into a param. The pair is
// unique, so it identifies the edge without the model having to hand out
// counters.
func NewEdgeID(param ParamID, ordinal int) EdgeID {
	return EdgeID(fmt.Sprintf("%s@%d", param, ordinal))
}

// Graph is the dependency graph of a container: what depends on what, how each
// dependency was wired, and what was passed where.
//
// It is plain data. Nothing here refers to the container it came from, which is
// what lets encoders live outside godi entirely.
type Graph struct {
	Schema      string        `json:"schema"`
	Scopes      []*Scope      `json:"scopes"`   // Depth first from the root.
	Nodes       []*Node       `json:"nodes"`    // Sorted by ID.
	Edges       []*Edge       `json:"edges"`    // Sorted by (From, Param, Ordinal).
	Bindings    []*Binding    `json:"bindings"` // Sorted by (Scope, Interface).
	Diagnostics []*Diagnostic `json:"diagnostics,omitempty"`

	// SourceRoot is the directory every file path is relative to, when they
	// share one. It is empty when they do not, and then the paths are as the
	// runtime gave them. Trimming it keeps output readable and stable across
	// machines; joining it back onto a path returns the original.
	SourceRoot string `json:"sourceRoot,omitzero"`

	// Lookup indexes, built on first use.
	nodes  map[NodeID]*Node
	params map[ParamID]*Param
	scopes map[ScopeID]*Scope
	out    map[NodeID][]*Edge
	in     map[NodeID][]*Edge
}

// Scope is a group of definitions. Child scopes hold services private to the
// definition that declared them.
type Scope struct {
	ID     ScopeID `json:"id"`
	Parent ScopeID `json:"parent,omitzero"` // Empty for the root scope.
	Depth  int     `json:"depth"`
	Name   string  `json:"name"`           // The container's own name for it; a uuid for child scopes.
	Owner  NodeID  `json:"owner,omitzero"` // The node that declared this scope.
}

// Label names a scope for a reader. A child scope's own name is the uuid of the
// definition that declared it, which says nothing, so it is named after that
// definition instead. Every encoder owes the reader the same answer, which is
// why this is on the model rather than in each of them.
func (s *Scope) Label() string {
	if s.Owner == "" {
		return string(s.ID)
	}
	owner := render.Short(string(s.Owner))
	if _, name, ok := strings.Cut(owner, ":"); ok {
		owner = name
	}
	return "children of " + owner
}

// NodeKind tells a service apart from a function.
type NodeKind string

const (
	NodeService  NodeKind = "service"
	NodeFunction NodeKind = "function"
)

// Node is a service or a function definition.
type Node struct {
	ID    NodeID   `json:"id"`
	Kind  NodeKind `json:"kind"`
	UUID  string   `json:"uuid"` // The container's own ID, for runtime lookups.
	Scope ScopeID  `json:"scope"`

	Type string `json:"type"` // Fully qualified: "github.com/acme/app/http.(*Server)".
	Name string `json:"name"` // Factory name for services, function name for functions.
	// Signature is what the factory or function accepts and returns, as Go
	// would write it: "func(*http.Router, app.Logger) *http.Server". It says in
	// one line what the argument rows say one at a time.
	Signature string   `json:"signature,omitzero"`
	Labels    []string `json:"labels,omitzero"`

	Lazy      bool `json:"lazy"`
	Shared    bool `json:"shared"` // Always false for functions.
	Autowired bool `json:"autowired"`

	ChildScope ScopeID  `json:"childScope,omitzero"` // The scope this node declared, if any.
	Params     []*Param `json:"params,omitzero"`

	// Registered is where the definition was written: the di.Svc or di.Func
	// call, or the compiler pass that created it.
	Registered Location `json:"registered,omitzero"`
	// Defined is where the factory or function itself is declared. It is empty
	// when the factory is one godi synthesised, as SvcVal does, because then
	// there is no source of the user's to point at.
	Defined Location `json:"defined,omitzero"`

	InDegree  int `json:"inDegree"`
	OutDegree int `json:"outDegree"`
	// Root reports that nothing in the container injects this node: it is the
	// top of a dependency tree. That is either an entry point, or wiring
	// nothing uses, and the container cannot tell the two apart - which is the
	// point of showing them.
	//
	// It is a fact about the wiring rather than a guess about intent, unlike
	// asking what is reachable, which needs a set of entry points the container
	// does not have. A cycle has no member of in-degree zero, so a cyclic
	// component has no root.
	Root bool `json:"root"`
	// Instantiated reports whether the container had built this service by the
	// time the graph was taken.
	Instantiated bool `json:"instantiated"`
	// Elided is how many of this node's neighbours a filter cut off. It is only
	// ever set by a limit - a focus that ran out of hops, a node cap - never by
	// an exclusion the caller named, because the point of it is to say that the
	// graph carries on where the picture stops.
	Elided int `json:"elided,omitzero"`
}

// TypeShort is Type without the package path, for labels.
func (n *Node) TypeShort() string { return render.Short(n.Type) }

// NameShort is Name without the package path, for labels.
func (n *Node) NameShort() string { return render.Short(n.Name) }

// Anonymous reports whether this node is a function literal.
//
// Those have no name of their own, so what the graph carries is what the
// runtime calls them: the function that encloses them and a counter, as in
// "main.build.func1", with a further counter for each level of nesting. Worth
// knowing because a name like that identifies a function without describing it,
// and its signature is the only thing that does.
// "func1" on its own is a name someone can perfectly well choose, so the marker
// alone is not enough to go on. What settles it is that a literal is named
// after whatever encloses it: inside its own package there is always a
// qualifier in front of the counter, and a function of one's own has nothing in
// front of it at all.
//
// The one case this cannot separate is a method someone called "func1", which
// is spelled exactly as a literal inside a method called anything else. Nothing
// in the name distinguishes them, and the cost of being wrong is a line of
// signature shown or not shown.
func (n *Node) Anonymous() bool {
	if n.Kind != NodeFunction {
		return false
	}

	// What the name says within its own package: "build.func1" for a literal
	// declared inside build, "migrate" for a function of its own.
	local := strings.TrimPrefix(n.Name, render.Package(n.Name)+".")

	for {
		dot := strings.LastIndexByte(local, '.')
		if dot < 0 {
			return false
		}

		part := local[dot+1:]
		if counter, ok := strings.CutPrefix(part, "func"); ok && digits(counter) {
			return true
		}
		// A nested literal adds a bare counter of its own; walk back past those
		// to reach the marker.
		if !digits(part) {
			return false
		}
		local = local[:dot]
	}
}

func digits(s string) bool {
	if s == "" {
		return false
	}
	for i := range len(s) {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// Package is the import path this node belongs to.
//
// It is worth having on its own because the qualified type is not: a generic
// names its type arguments in full, so the one line that would tell you where a
// service lives is the one line nobody can read. The package answers that
// question and nothing else.
//
// A function's type is only a signature, and a service can be of a builtin
// type; either way the factory that made it has a package when the type does
// not.
func (n *Node) Package() string {
	if n.Kind == NodeService {
		if pkg := render.Package(n.Type); pkg != "" {
			return pkg
		}
	}
	return render.Package(n.Name)
}

// Location is a place in the source. Paths are relative to the graph's
// SourceRoot when it has one.
//
// What the paths look like depends on how the binary was built: a release built
// with -trimpath reports module-relative paths that no editor can open, so a
// location is worth showing but not worth promising to resolve.
type Location struct {
	File string `json:"file"`
	Line int    `json:"line"`
	Func string `json:"func,omitzero"` // The function that registered it, where that is known.
}

// IsZero reports whether nothing was recorded. It is what lets a Location be
// left out of the encoded graph entirely.
func (l Location) IsZero() bool { return l.File == "" && l.Line == 0 }

func (l Location) String() string {
	switch {
	case l.File == "":
		return ""
	case l.Line == 0:
		return l.File
	default:
		return fmt.Sprintf("%s:%d", l.File, l.Line)
	}
}

// InjectionKind tells where an argument is injected.
type InjectionKind string

const (
	InjectFactoryArg     InjectionKind = "factory-arg"
	InjectFunctionArg    InjectionKind = "function-arg"
	InjectMethodArg      InjectionKind = "method-arg"
	InjectMethodReceiver InjectionKind = "method-receiver"
)

// ArgOrigin says who wired an argument.
type ArgOrigin string

const (
	ArgOriginNone         ArgOrigin = "none"          // Nothing wired it; only possible before autowiring runs.
	ArgOriginManual       ArgOrigin = "manual"        // The user wired it, at definition time.
	ArgOriginAutowiring   ArgOrigin = "autowiring"    // godi's autowiring wired it.
	ArgOriginCompilerPass ArgOrigin = "compiler-pass" // A compiler pass wired it.
)

// ArgKind is the kind of argument filling a slot.
type ArgKind string

const (
	ArgKindNone          ArgKind = "none"
	ArgKindLiteral       ArgKind = "literal"
	ArgKindRef           ArgKind = "ref"
	ArgKindType          ArgKind = "type"
	ArgKindLabel         ArgKind = "label"
	ArgKindFlexibleSlice ArgKind = "flexible-slice"
	ArgKindCompound      ArgKind = "compound"
	ArgKindUnknown       ArgKind = "unknown"
)

// Param is one injection point: a single argument slot of a factory, function
// or method call. It produces zero edges for a literal, one for an ordinary
// dependency, and many for a slice or variadic slot.
type Param struct {
	ID     ParamID       `json:"id"`
	Node   NodeID        `json:"node"`
	Kind   InjectionKind `json:"kind"`
	Method string        `json:"method,omitzero"` // Set for method arguments.
	Index  int           `json:"index"`           // Slot index; method arguments start at 1.

	Type     string `json:"type"`
	ElemType string `json:"elemType,omitzero"` // Element type, for slice slots.
	Slice    bool   `json:"slice"`
	Variadic bool   `json:"variadic"`

	Origin     ArgOrigin `json:"origin"`
	OriginPass string    `json:"originPass,omitzero"` // Pass name, when a pass wired it.
	Arg        ArgKind   `json:"arg"`
	Label      string    `json:"label,omitzero"`
	Literals   []Literal `json:"literals,omitzero"`

	EdgeCount  int    `json:"edgeCount"`
	Unresolved bool   `json:"unresolved,omitzero"`
	Note       string `json:"note,omitzero"`
}

// TypeShort is Type without the package path, for labels.
func (p *Param) TypeShort() string { return render.Short(p.Type) }

// Literal is a constant passed to a factory. Values are omitted unless asked
// for: literals routinely carry credentials.
type Literal struct {
	Type      string `json:"type"`
	Value     string `json:"value,omitzero"`
	Truncated bool   `json:"truncated,omitzero"`
	Redacted  bool   `json:"redacted,omitzero"`
}

// String renders one constant for display. The type is deliberately not
// repeated: every encoder already shows it on the argument row this sits
// beside. It lives on the model because three encoders rendering the same
// constant three ways is three chances to disagree.
func (l Literal) String() string {
	switch {
	case l.Value != "" && l.Truncated:
		return l.Value + "…"
	case l.Value != "":
		// A redactor's replacement lands here, which is the point of it.
		return l.Value
	case l.Redacted:
		return "‹redacted›"
	default:
		return "‹literal›"
	}
}

// LiteralsText renders every constant on this argument as one string.
func (p *Param) LiteralsText() string {
	parts := make([]string, 0, len(p.Literals))
	for _, lit := range p.Literals {
		parts = append(parts, lit.String())
	}
	return strings.Join(parts, ", ")
}

// Resolution is the mechanism that matched a dependency to a param.
type Resolution string

const (
	ResolutionRef         Resolution = "ref"             // An explicit reference to a definition.
	ResolutionByType      Resolution = "by-type"         // Matched against the registry by type.
	ResolutionBySliceType Resolution = "by-slice-type"   // A slice slot matched a []T service.
	ResolutionByElemType  Resolution = "by-element-type" // A slice slot collected T services.
	ResolutionByLabel     Resolution = "by-label"
)

// Edge is one dependency injected into one param.
//
// Provenance has two independent facets: who wired the argument (Origin,
// OriginPass) and how it resolved to this target (Resolution, Bindings).
type Edge struct {
	ID    EdgeID        `json:"id"`
	From  NodeID        `json:"from"` // The consumer.
	To    NodeID        `json:"to"`   // The dependency.
	Param ParamID       `json:"param"`
	Kind  InjectionKind `json:"kind"` // Copied from the param, so encoders need no lookup.

	Origin     ArgOrigin    `json:"origin"`
	OriginPass string       `json:"originPass,omitzero"`
	Resolution Resolution   `json:"resolution"`
	Bindings   []BindingHop `json:"bindings,omitzero"` // Interface bindings traversed.

	ParamType string `json:"paramType"` // What the consumer declared.
	Ordinal   int    `json:"ordinal"`   // Position among the param's edges, i.e. in the injected slice.
	OfMany    bool   `json:"ofMany"`
	Cycle     bool   `json:"cycle,omitzero"`
}

// PassCredit names the compiler pass responsible for this edge, and is empty
// when none is. godi's own automation is deliberately not named: it is the
// default the reader already knows.
//
// A pass can be responsible in either of two ways - by wiring the argument, or
// by creating the binding the argument resolved through - and the answer is the
// same either way, so callers need not know which happened. Only when two
// different passes each had a hand in the same edge does it say which did what.
//
// It lives on the model because every encoder owes the reader the same answer.
func (e *Edge) PassCredit() string {
	var wiredBy, boundBy string
	if e.Origin == ArgOriginCompilerPass {
		wiredBy = e.OriginPass
	}
	if hop, ok := e.Binding(); ok && hop.Origin == BindOriginCompilerPass {
		boundBy = hop.OriginPass
	}

	switch {
	case wiredBy != "" && boundBy != "" && wiredBy != boundBy:
		return "arg: " + wiredBy + ", bind: " + boundBy
	case wiredBy != "":
		return wiredBy
	default:
		return boundBy
	}
}

// Binding returns the first interface binding this edge resolved through, which
// is the one applied to the declared parameter type.
func (e *Edge) Binding() (BindingHop, bool) {
	if len(e.Bindings) == 0 {
		return BindingHop{}, false
	}
	return e.Bindings[0], true
}

// BindOrigin says who created an interface binding.
type BindOrigin string

const (
	BindOriginManual       BindOrigin = "manual"        // The user declared it.
	BindOriginAutobinding  BindOrigin = "autobinding"   // godi created it automatically.
	BindOriginCompilerPass BindOrigin = "compiler-pass" // A compiler pass created it.
)

// BindingHop is one interface binding traversed while resolving an edge.
// Bindings chain, so an edge can traverse several.
type BindingHop struct {
	Interface  string     `json:"interface"`
	Scope      ScopeID    `json:"scope"`
	Origin     BindOrigin `json:"origin"`
	OriginPass string     `json:"originPass,omitzero"`
}

// Binding is an interface binding declared in, or created for, a scope.
type Binding struct {
	Scope      ScopeID    `json:"scope"`
	Interface  string     `json:"interface"`
	Origin     BindOrigin `json:"origin"`
	OriginPass string     `json:"originPass,omitzero"`
	BoundTo    string     `json:"boundTo"`
	Targets    []NodeID   `json:"targets,omitzero"`
	// EdgeCount is how many edges resolved through this binding. Zero means it
	// is declared but unused.
	EdgeCount int `json:"edgeCount"`
}

// Diagnostic reports something the extractor could not make sense of. Extraction
// never fails on odd input; it records it here instead.
type Diagnostic struct {
	Severity string  `json:"severity"`
	Scope    ScopeID `json:"scope,omitzero"`
	Node     NodeID  `json:"node,omitzero"`
	Param    ParamID `json:"param,omitzero"`
	Message  string  `json:"message"`
}

// Node returns the node with the given ID.
func (g *Graph) Node(id NodeID) (*Node, bool) {
	g.index()
	n, ok := g.nodes[id]
	return n, ok
}

// Param returns the injection point with the given ID.
func (g *Graph) Param(id ParamID) (*Param, bool) {
	g.index()
	p, ok := g.params[id]
	return p, ok
}

// Scope returns the scope with the given ID.
func (g *Graph) Scope(id ScopeID) (*Scope, bool) {
	g.index()
	s, ok := g.scopes[id]
	return s, ok
}

// OutEdges returns the edges from the given node to its dependencies.
func (g *Graph) OutEdges(id NodeID) []*Edge {
	g.index()
	return g.out[id]
}

// InEdges returns the edges from the consumers of the given node.
func (g *Graph) InEdges(id NodeID) []*Edge {
	g.index()
	return g.in[id]
}

// ChildScopes returns the scopes directly nested in the given one.
func (g *Graph) ChildScopes(id ScopeID) []*Scope {
	var out []*Scope
	for _, s := range g.Scopes {
		if s.Parent == id && s.ID != id {
			out = append(out, s)
		}
	}
	return out
}

// ScopeNodes returns the nodes registered in the given scope.
func (g *Graph) ScopeNodes(id ScopeID) []*Node {
	var out []*Node
	for _, n := range g.Nodes {
		if n.Scope == id {
			out = append(out, n)
		}
	}
	return out
}

// Encode writes the graph to w in the encoder's format.
func (g *Graph) Encode(w io.Writer, enc Encoder) error {
	if g == nil {
		return fmt.Errorf("graph: cannot encode a nil graph")
	}
	if v, ok := enc.(Validator); ok {
		if err := v.Check(g); err != nil {
			return errorsx.Wrapf(err, "graph: %s cannot encode this graph", enc.Format().Name)
		}
	}
	if err := enc.Encode(g, w); err != nil {
		return errorsx.Wrapf(err, "graph: %s encoding failed", enc.Format().Name)
	}
	return nil
}

func (g *Graph) index() {
	if g.nodes != nil {
		return
	}

	g.nodes = make(map[NodeID]*Node, len(g.Nodes))
	g.params = make(map[ParamID]*Param)
	g.scopes = make(map[ScopeID]*Scope, len(g.Scopes))
	g.out = make(map[NodeID][]*Edge)
	g.in = make(map[NodeID][]*Edge)

	for _, s := range g.Scopes {
		g.scopes[s.ID] = s
	}
	for _, n := range g.Nodes {
		g.nodes[n.ID] = n
		for _, p := range n.Params {
			g.params[p.ID] = p
		}
	}
	for _, e := range g.Edges {
		g.out[e.From] = append(g.out[e.From], e)
		g.in[e.To] = append(g.in[e.To], e)
	}
}
