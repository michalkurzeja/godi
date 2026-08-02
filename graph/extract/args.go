package extract

import (
	"cmp"
	"fmt"
	"reflect"
	"strings"

	"github.com/michalkurzeja/godi/v2/di"
	"github.com/michalkurzeja/godi/v2/graph"
	"github.com/michalkurzeja/godi/v2/internal/util"
)

// methodParams emits the arguments of a method call. Slot 0 holds the receiver,
// which godi injects itself. It is only worth an edge if it points somewhere
// other than the owning definition.
func (x *extractor) methodParams(node *graph.Node, scope *di.Scope, def *di.ServiceDefinition, method *di.Method) {
	slots := method.Args().Slots()
	if len(slots) == 0 {
		return
	}

	// The method's own name is enough. Qualifying it would repeat the type
	// already on the node.
	name := x.shortMethodName(method.Name())

	if receiver := slots[0]; receiver.IsFilled() && !x.refersTo(scope, receiver, def) {
		x.param(node, scope, receiver, graph.InjectMethodReceiver, name)
	}

	for _, slot := range slots[1:] {
		x.param(node, scope, slot, graph.InjectMethodArg, name)
	}
}

// refersTo reports whether the slot holds the plain reference to the definition
// itself, which is what godi puts in a method's receiver slot. An edge from a
// service to itself says nothing. Anything else in that slot is worth showing.
func (x *extractor) refersTo(scope *di.Scope, slot *di.Slot, def *di.ServiceDefinition) bool {
	t := di.TraceArg(scope, slot.Arg())
	return t.Kind == di.ArgKindRef && len(t.Matches) == 1 && t.Matches[0] == def.ID()
}

func (x *extractor) shortMethodName(name string) string {
	if i := strings.LastIndexByte(name, '.'); i >= 0 {
		return name[i+1:]
	}
	return name
}

func (x *extractor) param(node *graph.Node, scope *di.Scope, slot *di.Slot, kind graph.InjectionKind, method string) {
	p := &graph.Param{
		ID:       x.paramID(node.ID, kind, method, int(slot.Index())),
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
		// Before autowiring runs, or a variadic slot nobody supplied: the argument
		// validation pass rejects every other unfilled slot. The origin is the whole
		// story, and each encoder has its own words for it.
		p.Origin, p.Arg = graph.ArgOriginNone, graph.ArgKindNone
		return
	}

	origin, pass := slot.Origin()
	p.Origin, p.OriginPass = x.argOriginOf(origin), pass

	arg := slot.Arg() // Fabricates a fresh compound for appended slots: call it once.
	trace := di.TraceArg(scope, arg)
	p.Arg = x.argKindOf(trace.Kind)
	x.walk(p, trace)
}

// walk turns one argument's trace into edges. The argument decided how it resolved.
// This only says what each part of that looks like in a graph.
func (x *extractor) walk(p *graph.Param, t di.ArgTrace) {
	switch t.Kind {
	case di.ArgKindLiteral:
		x.literal(p, t.Value)
	case di.ArgKindLabel:
		p.Label = string(t.Label)
	case di.ArgKindRef, di.ArgKindType, di.ArgKindFlexibleSlice, di.ArgKindCompound:
	}

	hops := x.hops(t.Bindings)
	for _, id := range t.Matches {
		x.edge(p, x.byUUID[id], x.resolutionOf(t.By), hops)
	}

	x.fault(p, t.Fault)

	for _, part := range t.Parts {
		x.walk(p, part)
	}
}

// fault reports what an argument failed to find, in the words the reader needs.
//
// A label that matched nothing is only a fault when the whole argument matched
// nothing. Inside a compound that did resolve, it is not a problem.
func (x *extractor) fault(p *graph.Param, f di.ArgFault) {
	switch f.Kind {
	case di.ArgFaultNone:
	case di.ArgFaultNoServicesOfType:
		x.unresolved(p, fmt.Sprintf("no services of type %s", util.Signature(f.Type)))
	case di.ArgFaultNoServicesWithLabel:
		if p.EdgeCount == 0 {
			x.unresolved(p, fmt.Sprintf("no services labelled %q", f.Label))
		}
	case di.ArgFaultCircularBinding:
		x.unresolved(p, fmt.Sprintf("interface binding for %s is circular", util.Signature(f.Type)))
	}
}

// hops is the bindings an argument resolved through, as the model records them. It
// is nil for an argument that went through none, so a graph of a container without
// bindings carries no empty lists.
func (x *extractor) hops(hops []di.BindingHop) []graph.BindingHop {
	if len(hops) == 0 {
		return nil
	}
	out := make([]graph.BindingHop, len(hops))
	for i, hop := range hops {
		out[i] = graph.BindingHop{
			Interface:  util.Signature(hop.Interface),
			Scope:      x.scopeIDs[hop.Scope],
			Origin:     x.bindOriginOf(hop.Origin),
			OriginPass: hop.OriginPass,
		}
	}
	return out
}

