package di

import (
	"errors"
	"fmt"
	"path"
	"reflect"
	"slices"
	"strings"

	dbgraph "github.com/dominikbraun/graph"

	"github.com/michalkurzeja/godi/v2/graph"
	"github.com/michalkurzeja/godi/v2/internal/util"
)

type extractor struct {
	container *Container
	cfg       graph.Config
	// snapshot says how far compilation had got, and is nil for a container
	// that finished building. It decides what counts as missing: before
	// autowiring runs, an unwired argument is work outstanding rather than a
	// fault.
	snapshot *graph.Snapshot
	out      *graph.Graph

	scopeIDs map[*Scope]graph.ScopeID
	svcIDs   map[*ServiceDefinition]graph.NodeID
	funIDs   map[*FunctionDefinition]graph.NodeID
	byUUID   map[ID]graph.NodeID // Every definition, for resolving edge targets.
	minted   map[string]int      // Node ID collision counters.
}

func newExtractor(c *Container, cfg graph.Config, snapshot *graph.Snapshot) *extractor {
	return &extractor{
		container: c,
		cfg:       cfg,
		snapshot:  snapshot,
		out:       &graph.Graph{Schema: graph.Schema, Snapshot: snapshot},
		scopeIDs:  make(map[*Scope]graph.ScopeID),
		svcIDs:    make(map[*ServiceDefinition]graph.NodeID),
		funIDs:    make(map[*FunctionDefinition]graph.NodeID),
		byUUID:    make(map[ID]graph.NodeID),
		minted:    make(map[string]int),
	}
}

func (x *extractor) extract() *graph.Graph {
	x.assignIDs()
	x.buildNodes()
	x.markCollected()
	x.buildBindings()
	x.markCycles()
	x.markIncomplete()
	x.countDegrees()
	x.sortAll()
	x.trimSourceRoot()
	return x.out
}

// assignIDs walks the scope tree top down, naming every scope and definition.
//
// The container only keeps parent pointers, and a child scope's own name is the
// uuid of the definition that declared it, so the tree is rebuilt here and child
// scopes are named after their owner's node ID instead.
func (x *extractor) assignIDs() {
	owners := x.childScopeOwners()

	x.assignScope(x.container.Root(), graph.ScopeID(RootScope), "", 0, owners)

	// Scopes created directly through Scope.NewChild belong to no definition, so
	// the walk above never reaches them. Attach them to their parent by raw name.
	for scope := range x.container.Scopes() {
		if _, ok := x.scopeIDs[scope]; ok {
			continue
		}
		parent := graph.ScopeID(RootScope)
		depth := 1
		if p := scope.Parent(); p != nil {
			if id, ok := x.scopeIDs[p]; ok {
				parent = id
				depth = x.depthOf(id) + 1
			}
		}
		x.assignScope(scope, parent+graph.ScopeID("/scope:"+scope.Name()), parent, depth, owners)
		x.diag("warning", graph.Diagnostic{
			Scope:   x.scopeIDs[scope],
			Message: fmt.Sprintf("scope %q belongs to no definition", scope.Name()),
		})
	}
}

// childScopeOwners maps every child scope to the definition that declared it.
func (x *extractor) childScopeOwners() map[*Scope]any {
	owners := make(map[*Scope]any)
	for _, def := range x.container.ServiceDefinitionsSeq() {
		if cs := def.ChildScope(); cs != nil {
			owners[cs] = def
		}
	}
	for _, def := range x.container.FunctionDefinitionsSeq() {
		if cs := def.ChildScope(); cs != nil {
			owners[cs] = def
		}
	}
	return owners
}

