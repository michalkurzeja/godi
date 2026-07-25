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
	out       *graph.Graph

	scopeIDs map[*Scope]graph.ScopeID
	svcIDs   map[*ServiceDefinition]graph.NodeID
	funIDs   map[*FunctionDefinition]graph.NodeID
	byUUID   map[ID]graph.NodeID // Every definition, for resolving edge targets.
	minted   map[string]int      // Node ID collision counters.
}

func newExtractor(c *Container, cfg graph.Config) *extractor {
	return &extractor{
		container: c,
		cfg:       cfg,
		out:       &graph.Graph{Schema: graph.Schema},
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

	x.assignScope(x.container.root, graph.ScopeID(RootScope), "", 0, owners)

	// Scopes created directly through Scope.NewChild belong to no definition, so
	// the walk above never reaches them. Attach them to their parent by raw name.
	for scope := range x.container.scopesSeq() {
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
	for _, def := range x.container.serviceDefsSeq() {
		if cs := def.ChildScope(); cs != nil {
			owners[cs] = def
		}
	}
	for _, def := range x.container.functionDefsSeq() {
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
	for scope, def := range x.container.serviceDefsSeq() {
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
			Instantiated: instantiated(scope, def.ID()),
			Registered:   registered(def.source),
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

	for scope, def := range x.container.functionDefsSeq() {
		node := &graph.Node{
			ID:         x.funIDs[def],
			Kind:       graph.NodeFunction,
			UUID:       def.ID().String(),
			Scope:      x.scopeIDs[scope],
			Type:       util.Signature(def.Type()),
			Name:       def.Func().Name(),
			Signature:  util.Signature(def.Func().value().Type()),
			Labels:     labelStrings(def.Labels()),
			Lazy:       def.IsLazy(),
			Autowired:  def.IsAutowired(),
			Registered: registered(def.source),
			Defined:    defined(def.Func().value()),
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

// registered turns a captured stack into the place the definition was written.
func registered(s source) graph.Location {
	file, line, fn := s.Location()
	return graph.Location{File: file, Line: line, Func: fn}
}

// implementation is whatever actually provides a service: the factory that
// builds it, or - when the service was registered as a value - the value
// itself. A function is a value like any other, and one registered that way is
// the whole of the service, so it is what deserves to be named and pointed at.
//
// A value that is not a function has none of this to give. There is no name for
// a struct someone handed over and nowhere to point at for it, and the factory
// holding it is godi's own.
func implementation(def *ServiceDefinition) (name, signature string, at graph.Location) {
	impl := def.Factory().value()
	if val, ok := def.Val(); ok {
		if !val.IsValid() || val.Kind() != reflect.Func {
			return "", "", graph.Location{}
		}
		impl = val
	}
	return funcName(impl), funcSignature(impl), defined(impl)
}

// funcSignature is what a function takes and returns, as Go would write it.
//
// Not the value's own type, which for a service registered as a value is
// usually a named one - and a name is exactly what a signature is not. Rebuilt
// unnamed, which leaves an ordinary factory's type as it already was.
func funcSignature(fn reflect.Value) string {
	typ := fn.Type()

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

// funcName is what to call a function. A method value is a wrapper the compiler
// writes, and it names it after the method with a suffix of its own; the method
// is what the reader means.
func funcName(fn reflect.Value) string {
	return strings.TrimSuffix(util.FuncName(fn), "-fm")
}

// defined is where a function is declared.
//
// A factory godi synthesised has no source of the user's to point at - SvcVal
// wraps a value in a closure of godi's own - so those are left empty rather
// than pointing the reader into godi. So is a method value, whose wrapper the
// compiler writes and marks as generated.
func defined(fn reflect.Value) graph.Location {
	file, line := util.FuncLocation(fn)
	if file == "" || file == "<autogenerated>" || isWiring(util.FuncName(fn)) {
		return graph.Location{}
	}
	return graph.Location{File: file, Line: line}
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
		// rejects an unfilled slot, so a built container never has one.
		p.Origin, p.Arg = graph.ArgOriginNone, graph.ArgKindNone
		p.Note = "not wired yet"
		return
	}

	p.Origin = argOriginOf(slot.origin)
	p.OriginPass = slot.originPass

	arg := slot.Arg() // Fabricates a fresh compound for appended slots: call it once.
	p.Arg = argKindOf(arg)
	x.resolve(scope, arg, p, nil, nil)
}

// resolve turns one argument into edges, mirroring the resolution the container
// performs at runtime and recording which mechanism matched.
//
// seen guards against binding cycles; it is a slice because chains are short and
// it must not leak between sibling sub-arguments of a compound.
func (x *extractor) resolve(scope *Scope, arg Arg, p *graph.Param, hops []graph.BindingHop, seen []reflect.Type) {
	switch a := arg.(type) {
	case *literalArg:
		x.literal(p, a)

	case *refArg:
		x.edge(p, x.byUUID[a.def.ID()], graph.ResolutionRef, hops)

	case *typeArg:
		// typeArg.typ is the element type when the arg collects a slice, and
		// that is what the resolver looks bindings and services up by.
		x.byType(scope, a.typ, p, hops, seen, graph.ResolutionByType)

	case *labelArg:
		p.Label = string(a.label)
		for _, id := range scope.GetServicesIDsByLabelInChain(a.label) {
			x.edge(p, x.byUUID[id], graph.ResolutionByLabel, hops)
		}
		if p.EdgeCount == 0 {
			x.unresolved(p, fmt.Sprintf("no services labelled %q", a.label))
		}

	case *flexibleSliceArg:
		x.flexibleSlice(scope, a, p, hops, seen)

	case *compoundArg:
		for _, sub := range a.args {
			x.resolve(scope, sub, p, hops, seen)
		}

	default:
		x.unresolved(p, fmt.Sprintf("unsupported argument type %T", arg))
	}
}

// byType resolves a type match, following an interface binding if one covers it.
func (x *extractor) byType(scope *Scope, typ reflect.Type, p *graph.Param, hops []graph.BindingHop, seen []reflect.Type, res graph.Resolution) {
	if bindScope, binding, ok := bindingInChain(scope, typ); ok {
		x.throughBinding(scope, typ, bindScope, binding, p, hops, seen)
		return
	}

	ids := scope.GetServicesIDsByTypeInChain(typ)
	for _, id := range ids {
		x.edge(p, x.byUUID[id], res, hops)
	}
	if len(ids) == 0 {
		x.unresolved(p, fmt.Sprintf("no services of type %s", util.Signature(typ)))
	}
}

// flexibleSlice mirrors flexibleSliceArgResolver: the slice type wins over the
// element type, and each is checked against the bindings first.
func (x *extractor) flexibleSlice(scope *Scope, a *flexibleSliceArg, p *graph.Param, hops []graph.BindingHop, seen []reflect.Type) {
	sliceTyp := a.Type()
	if bindScope, binding, ok := bindingInChain(scope, sliceTyp); ok {
		x.throughBinding(scope, sliceTyp, bindScope, binding, p, hops, seen)
		return
	}
	if ids := scope.GetServicesIDsByTypeInChain(sliceTyp); len(ids) > 0 {
		for _, id := range ids {
			x.edge(p, x.byUUID[id], graph.ResolutionBySliceType, hops)
		}
		return
	}

	elemTyp := a.elemType
	if bindScope, binding, ok := bindingInChain(scope, elemTyp); ok {
		x.throughBinding(scope, elemTyp, bindScope, binding, p, hops, seen)
		return
	}
	ids := scope.GetServicesIDsByTypeInChain(elemTyp)
	for _, id := range ids {
		x.edge(p, x.byUUID[id], graph.ResolutionByElemType, hops)
	}
	if len(ids) == 0 && !a.allowEmpty {
		x.unresolved(p, fmt.Sprintf("no services of type %s", util.Signature(sliceTyp)))
	}
}

func (x *extractor) throughBinding(scope *Scope, typ reflect.Type, bindScope *Scope, binding *InterfaceBinding, p *graph.Param, hops []graph.BindingHop, seen []reflect.Type) {
	if slices.Contains(seen, typ) {
		x.unresolved(p, fmt.Sprintf("interface binding for %s is circular", util.Signature(typ)))
		return
	}

	hop := graph.BindingHop{
		Interface:  util.Signature(binding.Interface()),
		Scope:      x.scopeIDs[bindScope],
		Origin:     bindOriginOf(binding.origin),
		OriginPass: binding.originPass,
	}
	x.resolve(scope, binding.BoundTo(), p, append(hops[:len(hops):len(hops)], hop), append(seen, typ))
}

func (x *extractor) literal(p *graph.Param, a *literalArg) {
	if x.cfg.NoLiterals {
		return
	}

	typ := reflect.TypeOf(a.v)
	lit := graph.Literal{Type: util.Signature(typ)}

	if x.cfg.LiteralValues {
		if x.cfg.Redactor != nil {
			if replacement, redact := x.cfg.Redactor(typ, a.v); redact {
				lit.Value, lit.Redacted = replacement, true
				p.Literals = append(p.Literals, lit)
				return
			}
		}
		if hasReadableValue(typ) {
			lit.Value, lit.Truncated = render(a.v, x.cfg.LiteralMax)
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
	for scope := range x.container.scopesSeq() {
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
}

// bindingInChain finds the binding covering typ, and the scope that declared it.
// GetBoundArgInChain alone would not say which scope it came from.
func bindingInChain(scope *Scope, typ reflect.Type) (*Scope, *InterfaceBinding, bool) {
	for s := range scope.Chain() {
		if binding, ok := s.GetBinding(typ); ok {
			return s, binding, true
		}
	}
	return nil, nil, false
}

func instantiated(scope *Scope, id ID) bool {
	_, ok := scope.instances[id]
	return ok
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

func argOriginOf(origin argOrigin) graph.ArgOrigin {
	switch origin {
	case argOriginManual:
		return graph.ArgOriginManual
	case argOriginAutowiring:
		return graph.ArgOriginAutowiring
	case argOriginCompilerPass:
		return graph.ArgOriginCompilerPass
	case argOriginNone:
		return graph.ArgOriginNone
	}
	return graph.ArgOriginNone
}

func bindOriginOf(origin bindOrigin) graph.BindOrigin {
	switch origin {
	case bindOriginManual:
		return graph.BindOriginManual
	case bindOriginAutobinding:
		return graph.BindOriginAutobinding
	case bindOriginCompilerPass:
		return graph.BindOriginCompilerPass
	}
	return graph.BindOriginManual
}

func argKindOf(arg Arg) graph.ArgKind {
	switch arg.(type) {
	case *literalArg:
		return graph.ArgKindLiteral
	case *refArg:
		return graph.ArgKindRef
	case *typeArg:
		return graph.ArgKindType
	case *labelArg:
		return graph.ArgKindLabel
	case *flexibleSliceArg:
		return graph.ArgKindFlexibleSlice
	case *compoundArg:
		return graph.ArgKindCompound
	}
	return graph.ArgKindUnknown
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