func (x *extractor) literal(p *graph.Param, v any) {
	if x.cfg.Literals == graph.LiteralNone {
		return
	}

	typ := reflect.TypeOf(v)
	lit := graph.Literal{Type: util.Signature(typ)}

	if x.cfg.Literals == graph.LiteralValues {
		if x.cfg.Redactor != nil {
			if replacement, redact := x.cfg.Redactor(typ, v); redact {
				lit.Value, lit.Redacted = replacement, true
				p.Literals = append(p.Literals, lit)
				return
			}
		}
		if x.hasReadableValue(typ) {
			lit.Value, lit.Truncated = x.render(v)
		}
	}

	p.Literals = append(p.Literals, lit)
}

// hasReadableValue reports whether formatting the value says anything the type
// does not already say.
//
// A func, a channel or a plain pointer formats as a machine address. That tells
// the reader nothing and differs on every run, so two graphs of the same wiring
// would not compare equal.
func (x *extractor) hasReadableValue(typ reflect.Type) bool {
	if typ == nil {
		return true // A nil literal formats as <nil>, which is worth showing.
	}

	// A type that formats itself is worth asking, whatever its kind is.
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
		ParamType:  cmp.Or(p.ElemType, p.Type),
		Ordinal:    p.EdgeCount,
	})
	p.EdgeCount++
}

// unresolved records what an argument failed to find. The note is the whole
// record. Wiring faults are read back off the parameters, so writing one down
// twice would be two records that can disagree.
func (x *extractor) unresolved(p *graph.Param, msg string) {
	p.Unresolved = true
	p.Note = msg
}

// note is something about this graph rather than about the container it describes.
// Nothing is wrong with the wiring, so nothing is marked.
func (x *extractor) note(severity graph.Severity, d graph.Diagnostic) {
	d.Severity = severity
	x.out.GraphDiagnostics = append(x.out.GraphDiagnostics, &d)
}

func (x *extractor) paramID(node graph.NodeID, kind graph.InjectionKind, method string, index int) graph.ParamID {
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

func (x *extractor) argOriginOf(origin di.ArgOrigin) graph.ArgOrigin {
	switch origin {
	case di.ArgOriginManual:
		return graph.ArgOriginManual
	case di.ArgOriginAutowiring:
		return graph.ArgOriginAutowiring
	case di.ArgOriginCompilerPass:
		return graph.ArgOriginCompilerPass
	case di.ArgOriginNone:
		return graph.ArgOriginNone
	}
	return graph.ArgOriginNone
}

func (x *extractor) bindOriginOf(origin di.BindOrigin) graph.BindOrigin {
	switch origin {
	case di.BindOriginManual:
		return graph.BindOriginManual
	case di.BindOriginAutobinding:
		return graph.BindOriginAutobinding
	case di.BindOriginCompilerPass:
		return graph.BindOriginCompilerPass
	}
	return graph.BindOriginManual
}

func (x *extractor) argKindOf(kind di.ArgKind) graph.ArgKind {
	switch kind {
	case di.ArgKindLiteral:
		return graph.ArgKindLiteral
	case di.ArgKindRef:
		return graph.ArgKindRef
	case di.ArgKindType:
		return graph.ArgKindType
	case di.ArgKindLabel:
		return graph.ArgKindLabel
	case di.ArgKindFlexibleSlice:
		return graph.ArgKindFlexibleSlice
	case di.ArgKindCompound:
		return graph.ArgKindCompound
	}
	return graph.ArgKindUnknown
}

func (x *extractor) resolutionOf(res di.Resolution) graph.Resolution {
	switch res {
	case di.ResolutionRef:
		return graph.ResolutionRef
	case di.ResolutionByType:
		return graph.ResolutionByType
	case di.ResolutionBySliceType:
		return graph.ResolutionBySliceType
	case di.ResolutionByElemType:
		return graph.ResolutionByElemType
	case di.ResolutionByLabel:
		return graph.ResolutionByLabel
	}
	return graph.ResolutionByType
}

// render formats a literal value for display. A value is arbitrary user data, so a
// broken Stringer must not take the whole graph down with it.
func (x *extractor) render(v any) (s string, truncated bool) {
	defer func() {
		if r := recover(); r != nil {
			s, truncated = "<panicked while formatting>", false
		}
	}()

	return util.Truncate(fmt.Sprintf("%v", v), x.cfg.LiteralMax)
}