func (x *extractor) assignScope(scope *Scope, id, parent graph.ScopeID, depth int, owners map[*Scope]any) {
	if scope == nil {
		return
	}
	if _, done := x.scopeIDs[scope]; done {
		return
	}
	x.scopeIDs[scope] = id

	entry := &graph.Scope{ID: id, Parent: parent, Depth: depth, Name: scope.Name()}
	if owner, ok := owners[scope]; ok {
		entry.Owner = x.ownerID(owner)
		entry.OwnerName = x.ownerName(owner)
	}
	x.out.Scopes = append(x.out.Scopes, entry)

	// Name the definitions before recursing: a child scope is named after the
	// node that declared it.
	for def := range scope.ServiceDefinitionsSeq() {
		nodeID := x.mint(id, "svc", util.Signature(def.Type()), def.Labels())
		x.svcIDs[def] = nodeID
		x.byUUID[def.ID()] = nodeID
	}
	for def := range scope.FunctionDefinitionsSeq() {
		nodeID := x.mint(id, "fn", def.Func().Name(), def.Labels())
		x.funIDs[def] = nodeID
		x.byUUID[def.ID()] = nodeID
	}

	for def := range scope.ServiceDefinitionsSeq() {
		if cs := def.ChildScope(); cs != nil && owners[cs] == def {
			x.assignScope(cs, graph.ScopeID(x.svcIDs[def]), id, depth+1, owners)
		}
	}
	for def := range scope.FunctionDefinitionsSeq() {
		if cs := def.ChildScope(); cs != nil && owners[cs] == def {
			x.assignScope(cs, graph.ScopeID(x.funIDs[def]), id, depth+1, owners)
		}
	}
}

func (x *extractor) ownerID(owner any) graph.NodeID {
	switch def := owner.(type) {
	case *ServiceDefinition:
		return x.svcIDs[def]
	case *FunctionDefinition:
		return x.funIDs[def]
	}
	return ""
}

// ownerName is what to call the definition that declared a scope: the same
// thing a node of it is called, a service by the type it provides and a
// function by its name. Recorded on the scope because a node ID is built to be
// unique rather than to be read, and because the owner may itself be filtered
// out of a graph that still has to name the scope it left behind.
func (x *extractor) ownerName(owner any) string {
	switch def := owner.(type) {
	case *ServiceDefinition:
		return util.Signature(def.Type())
	case *FunctionDefinition:
		return def.Func().Name()
	}
	return ""
}

func (x *extractor) depthOf(id graph.ScopeID) int {
	for _, s := range x.out.Scopes {
		if s.ID == id {
			return s.Depth
		}
	}
	return 0
}

// mint builds a readable, build-stable node ID. Definition uuids are regenerated
// on every build, so they would make the output impossible to diff or compare.
func (x *extractor) mint(scope graph.ScopeID, kind, name string, labels []Label) graph.NodeID {
	var sb strings.Builder
	sb.WriteString(string(scope))
	sb.WriteString("/")
	sb.WriteString(kind)
	sb.WriteString(":")
	sb.WriteString(name)

	if len(labels) > 0 {
		strs := make([]string, len(labels))
		for i, l := range labels {
			strs[i] = string(l)
		}
		slices.Sort(strs)
		sb.WriteString("[")
		sb.WriteString(strings.Join(strs, ","))
		sb.WriteString("]")
	}

	base := sb.String()
	n := x.minted[base]
	x.minted[base] = n + 1
	if n > 0 {
		return graph.NodeID(fmt.Sprintf("%s#%d", base, n))
	}
	return graph.NodeID(base)
}

