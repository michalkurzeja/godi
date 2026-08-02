package di

import (
	"bufio"
	"fmt"
	"io"
	"reflect"
	"strings"

	"github.com/michalkurzeja/godi/v2/internal/util"
)

// Print writes the contents of the scope, and of every scope nested in it, to
// the given writer.
//
// Deprecated: use the graph package with the text encoder, which gives you the
// whole container, the other formats, and the filters:
//
//	g, err := extract.From(c)
//	err = g.Encode(w, text.New())
func Print(s *Scope, w io.Writer) {
	if s == nil || s.container == nil {
		return
	}

	// Print returns nothing, so a failure has nowhere to go. The only one
	// possible is the writer's own.
	p := &printer{w: bufio.NewWriter(w), names: s.container.definitionNames()}
	p.scope(s, 0)
	_ = p.w.Flush()
}

// printer writes the outline. It stays plain and self-contained: nothing here may
// pull a graph encoder onto every godi binary's import path.
type printer struct {
	w     *bufio.Writer
	names map[ID]string
}

func (p *printer) linef(depth int, format string, args ...any) {
	_, _ = fmt.Fprintf(p.w, "%s%s\n", strings.Repeat("  ", depth), fmt.Sprintf(format, args...))
}

func (p *printer) scope(s *Scope, depth int) {
	p.linef(depth, "scope %s", p.scopeName(s))

	p.bindings(s, depth+1)
	p.services(s, depth+1)
	p.functions(s, depth+1)

	// Nested, because a child scope is only reachable through the definition
	// that declared it. The indentation says so.
	for child := range s.container.Scopes() {
		if child.Parent() == s {
			_, _ = p.w.WriteString("\n")
			p.scope(child, depth+1)
		}
	}
}

// scopeName is what to call a scope. A child scope's own name is a uuid, which
// says nothing to a reader, so it is named after the definition that declared
// it.
func (p *printer) scopeName(s *Scope) string {
	for _, def := range s.container.ServiceDefinitionsSeq() {
		if def.ChildScope() == s {
			return "children of " + util.Signature(def.Type())
		}
	}
	for _, def := range s.container.FunctionDefinitionsSeq() {
		if def.ChildScope() == s {
			return "children of " + def.Func().Name()
		}
	}
	return s.Name()
}

func (p *printer) bindings(s *Scope, depth int) {
	bindings := s.GetBindings()
	if len(bindings) == 0 {
		return
	}

	p.linef(depth, "bindings:")
	for _, binding := range bindings {
		origin, pass := binding.Origin()
		p.linef(depth+1, "%s -> %s  [%s]",
			util.Signature(binding.Interface()), binding.BoundTo(), bindOriginName(origin, pass))
	}
}

func (p *printer) services(s *Scope, depth int) {
	var defs []*ServiceDefinition
	for def := range s.ServiceDefinitionsSeq() {
		defs = append(defs, def)
	}
	if len(defs) == 0 {
		return
	}

	p.linef(depth, "services:")
	for _, def := range defs {
		p.linef(depth+1, "%s%s", util.Signature(def.Type()), flags(def.IsLazy(), def.IsShared(), def.IsAutowired(), s.Instantiated(def.ID()), def.Labels()))
		p.linef(depth+2, "factory: %s", def.FactoryName())

		p.args(def.EffectiveScope(), def.Factory().Args(), depth+2)

		for _, method := range def.MethodCalls() {
			p.linef(depth+2, "method call %s:", method.Name())
			p.args(def.EffectiveScope(), method.Args(), depth+3)
		}
	}
}

func (p *printer) functions(s *Scope, depth int) {
	var defs []*FunctionDefinition
	for def := range s.FunctionDefinitionsSeq() {
		defs = append(defs, def)
	}
	if len(defs) == 0 {
		return
	}

	p.linef(depth, "functions:")
	for _, def := range defs {
		p.linef(depth+1, "%s%s", def.Func().Name(), flags(def.IsLazy(), true, def.IsAutowired(), false, def.Labels()))
		p.args(def.EffectiveScope(), def.Func().Args(), depth+2)
	}
}

// args is one row per argument: what was asked for, who filled it, and what it
// resolved to.
func (p *printer) args(scope *Scope, args *ArgList, depth int) {
	slots := args.Slots()
	if len(slots) == 0 {
		return
	}

	p.linef(depth, "args:")
	for _, slot := range slots {
		head := fmt.Sprintf("%d <- %s", slot.Index(), util.Signature(slot.Type()))
		if !slot.IsFilled() {
			p.linef(depth+1, "%s  (not wired)", head)
			continue
		}

		origin, pass := slot.Origin()
		trace := TraceArg(scope, slot.Arg())

		// The value is left out on purpose. A literal is often a connection
		// string or a token, and this writes to whatever it is given.
		if trace.Kind == ArgKindLiteral {
			p.linef(depth+1, "%s = %s  [%s]", head, util.Signature(reflect.TypeOf(trace.Value)), argOriginName(origin, pass))
			continue
		}

		p.linef(depth+1, "%s  [%s]", head, argOriginName(origin, pass))
		matches := traceMatches(trace)
		if len(matches) == 0 {
			p.linef(depth+2, "(nothing)")
		}
		for _, id := range matches {
			p.linef(depth+2, "-> %s", p.names[id])
		}
	}
}

// traceMatches is everything an argument resolved to, in the order it did.
func traceMatches(t ArgTrace) []ID {
	ids := t.Matches
	for _, part := range t.Parts {
		ids = append(ids, traceMatches(part)...)
	}
	return ids
}

// definitionNames is what to call each definition in a row that points at it.
func (c *Container) definitionNames() map[ID]string {
	names := make(map[ID]string)
	for _, def := range c.ServiceDefinitionsSeq() {
		names[def.ID()] = util.Signature(def.Type())
	}
	for _, def := range c.FunctionDefinitionsSeq() {
		names[def.ID()] = def.Func().Name()
	}
	return names
}

// flags are the properties worth naming. Only the unusual half of each pair is
// printed: a lazy shared service is the default and says nothing. What is left is
// what someone chose.
func flags(lazy, shared, autowired, instantiated bool, labels []Label) string {
	var out []string
	if !lazy {
		out = append(out, "eager")
	}
	if !shared {
		out = append(out, "not shared")
	}
	if !autowired {
		out = append(out, "not autowired")
	}
	if instantiated {
		out = append(out, "instantiated")
	}
	for _, label := range labels {
		out = append(out, label.String())
	}

	if len(out) == 0 {
		return ""
	}
	return "  [" + strings.Join(out, ", ") + "]"
}

// Only an extension is worth naming. godi's own automation runs under a compiler
// pass too, and "autowiring (autowiring)" says nothing.
func argOriginName(origin ArgOrigin, pass string) string {
	switch origin {
	case ArgOriginNone:
		return "not wired"
	case ArgOriginManual:
		return "manual"
	case ArgOriginAutowiring:
		return "autowiring"
	case ArgOriginCompilerPass:
		if pass != "" {
			return "compiler-pass: " + pass
		}
		return "compiler-pass"
	}
	return "unknown"
}

func bindOriginName(origin BindOrigin, pass string) string {
	switch origin {
	case BindOriginManual:
		return "manual"
	case BindOriginAutobinding:
		return "autobinding"
	case BindOriginCompilerPass:
		if pass != "" {
			return "compiler-pass: " + pass
		}
		return "compiler-pass"
	}
	return "unknown"
}
