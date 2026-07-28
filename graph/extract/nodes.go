package extract

import (
	"reflect"
	"slices"
	"strings"

	"github.com/michalkurzeja/godi/v2/di"
	"github.com/michalkurzeja/godi/v2/graph"
	"github.com/michalkurzeja/godi/v2/internal/util"
)

func (x *extractor) buildNodes() {
	for scope, def := range x.container.ServiceDefinitionsSeq() {
		node := &graph.Node{
			ID:           x.svcIDs[def],
			Kind:         graph.NodeService,
			UUID:         def.ID().String(),
			Scope:        x.scopeIDs[scope],
			Type:         util.Signature(def.Type()),
			Labels:       x.labelStrings(def.Labels()),
			Lazy:         def.IsLazy(),
			Shared:       def.IsShared(),
			Autowired:    def.IsAutowired(),
			Instantiated: scope.Instantiated(def.ID()),
			Registered:   x.registered(def.RegisteredAt()),
		}
		node.Name, node.Signature, node.Defined = x.implementation(def)
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
			Labels:     x.labelStrings(def.Labels()),
			Lazy:       def.IsLazy(),
			Autowired:  def.IsAutowired(),
			Registered: x.registered(def.RegisteredAt()),
			Defined:    x.registered(def.DefinedAt()),
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
func (x *extractor) registered(loc di.Location) graph.Location {
	return graph.Location{File: loc.File, Line: loc.Line, Func: loc.Func}
}

// implementation is how the model names whatever provides a service.
//
// The engine picks which it is: the factory, or the value when the service was
// registered as one. This is the display half, a name a reader would recognise
// and a signature rather than a type.
func (x *extractor) implementation(def *di.ServiceDefinition) (name, signature string, at graph.Location) {
	implName, typ, loc := def.Implementation()
	if typ == nil {
		return "", "", graph.Location{}
	}
	return x.trimMethodWrapper(implName), x.funcSignature(typ), x.registered(loc)
}

// funcSignature is what a function takes and returns, as Go would write it.
//
// Not the value's own type. For a service registered as a value that is usually a
// named type, and a name is what a signature is not. The type is rebuilt unnamed,
// which leaves an ordinary factory's type as it already was.
func (x *extractor) funcSignature(typ reflect.Type) string {
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
// compiler writes, named after the method with a suffix of its own. The method is
// what the reader means.
func (x *extractor) trimMethodWrapper(name string) string {
	return strings.TrimSuffix(name, "-fm")
}

// labelStrings is a definition's labels as the model holds them: sorted, so that
// two graphs of the same wiring compare equal.
func (x *extractor) labelStrings(labels []di.Label) []string {
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