func (x *extractor) buildNodes() {
	for scope, def := range x.container.ServiceDefinitionsSeq() {
		node := &graph.Node{
			ID:           x.svcIDs[def],
			Kind:         graph.NodeService,
			UUID:         def.ID().String(),
			Scope:        x.scopeIDs[scope],
			Type:         util.Signature(def.Type()),
			Labels:       labelStrings(def.Labels()),
			Lazy:         def.IsLazy(),
			Shared:       def.IsShared(),
			Autowired:    def.IsAutowired(),
			Instantiated: scope.Instantiated(def.ID()),
			Registered:   registered(def.RegisteredAt()),
		}
		node.Name, node.Signature, node.Defined = implementation(def)
		_, node.FromValue = def.Val()
		if cs := def.ChildScope(); cs != nil {
			node.ChildScope = x.scopeIDs[cs]
		}

		resolveScope := def.EffectiveScope()
		for _, slot := range def.Factory().Args().Slots() {
			x.param(node, resolveScope, slot, graph.InjectFactoryArg, "")
		}
		for _, method := range def.MethodCalls() {
			x.methodParams(node, resolveScope, def, method)
		}

		x.out.Nodes = append(x.out.Nodes, node)
	}

	for scope, def := range x.container.FunctionDefinitionsSeq() {
		node := &graph.Node{
			ID:         x.funIDs[def],
			Kind:       graph.NodeFunction,
			UUID:       def.ID().String(),
			Scope:      x.scopeIDs[scope],
			Type:       util.Signature(def.Type()),
			Name:       def.Func().Name(),
			Signature:  util.Signature(def.Func().Type()),
			Labels:     labelStrings(def.Labels()),
			Lazy:       def.IsLazy(),
			Autowired:  def.IsAutowired(),
			Registered: registered(def.RegisteredAt()),
			Defined:    registered(def.DefinedAt()),
		}
		if cs := def.ChildScope(); cs != nil {
			node.ChildScope = x.scopeIDs[cs]
		}

		resolveScope := def.EffectiveScope()
		for _, slot := range def.Func().Args().Slots() {
			x.param(node, resolveScope, slot, graph.InjectFunctionArg, "")
		}

		x.out.Nodes = append(x.out.Nodes, node)
	}
}

// registered turns a definition's registration site into the model's own.
func registered(loc Location) graph.Location {
	return graph.Location{File: loc.File, Line: loc.Line, Func: loc.Func}
}

// implementation is how the model names whatever provides a service. The engine
// picks it - the factory, or the value when the service was registered as one -
// and this is the display half: a name a reader would recognise, and a
// signature rather than a type.
func implementation(def *ServiceDefinition) (name, signature string, at graph.Location) {
	implName, typ, loc := def.Implementation()
	if typ == nil {
		return "", "", graph.Location{}
	}
	return trimMethodWrapper(implName), funcSignature(typ), registered(loc)
}

// funcSignature is what a function takes and returns, as Go would write it.
//
// Not the value's own type, which for a service registered as a value is
// usually a named one - and a name is exactly what a signature is not. Rebuilt
// unnamed, which leaves an ordinary factory's type as it already was.
func funcSignature(typ reflect.Type) string {
	in := make([]reflect.Type, typ.NumIn())
	for i := range in {
		in[i] = typ.In(i)
	}
	out := make([]reflect.Type, typ.NumOut())
	for i := range out {
		out[i] = typ.Out(i)
	}
	return util.Signature(reflect.FuncOf(in, out, typ.IsVariadic()))
}

// trimMethodWrapper is what to call a function. A method value is a wrapper the
// compiler writes, and it names it after the method with a suffix of its own;
// the method is what the reader means.
func trimMethodWrapper(name string) string {
	return strings.TrimSuffix(name, "-fm")
}

// trimSourceRoot takes the directory every path shares out of the paths and
// records it once. Absolute build-machine paths make the output long, unstable
// between machines, and noisy to diff; the root puts them back together again.
func (x *extractor) trimSourceRoot() {
	var paths []string
	for _, node := range x.out.Nodes {
		for _, loc := range []graph.Location{node.Registered, node.Defined} {
			if loc.File != "" {
				paths = append(paths, loc.File)
			}
		}
	}

	root := commonDir(paths)
	if root == "" {
		return
	}

	x.out.SourceRoot = root
	for _, node := range x.out.Nodes {
		node.Registered.File = strings.TrimPrefix(node.Registered.File, root+"/")
		node.Defined.File = strings.TrimPrefix(node.Defined.File, root+"/")
	}
}

// commonDir is the longest directory prefix shared by every path, or empty when
// that is nothing worth trimming. A container wired partly from the standard
// library shares only the filesystem root, which is no help to anyone.
func commonDir(paths []string) string {
	if len(paths) == 0 {
		return ""
	}

	shared := strings.Split(path.Dir(paths[0]), "/")
	for _, p := range paths[1:] {
		parts := strings.Split(path.Dir(p), "/")
		shared = shared[:min(len(shared), len(parts))]
		for i := range shared {
			if shared[i] != parts[i] {
				shared = shared[:i]
				break
			}
		}
	}

	// Nothing, the filesystem root, and "the current directory" are all prefixes
	// that would be recorded without shortening a single path.
	root := strings.Join(shared, "/")
	if root == "" || root == "/" || root == "." {
		return ""
	}
	return root
}

