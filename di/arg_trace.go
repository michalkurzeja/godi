package di

import (
	"reflect"
	"slices"
)

// ArgKind is the kind of an argument. The dependency graph reports it.
type ArgKind uint8

const (
	ArgKindLiteral ArgKind = iota
	ArgKindRef
	ArgKindType
	ArgKindLabel
	ArgKindFlexibleSlice
	ArgKindCompound
)

// Resolution says how an argument found what it resolved to.
type Resolution uint8

const (
	ResolutionRef Resolution = iota
	ResolutionByType
	ResolutionBySliceType
	ResolutionByElemType
	ResolutionByLabel
)

// ArgFaultKind is why an argument matched nothing.
//
// Not every fault is worth reporting. A label that matches nothing inside a
// compound that did resolve is not a problem, so the caller decides.
type ArgFaultKind uint8

const (
	ArgFaultNone ArgFaultKind = iota
	ArgFaultNoServicesOfType
	ArgFaultNoServicesWithLabel
	ArgFaultCircularBinding
)

// ArgFault is why an argument matched nothing, and the type or label it looked
// for.
type ArgFault struct {
	Kind  ArgFaultKind
	Type  reflect.Type
	Label Label
}

// BindingHop is one interface binding an argument resolved through.
type BindingHop struct {
	Interface reflect.Type
	// Scope is the scope that declared the binding. It is not always the scope
	// that resolves the argument.
	Scope      *Scope
	Origin     BindOrigin
	OriginPass string
}

// ArgTrace is how an argument resolved: what it matched, by which mechanism,
// and through which interface bindings.
//
// Resolving an argument returns its value. It does not say how the value was
// found. The dependency graph needs that, so a trace records it.
//
// Parts holds sub-arguments, in the order they were tried: the parts of a
// compound, or the argument a binding points to.
type ArgTrace struct {
	Kind ArgKind
	// Value is the literal itself, for ArgKindLiteral.
	Value any
	// Label is what was looked up, for ArgKindLabel.
	Label Label
	// Bindings are the hops traversed to reach this argument.
	Bindings []BindingHop
	// Matches are the definitions this argument resolved to, and By is how.
	Matches []ID
	By      Resolution
	Parts   []ArgTrace
	Fault   ArgFault
}

// TraceArg describes how the argument resolves in the scope.
func TraceArg(scope *Scope, arg Arg) ArgTrace {
	if arg == nil {
		return ArgTrace{}
	}
	return arg.trace(scope, tracePath{})
}

// tracePath is where a trace has got to: the bindings followed to reach the
// current argument, and the interfaces already followed.
//
// The interfaces are kept so that a binding pointing back at itself is reported
// instead of followed forever.
type tracePath struct {
	hops []BindingHop
	seen []reflect.Type
}

func (p tracePath) followed(typ reflect.Type) bool {
	return slices.Contains(p.seen, typ)
}

// through extends the path with a binding. Both slices are copied, so sibling
// sub-arguments of a compound cannot write into each other's path.
func (p tracePath) through(typ reflect.Type, bindScope *Scope, binding *InterfaceBinding) tracePath {
	hop := BindingHop{
		Interface:  binding.Interface(),
		Scope:      bindScope,
		Origin:     binding.origin,
		OriginPass: binding.originPass,
	}
	return tracePath{
		hops: append(p.hops[:len(p.hops):len(p.hops)], hop),
		seen: append(p.seen[:len(p.seen):len(p.seen)], typ),
	}
}

// matchType records a type match, following an interface binding if one covers
// the type.
func (t *ArgTrace) matchType(scope *Scope, typ reflect.Type, path tracePath, by Resolution) {
	if bindScope, binding, ok := scope.bindingInChain(typ); ok {
		t.follow(scope, typ, bindScope, binding, path)
		return
	}

	t.Matches, t.By = scope.GetServicesIDsByTypeInChain(typ), by
	if len(t.Matches) == 0 {
		t.Fault = ArgFault{Kind: ArgFaultNoServicesOfType, Type: typ}
	}
}

// follow traces the argument a binding points at. It stops if the binding leads
// back to an interface already followed.
func (t *ArgTrace) follow(scope *Scope, typ reflect.Type, bindScope *Scope, binding *InterfaceBinding, path tracePath) {
	if path.followed(typ) {
		t.Fault = ArgFault{Kind: ArgFaultCircularBinding, Type: typ}
		return
	}
	t.Parts = append(t.Parts, binding.BoundTo().trace(scope, path.through(typ, bindScope, binding)))
}
