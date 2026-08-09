package graph

import (
	"fmt"
	"io"
	"strings"
	"sync"

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
// It is plain data. Nothing here refers to the container it came from, so an
// encoder can live outside godi.
type Graph struct {
	// Schema is not written here. Serialising a graph wraps it in an envelope
	// whose metadata carries the schema, so the file says it once. See
	// WriteJSON.
	Schema   string     `json:"-"`
	Scopes   []*Scope   `json:"scopes"`   // Depth first from the root.
	Nodes    []*Node    `json:"nodes"`    // Sorted by ID.
	Edges    []*Edge    `json:"edges"`    // Sorted by (From, Param, Ordinal).
	Bindings []*Binding `json:"bindings"` // Sorted by (Scope, Interface).

	// Diagnostics are what belongs to no scope, node or argument: a compiler pass
	// that could not be scheduled, a definition that never made it into the
	// container, a file written against a schema this build does not know. It is
	// where a diagnostic goes when there is nothing narrower to put it on.
	Diagnostics []Diagnostic `json:"diagnostics,omitzero"`
	// ElidedDiagnostics is how many a filter cut off with the elements they were
	// about. Without it, narrowing a graph would quietly answer "nothing is wrong"
	// about a container that is broken elsewhere.
	ElidedDiagnostics int `json:"elidedDiagnostics,omitzero"`

	// Snapshot is set when the graph was taken from a container that was not
	// finished being built. It is nil for a built container. That is the only
	// graph whose wiring nobody will change again.
	Snapshot *Snapshot `json:"snapshot,omitzero"`

	// SourceRoot is the directory every file path is relative to, when they
	// share one. It is empty when they do not, and then the paths are as the
	// runtime gave them. Trimming it keeps output readable and stable across
	// machines. Joining it back onto a path returns the original.
	SourceRoot string `json:"sourceRoot,omitzero"`

	// Lookup indexes, built on first use. A Graph is handed to concurrent
	// readers (graph.Static serves the same *Graph to every HTTP request), so
	// building them is guarded rather than left to a bare nil check.
	indexOnce sync.Once
	nodes     map[NodeID]*Node
	params    map[ParamID]*Param
	scopes    map[ScopeID]*Scope
	out       map[NodeID][]*Edge
	in        map[NodeID][]*Edge
}

// Scope is a group of definitions. Child scopes hold services private to the
// definition that declared them.
type Scope struct {
	ID     ScopeID `json:"id"`
	Parent ScopeID `json:"parent,omitzero"` // Empty for the root scope.
	Depth  int     `json:"depth"`
	Name   string  `json:"name"`           // The container's own name for it; a uuid for child scopes.
	Owner  NodeID  `json:"owner,omitzero"` // The node that declared this scope.
	// OwnerName is what that node is called, fully qualified. Owner is a path
	// built for uniqueness, so it is too cluttered to derive this from.
	OwnerName string `json:"ownerName,omitzero"`

	// Diagnostics are what is worth saying about this scope: that no definition
	// owns it, or whatever a compiler pass reported here.
	Diagnostics []Diagnostic `json:"diagnostics,omitzero"`
}

// Label names a scope for a reader. A child scope's own name is a uuid, which
// says nothing, so it is named after the definition that declared it.
//
// It is on the model so that every format gives the same answer.
func (s *Scope) Label() string {
	if s.OwnerName == "" {
		return string(s.ID)
	}
	return "children of " + render.Short(s.OwnerName)
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
	// FromValue reports that the service was handed to the container as a value
	// rather than built by a factory. Name, Signature and Declared then describe
	// that value, if it is a function. They are empty if it is not: a struct
	// someone passed in has nothing to name.
	FromValue bool `json:"fromValue,omitzero"`

	ChildScope ScopeID  `json:"childScope,omitzero"` // The scope this node declared, if any.
	Params     []*Param `json:"params,omitzero"`

	// Registered is where the definition was written: the di.Svc or di.Func
	// call, or the compiler pass that created it.
	Registered Location `json:"registered,omitzero"`
	// Declared is where the factory or function itself is written. It is empty
	// when the factory is one godi synthesised, as SvcVal does, because then
	// there is no source of the user's to point at.
	Declared Location `json:"declared,omitzero"`

	InDegree  int `json:"inDegree"`
	OutDegree int `json:"outDegree"`
	// Root reports that nothing in the container injects this node: it is the top
	// of a dependency tree. That is either an entry point or wiring nothing uses.
	// The container cannot tell the two apart, which is why it shows both.
	//
	// It is a fact about the wiring, not a guess about intent. Asking what is
	// reachable instead would need a set of entry points the container does not
	// have. Note that a cycle has no member of in-degree zero, so a cyclic
	// component has no root.
	Root bool `json:"root"`
	// Instantiated reports whether the container had built this service by the
	// time the graph was taken.
	Instantiated bool `json:"instantiated"`
	// Elided is how many of this node's neighbours a filter cut off. Only a limit
	// sets it: a focus that ran out of hops, or a node cap. An exclusion the
	// caller named does not, because the point is to say that the graph carries
	// on where the picture stops.
	Elided int `json:"elided,omitzero"`

	// Diagnostics are what is wrong with this definition rather than with one of
	// its arguments: a factory that returned an error, a service the compiler
	// could not build. What is wrong with an argument is on the argument.
	Diagnostics []Diagnostic `json:"diagnostics,omitzero"`
}

// Faulty reports that this node is broken: either it carries an error of its own,
// or one of its arguments does. It is a question rather than a stored flag, so
// what a reader is told and what is drawn as broken cannot disagree.
func (n *Node) Faulty() bool {
	if hasError(n.Diagnostics) {
		return true
	}
	for _, p := range n.Params {
		if p.Faulty() {
			return true
		}
	}
	return false
}

// TypeShort is Type without the package path, for labels.
func (n *Node) TypeShort() string { return render.Short(n.Type) }

// NameShort is Name without the package path, for labels.
func (n *Node) NameShort() string { return render.Short(n.Name) }

// KnownByName reports whether a reader knows this node by its name rather than
// by the type it provides.
//
// A service is known by its type. A function is known by its name, because a
// function's type is only its signature.
//
// A service registered as a function value is known by its name too: its type is
// that function's signature, so the type adds nothing. The exception is a
// literal, whose name the runtime made up. That name identifies the function
// without describing it, so the signature is the better heading.
//
// It is on the model so that every format gives the same answer.
func (n *Node) KnownByName() bool {
	return n.Kind == NodeFunction || (n.FromValue && n.Name != "" && !n.Anonymous())
}

// Title is what the node is headed by: its name or its type, whichever it is
// known by.
func (n *Node) Title() string {
	if n.KnownByName() {
		return n.NameShort()
	}
	return n.TypeShort()
}

// Subtitle is the line under the heading: whichever of the name and the
// signature the title did not take.
//
// Where the name is the heading, what is left to say is what the function takes
// and returns. That covers a function, a service registered as a named function
// value, and a function literal, whose made-up name describes nothing.
//
// It is empty when there is nothing to add. The caller drops the line when it
// repeats the title.
func (n *Node) Subtitle() string {
	if !n.KnownByName() && !n.Anonymous() {
		return n.NameShort()
	}
	if n.Signature == "" {
		return n.TypeShort()
	}
	return render.Short(n.Signature)
}

// Anonymous reports whether the code behind this node is a function literal:
// the function itself for a function node, and for a service either the factory
// that builds it or the value it was registered as.
//
// A literal has no name of its own. What the graph carries is what the runtime
// calls it: the enclosing function and a counter, as in "main.build.func1", with
// a further counter per level of nesting. Such a name identifies a function
// without describing it, so the signature is the only thing that does.
//
// "func1" alone is a name someone can choose, so the counter is not enough to go
// on. What settles it is that a literal is named after whatever encloses it.
// Inside its own package there is always a qualifier in front of the counter, and
// a function of one's own has nothing in front of it.
//
// One case cannot be told apart: a method called "func1" is spelled exactly like
// a literal inside a method called anything else. The cost of being wrong is one
// line of signature shown or not shown.
func (n *Node) Anonymous() bool {
	// What the name says within its own package: "build.func1" for a literal
	// declared inside build, "migrate" for a function of its own.
	local := strings.TrimPrefix(n.Name, render.Package(n.Name)+".")

	for {
		dot := strings.LastIndexByte(local, '.')
		if dot < 0 {
			return false
		}

		part := local[dot+1:]
		if counter, ok := strings.CutPrefix(part, "func"); ok && n.digits(counter) {
			return true
		}
		// A nested literal adds a bare counter of its own. Walk back past those to
		// reach the marker.
		if !n.digits(part) {
			return false
		}
		local = local[:dot]
	}
}

func (n *Node) digits(s string) bool {
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
// The qualified type cannot answer that. A generic names its type arguments in
// full, so the line that would say where a service lives is unreadable. This
// answers it and nothing else.
//
// A function's type is only a signature, and a service can be of a builtin type.
// Either way, the factory that made it has a package when the type does not.
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
// What the paths look like depends on how the binary was built. A release built
// with -trimpath reports module-relative paths that no editor can open, so a
// location is worth showing but not worth promising to resolve.
type Location struct {
	File string `json:"file"`
	Line int    `json:"line"`
	Func string `json:"func,omitzero"` // The function that registered it, where that is known.
}

// IsZero reports whether nothing was recorded. An empty Location is left out of
// the encoded graph.
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

	EdgeCount int `json:"edgeCount"`

	// Diagnostics are what is wrong with this argument: what the compiler
	// objected to, or what extraction could not resolve when no pass had objected
	// yet.
	Diagnostics []Diagnostic `json:"diagnostics,omitzero"`
}

// Faulty reports that this argument will not resolve as it stands.
func (p *Param) Faulty() bool { return hasError(p.Diagnostics) }

// TypeShort is Type without the package path, for labels.
func (p *Param) TypeShort() string { return render.Short(p.Type) }

// Position is what to call this argument in a sentence. It names the method the
// argument belongs to when it belongs to one, because "argument 2" alone means
// two different slots on a service with a method call.
//
// It is on the model so that every format, and every diagnostic, words an
// argument the same way.
func (p *Param) Position() string {
	if p.Method != "" {
		return fmt.Sprintf("argument %d of %s", p.Index, p.Method)
	}
	return fmt.Sprintf("argument %d", p.Index)
}

// Unwired reports that nothing filled this argument and something had to. A
// variadic slot is exempt: no arguments is a valid call, so an empty one is an
// optional dependency nothing provides rather than a gap in the wiring.
//
// It is on the model so that every format draws the same arguments as faulty,
// and so that they are the ones Graph.WiringDiagnostics reports.
func (p *Param) Unwired() bool {
	return p.Origin == ArgOriginNone && !p.Variadic
}

// Literal is a constant passed to a factory. Values are omitted unless asked
// for: literals routinely carry credentials.
type Literal struct {
	Type      string `json:"type"`
	Value     string `json:"value,omitzero"`
	Truncated bool   `json:"truncated,omitzero"`
	Redacted  bool   `json:"redacted,omitzero"`
}

// String renders one constant for display. The type is left out: every encoder
// already shows it on the argument row this sits beside.
//
// It is on the model so that every format renders a constant the same way.
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
	// Candidate reports that this is not wiring: it is one of the dependencies
	// that could have satisfied the argument, which is why the argument is
	// faulty. The container chose none of them.
	Candidate bool `json:"candidate,omitzero"`
}