// methodParams emits the arguments of a method call. Slot 0 holds the receiver,
// which godi injects itself; it is only worth an edge if it somehow points
// somewhere other than the owning definition.
func (x *extractor) methodParams(node *graph.Node, scope *Scope, def *ServiceDefinition, method *Method) {
	slots := method.Args().Slots()
	if len(slots) == 0 {
		return
	}

	// The method's own name is enough: qualifying it would just repeat the type
	// already on the node.
	name := shortMethodName(method.Name())

	if receiver := slots[0]; receiver.IsFilled() {
		if ref, ok := receiver.Arg().(*refArg); !ok || ref.def != def {
			x.param(node, scope, receiver, graph.InjectMethodReceiver, name)
		}
	}

	for _, slot := range slots[1:] {
		x.param(node, scope, slot, graph.InjectMethodArg, name)
	}
}

func shortMethodName(name string) string {
	if i := strings.LastIndexByte(name, '.'); i >= 0 {
		return name[i+1:]
	}
	return name
}

func (x *extractor) param(node *graph.Node, scope *Scope, slot *Slot, kind graph.InjectionKind, method string) {
	p := &graph.Param{
		ID:       paramID(node.ID, kind, method, int(slot.Index())),
		Node:     node.ID,
		Kind:     kind,
		Method:   method,
		Index:    int(slot.Index()),
		Type:     util.Signature(slot.Type()),
		Slice:    slot.IsSlice(),
		Variadic: slot.IsVariadicSlice(),
	}
	if p.Slice {
		p.ElemType = util.Signature(slot.ElemType())
	}
	node.Params = append(node.Params, p)

	if !slot.IsFilled() {
		// Only reachable before autowiring runs: the argument validation pass
		// rejects an unfilled slot, so a built container never has one. The
		// origin is the whole story, and each encoder has its own words for it.
		p.Origin, p.Arg = graph.ArgOriginNone, graph.ArgKindNone
		return
	}

	p.Origin = argOriginOf(slot.origin)
	p.OriginPass = slot.originPass

	arg := slot.Arg() // Fabricates a fresh compound for appended slots: call it once.
	trace := TraceArg(scope, arg)
	p.Arg = argKindOf(trace.Kind)
	x.walk(p, trace)
}

// walk turns one argument's trace into edges. The resolution itself belongs to
// the argument - this only says what each part of it looks like in a graph.
func (x *extractor) walk(p *graph.Param, t ArgTrace) {
	switch t.Kind {
	case ArgKindLiteral:
		x.literal(p, t.Value)
	case ArgKindLabel:
		p.Label = string(t.Label)
	case ArgKindRef, ArgKindType, ArgKindFlexibleSlice, ArgKindCompound:
	}

	hops := x.hops(t.Bindings)
	for _, id := range t.Matches {
		x.edge(p, x.byUUID[id], resolutionOf(t.By), hops)
	}

	x.fault(p, t.Fault)

	for _, part := range t.Parts {
		x.walk(p, part)
	}
}

// fault reports what an argument failed to find, in the words the reader needs.
//
// An argument matching nothing by label is only a fault when the whole argument
// matched nothing: a label that adds nothing to a compound that resolved is
// worth no complaint.
func (x *extractor) fault(p *graph.Param, f ArgFault) {
	switch f.Kind {
	case ArgFaultNone:
	case ArgFaultNoServicesOfType:
		x.unresolved(p, fmt.Sprintf("no services of type %s", util.Signature(f.Type)))
	case ArgFaultNoServicesWithLabel:
		if p.EdgeCount == 0 {
			x.unresolved(p, fmt.Sprintf("no services labelled %q", f.Label))
		}
	case ArgFaultCircularBinding:
		x.unresolved(p, fmt.Sprintf("interface binding for %s is circular", util.Signature(f.Type)))
	}
}

// hops is the bindings an argument resolved through, as the model records them.
// Nil for an argument that went through none, so that a graph of a container
// without bindings carries no empty lists.
func (x *extractor) hops(hops []BindingHop) []graph.BindingHop {
	if len(hops) == 0 {
		return nil
	}
	out := make([]graph.BindingHop, len(hops))
	for i, hop := range hops {
		out[i] = graph.BindingHop{
			Interface:  util.Signature(hop.Interface),
			Scope:      x.scopeIDs[hop.Scope],
			Origin:     bindOriginOf(hop.Origin),
			OriginPass: hop.OriginPass,
		}
	}
	return out
}

func (x *extractor) literal(p *graph.Param, v any) {
	if x.cfg.NoLiterals {
		return
	}

	typ := reflect.TypeOf(v)
	lit := graph.Literal{Type: util.Signature(typ)}

	if x.cfg.LiteralValues {
		if x.cfg.Redactor != nil {
			if replacement, redact := x.cfg.Redactor(typ, v); redact {
				lit.Value, lit.Redacted = replacement, true
				p.Literals = append(p.Literals, lit)
				return
			}
		}
		if hasReadableValue(typ) {
			lit.Value, lit.Truncated = render(v, x.cfg.LiteralMax)
		}
	}

	p.Literals = append(p.Literals, lit)
}

// hasReadableValue reports whether formatting the value says anything the type
// does not already say.
//
// A func, a channel or a plain pointer formats as a machine address: it tells
// the reader nothing and differs on every run, which would make graphs of the
// same wiring fail to compare.
func hasReadableValue(typ reflect.Type) bool {
	if typ == nil {
		return true // A nil literal formats as <nil>, which is worth showing.
	}

	// A type that formats itself is worth asking, whatever its kind.
	if typ.Implements(stringerType) || typ.Implements(errorType) {
		return true
	}

	switch typ.Kind() {
	case reflect.Func, reflect.Chan, reflect.UnsafePointer:
		return false
	case reflect.Pointer:
		// fmt prints &{...} for a pointer to a composite, an address otherwise.
		switch typ.Elem().Kind() {
		case reflect.Struct, reflect.Array, reflect.Slice, reflect.Map:
			return true
		default:
			return false
		}
	default:
		return true
	}
}

var (
	stringerType = reflect.TypeFor[fmt.Stringer]()
	errorType    = reflect.TypeFor[error]()
)

func (x *extractor) edge(p *graph.Param, to graph.NodeID, res graph.Resolution, hops []graph.BindingHop) {
	if to == "" {
		x.unresolved(p, "dependency is not registered in this container")
		return
	}

	x.out.Edges = append(x.out.Edges, &graph.Edge{
		ID:         graph.NewEdgeID(p.ID, p.EdgeCount),
		From:       p.Node,
		To:         to,
		Param:      p.ID,
		Kind:       p.Kind,
		Origin:     p.Origin,
		OriginPass: p.OriginPass,
		Resolution: res,
		Bindings:   hops,
		ParamType:  cmpOr(p.ElemType, p.Type),
		Ordinal:    p.EdgeCount,
	})
	p.EdgeCount++
}

func (x *extractor) unresolved(p *graph.Param, msg string) {
	p.Unresolved = true
	p.Note = msg
	x.diag("warning", graph.Diagnostic{Node: p.Node, Param: p.ID, Message: msg})
}

func (x *extractor) diag(severity string, d graph.Diagnostic) {
	d.Severity = severity
	x.out.Diagnostics = append(x.out.Diagnostics, &d)
}

// markCollected flags the edges that feed a slot taking more than one
// dependency, so that an encoder can draw a collection as such even when only
// one service happens to match today.
func (x *extractor) markCollected() {
	params := make(map[graph.ParamID]*graph.Param)
	for _, node := range x.out.Nodes {
		for _, p := range node.Params {
			params[p.ID] = p
		}
	}
	for _, edge := range x.out.Edges {
		if p, ok := params[edge.Param]; ok {
			edge.OfMany = p.Slice || p.EdgeCount > 1
		}
	}
}