// PassCredit names the compiler pass responsible for this edge, and is empty when
// none is. godi's own automation is not named: it is the default the reader
// already knows.
//
// A pass can be responsible in two ways: by wiring the argument, or by creating
// the binding the argument resolved through. The answer reads the same either
// way. Only when two different passes each had a hand in one edge does it say
// which did what.
//
// It is on the model so that every format gives the same answer.
// DecidedBy is who chose this particular dependency: whoever created the
// binding it resolved through, and otherwise whoever wired the argument.
//
// godi creating a binding for you and godi autowiring an argument are the same
// answer to "who decided": godi did.
//
// The colour of an edge and the viewer's filters both ask this, so it is answered
// here.
func (e *Edge) DecidedBy() ArgOrigin {
	hop, ok := e.Binding()
	if !ok {
		return e.Origin
	}

	switch hop.Origin {
	case BindOriginManual:
		return ArgOriginManual
	case BindOriginAutobinding:
		return ArgOriginAutowiring
	case BindOriginCompilerPass:
		return ArgOriginCompilerPass
	}
	return ArgOriginNone
}

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

// EdgeFaulty reports that this edge is part of wiring that will not work: the
// argument it feeds carries an error, or it closes a cycle.
//
// It is derived from the argument rather than stored on the edge, so what is
// drawn as wrong and what is reported as wrong come from one record. Every edge
// of a faulty argument counts, including one that on its own found a real
// service: an argument that cannot choose between two of them resolves to
// neither, and the reader has both to look at.
//
// It is on the model so that every format draws the same edges as faulty.
func (g *Graph) EdgeFaulty(e *Edge) bool {
	if e.Cycle {
		return true
	}
	p, ok := g.Param(e.Param)
	return ok && p.Faulty()
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

// Snapshot says when a graph was taken from a container that was still being
// built: before compilation started, or between two compiler passes.
//
// Wiring that a later pass would add is not there yet. A snapshot describes the
// work so far, not the container the application runs.
type Snapshot struct {
	// Stage is the compiler stage in progress. Empty before compilation starts.
	Stage string `json:"stage,omitzero"`
	// Pass is the compiler pass in progress. Empty outside a pass.
	Pass string `json:"pass,omitzero"`
	// Failed names the pass that returned an error, when compilation stopped
	// there. The graph is then as far as the container ever got, which is where
	// whatever went wrong is still visible.
	Failed string `json:"failed,omitzero"`
	// Done names the passes that had already finished, in the order they ran. It
	// says how much of the wiring is here.
	Done []string `json:"done,omitzero"`
	// Autowired says godi's autowiring has run. After it, an argument still
	// unwired is one nothing is going to wire. Before it, that is only work not
	// done yet.
	Autowired bool `json:"autowired,omitzero"`
}

// Label says in a few words when the graph was taken.
func (s *Snapshot) Label() string {
	switch {
	case s == nil:
		return ""
	case s.Failed != "":
		return "taken where the " + s.Failed + " pass failed"
	case s.Pass != "":
		return "taken during the " + s.Pass + " pass"
	case s.Stage != "":
		return "taken before the " + s.Stage + " stage"
	default:
		return "taken before the container was compiled"
	}
}

// Partial reports that the graph was taken while the container was still being
// built, and so describes wiring that is not finished.
func (g *Graph) Partial() bool { return g != nil && g.Snapshot != nil }

// Severity says how much a Diagnostic matters.
type Severity string

const (
	SeverityInfo    Severity = "info"    // Worth knowing; nothing is wrong.
	SeverityWarning Severity = "warning" // Something will not work as read.
	SeverityError   Severity = "error"   // Something is broken.
)

// Diagnostic reports something worth saying about a container: an argument that
// resolves to nothing, a service whose factory failed, a scope no definition
// owns, a file written by a version that knew a different schema.
//
// It says nothing about where it belongs, because it is stored on what it is
// about — a Param, a Node, a Scope, or the Graph itself when nothing narrower
// fits. Ask for it with AllDiagnostics, which pairs each with the element it came
// from.
type Diagnostic struct {
	Severity Severity `json:"severity"`
	Message  string   `json:"message"`
	// Pass names the compiler pass that reported it, where one did. Extraction
	// and the reader of a file report their own findings and name no pass.
	Pass string `json:"pass,omitzero"`
	// Location is where in the source this is about, for a diagnostic whose place
	// in the graph cannot say. A definition that would not parse is not a node, so
	// the file and line are all a reader has to go on. Paths are relative to the
	// graph's SourceRoot, like any other location.
	Location Location `json:"location,omitzero"`
}

func hasError(diags []Diagnostic) bool {
	for _, d := range diags {
		if d.Severity == SeverityError {
			return true
		}
	}
	return false
}

// LocatedDiagnostic is a diagnostic and what it is about. The ids are read off
// the element it was stored on, so a reader that wants to point at it does not
// have to walk the graph again.
type LocatedDiagnostic struct {
	Diagnostic
	Scope ScopeID
	Node  NodeID
	Param ParamID
}

// Where names what a diagnostic is about, for a reader looking at a list of them
// rather than at the thing itself. It is empty for one about the container.
//
// It is on the model so that every format names a place the same way.
func (d LocatedDiagnostic) Where(g *Graph) string {
	if d.Node == "" {
		if d.Scope == "" {
			return ""
		}
		if s, ok := g.Scope(d.Scope); ok {
			return s.Label()
		}
		return string(d.Scope)
	}

	where := string(d.Node)
	if node, ok := g.Node(d.Node); ok {
		where = node.Title()
	}
	if p, ok := g.Param(d.Param); ok {
		where += " " + p.Position()
	}
	return where
}

// AllDiagnostics is everything worth reporting about a graph, in a stable order:
// the container, then each scope, then each node followed by its arguments.
//
// It is a walk rather than a list of its own. Every diagnostic is stored once, on
// the element it is about, so what a reader is told and what is drawn as broken
// come from the same record and cannot disagree.
func (g *Graph) AllDiagnostics() []LocatedDiagnostic {
	var out []LocatedDiagnostic

	for _, d := range g.Diagnostics {
		out = append(out, LocatedDiagnostic{Diagnostic: d})
	}
	for _, s := range g.Scopes {
		for _, d := range s.Diagnostics {
			out = append(out, LocatedDiagnostic{Diagnostic: d, Scope: s.ID})
		}
	}
	for _, n := range g.Nodes {
		for _, d := range n.Diagnostics {
			out = append(out, LocatedDiagnostic{Diagnostic: d, Scope: n.Scope, Node: n.ID})
		}
		for _, p := range n.Params {
			for _, d := range p.Diagnostics {
				out = append(out, LocatedDiagnostic{Diagnostic: d, Scope: n.Scope, Node: n.ID, Param: p.ID})
			}
		}
	}

	return out
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
	g.indexOnce.Do(func() {
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
	})
}