func (x *extractor) buildBindings() {
	for scope := range x.container.Scopes() {
		scopeID := x.scopeIDs[scope]
		for _, binding := range scope.GetBindings() {
			entry := &graph.Binding{
				Scope:      scopeID,
				Interface:  util.Signature(binding.Interface()),
				Origin:     bindOriginOf(binding.origin),
				OriginPass: binding.originPass,
				BoundTo:    binding.BoundTo().String(),
			}
			for _, id := range ResolveArgIDs(scope, binding.BoundTo()) {
				if nodeID, ok := x.byUUID[id]; ok {
					entry.Targets = append(entry.Targets, nodeID)
				}
			}
			x.out.Bindings = append(x.out.Bindings, entry)
		}
	}

	// Count the edges that resolved through each binding, so that a binding
	// nothing ever used stands out.
	for _, edge := range x.out.Edges {
		for _, hop := range edge.Bindings {
			for _, entry := range x.out.Bindings {
				if entry.Scope == hop.Scope && entry.Interface == hop.Interface {
					entry.EdgeCount++
					break
				}
			}
		}
	}
}

// markCycles flags the edges that close a cycle. Cycle validation can be turned
// off, so the graph is not guaranteed acyclic.
func (x *extractor) markCycles() {
	g := dbgraph.New(func(id graph.NodeID) graph.NodeID { return id }, dbgraph.Directed(), dbgraph.PreventCycles())

	for _, node := range x.out.Nodes {
		_ = g.AddVertex(node.ID)
	}
	for _, edge := range x.out.Edges {
		err := g.AddEdge(edge.From, edge.To)
		if errors.Is(err, dbgraph.ErrEdgeCreatesCycle) {
			edge.Cycle = true
		}
	}
}

// markIncomplete flags the nodes something is wrong with, so a reader can find
// them without reading every argument of every service.
//
// Two things count: an argument naming a dependency the container does not
// have, and one nothing has wired once nothing is going to. Before autowiring
// runs, the second is simply work outstanding - every argument is unwired then,
// and marking every node would say nothing at all.
func (x *extractor) markIncomplete() {
	wired := x.snapshot == nil || x.snapshot.Autowired

	for _, node := range x.out.Nodes {
		for _, p := range node.Params {
			// An argument naming something that is not there was reported where
			// it failed to resolve. One nothing filled fails no lookup, so it is
			// reported here - and reported, rather than only marked, so that a
			// reader counting what is wrong is not told a number short of one.
			if wired && p.Origin == graph.ArgOriginNone {
				x.diag("warning", graph.Diagnostic{
					Node: node.ID, Param: p.ID, Message: notSet(p),
				})
			}
			if p.Unresolved || (wired && p.Origin == graph.ArgOriginNone) {
				node.Incomplete = true
			}
		}
	}
}

// notSet words an argument nobody filled the way the compiler does, naming the
// method when the argument belongs to one: "argument 2" alone means two
// different slots on a service with a method call.
func notSet(p *graph.Param) string {
	if p.Method != "" {
		return fmt.Sprintf("argument %d of %s is not set", p.Index, p.Method)
	}
	return fmt.Sprintf("argument %d is not set", p.Index)
}

func (x *extractor) countDegrees() {
	nodes := make(map[graph.NodeID]*graph.Node, len(x.out.Nodes))
	for _, node := range x.out.Nodes {
		nodes[node.ID] = node
	}
	for _, edge := range x.out.Edges {
		if from, ok := nodes[edge.From]; ok {
			from.OutDegree++
		}
		if to, ok := nodes[edge.To]; ok {
			to.InDegree++
		}
	}

	// Nothing injects it, so it is the top of a tree.
	for _, node := range x.out.Nodes {
		node.Root = node.InDegree == 0
	}
}

func (x *extractor) sortAll() {
	slices.SortFunc(x.out.Nodes, func(a, b *graph.Node) int {
		return strings.Compare(string(a.ID), string(b.ID))
	})
	slices.SortFunc(x.out.Edges, func(a, b *graph.Edge) int {
		if c := strings.Compare(string(a.From), string(b.From)); c != 0 {
			return c
		}
		if c := strings.Compare(string(a.Param), string(b.Param)); c != 0 {
			return c
		}
		return a.Ordinal - b.Ordinal
	})
	slices.SortFunc(x.out.Bindings, func(a, b *graph.Binding) int {
		if c := strings.Compare(string(a.Scope), string(b.Scope)); c != 0 {
			return c
		}
		return strings.Compare(a.Interface, b.Interface)
	})
	// By what they are about, so that everything wrong with one node is read
	// together, whatever order the two passes that found it ran in.
	slices.SortFunc(x.out.Diagnostics, func(a, b *graph.Diagnostic) int {
		if c := strings.Compare(string(a.Node), string(b.Node)); c != 0 {
			return c
		}
		if c := strings.Compare(string(a.Param), string(b.Param)); c != 0 {
			return c
		}
		return strings.Compare(a.Message, b.Message)
	})
}

func paramID(node graph.NodeID, kind graph.InjectionKind, method string, index int) graph.ParamID {
	switch kind {
	case graph.InjectFunctionArg:
		return graph.ParamID(fmt.Sprintf("%s#c:%d", node, index))
	case graph.InjectMethodArg, graph.InjectMethodReceiver:
		return graph.ParamID(fmt.Sprintf("%s#m:%s:%d", node, method, index))
	case graph.InjectFactoryArg:
		return graph.ParamID(fmt.Sprintf("%s#f:%d", node, index))
	}
	return graph.ParamID(fmt.Sprintf("%s#f:%d", node, index))
}

func argOriginOf(origin ArgOrigin) graph.ArgOrigin {
	switch origin {
	case ArgOriginManual:
		return graph.ArgOriginManual
	case ArgOriginAutowiring:
		return graph.ArgOriginAutowiring
	case ArgOriginCompilerPass:
		return graph.ArgOriginCompilerPass
	case ArgOriginNone:
		return graph.ArgOriginNone
	}
	return graph.ArgOriginNone
}

func bindOriginOf(origin BindOrigin) graph.BindOrigin {
	switch origin {
	case BindOriginManual:
		return graph.BindOriginManual
	case BindOriginAutobinding:
		return graph.BindOriginAutobinding
	case BindOriginCompilerPass:
		return graph.BindOriginCompilerPass
	}
	return graph.BindOriginManual
}

func argKindOf(kind ArgKind) graph.ArgKind {
	switch kind {
	case ArgKindLiteral:
		return graph.ArgKindLiteral
	case ArgKindRef:
		return graph.ArgKindRef
	case ArgKindType:
		return graph.ArgKindType
	case ArgKindLabel:
		return graph.ArgKindLabel
	case ArgKindFlexibleSlice:
		return graph.ArgKindFlexibleSlice
	case ArgKindCompound:
		return graph.ArgKindCompound
	}
	return graph.ArgKindUnknown
}

func resolutionOf(res Resolution) graph.Resolution {
	switch res {
	case ResolutionRef:
		return graph.ResolutionRef
	case ResolutionByType:
		return graph.ResolutionByType
	case ResolutionBySliceType:
		return graph.ResolutionBySliceType
	case ResolutionByElemType:
		return graph.ResolutionByElemType
	case ResolutionByLabel:
		return graph.ResolutionByLabel
	}
	return graph.ResolutionByType
}

func labelStrings(labels []Label) []string {
	if len(labels) == 0 {
		return nil
	}
	out := make([]string, len(labels))
	for i, l := range labels {
		out[i] = string(l)
	}
	slices.Sort(out)
	return out
}

// render formats a literal value for display. A value is arbitrary user data, so
// a broken Stringer must not take the whole graph down with it.
func render(v any, maxRunes int) (s string, truncated bool) {
	defer func() {
		if r := recover(); r != nil {
			s, truncated = "<panicked while formatting>", false
		}
	}()

	return util.Truncate(fmt.Sprintf("%v", v), maxRunes)
}

func cmpOr(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
