package graph

import (
	"cmp"
	"slices"
	"strings"

	"github.com/michalkurzeja/godi/v2/graph/internal/render"
)

// Filters narrow a graph after it has been extracted.
//
// Past a hundred or so nodes, no layout engine and no format produces a picture
// worth looking at. Narrowing the question is what makes a real application
// readable.
//
// Filters work on the model, so every format gets them and no encoder contains
// filter logic.

// Matcher tests one node against one of its properties. The By* functions build
// them; All, Any and Not combine them.
type Matcher func(*Node) bool

// Patterns are globs in which * stands for any run of characters, including
// none. Each is tried against the full name and against the short form, so a
// node named "github.com/acme/app.(*Server)" is found by that, by
// "app.(*Server)", and by "github.com/acme/*".
func matches(patterns []string, full string) bool {
	short := render.Short(full)
	for _, pattern := range patterns {
		if glob(pattern, full) || glob(pattern, short) {
			return true
		}
	}
	return false
}

func glob(pattern, s string) bool {
	parts := strings.Split(pattern, "*")
	if len(parts) == 1 {
		return pattern == s
	}

	if !strings.HasPrefix(s, parts[0]) {
		return false
	}
	s = s[len(parts[0]):]

	// Each middle part has to appear in order. Taking the first occurrence of
	// each is enough, because the parts cannot overlap.
	for _, part := range parts[1 : len(parts)-1] {
		i := strings.Index(s, part)
		if i < 0 {
			return false
		}
		s = s[i+len(part):]
	}
	return strings.HasSuffix(s, parts[len(parts)-1])
}

// ByType matches services by the type they provide, and functions by their
// signature.
func ByType(patterns ...string) Matcher {
	return func(n *Node) bool { return matches(patterns, n.Type) }
}

// ByName matches nodes by the name of their factory or function.
func ByName(patterns ...string) Matcher {
	return func(n *Node) bool { return matches(patterns, n.Name) }
}

// ByLabel matches nodes carrying any of the given labels.
func ByLabel(patterns ...string) Matcher {
	return func(n *Node) bool {
		return slices.ContainsFunc(n.Labels, func(label string) bool {
			return matches(patterns, label)
		})
	}
}

// ByID matches nodes by their graph ID.
func ByID(patterns ...string) Matcher {
	return func(n *Node) bool { return matches(patterns, string(n.ID)) }
}

// ByFile matches nodes registered or defined in a matching file. Paths are
// relative to the graph's SourceRoot, so "internal/*" reaches a whole tree.
func ByFile(patterns ...string) Matcher {
	return func(n *Node) bool {
		return matches(patterns, n.Registered.File) || matches(patterns, n.Defined.File)
	}
}

// All matches a node every one of the given matchers accepts. With no matchers it
// matches everything, as an empty "and" does.
func All(matchers ...Matcher) Matcher {
	return func(n *Node) bool {
		return !slices.ContainsFunc(matchers, func(match Matcher) bool { return !match(n) })
	}
}

// Any matches a node at least one of the given matchers accepts.
func Any(matchers ...Matcher) Matcher {
	return func(n *Node) bool {
		return slices.ContainsFunc(matchers, func(match Matcher) bool { return match(n) })
	}
}

// Not inverts a matcher.
func Not(match Matcher) Matcher {
	return func(n *Node) bool { return !match(n) }
}

// ---------------------------------------------------------------- filters ---

// Filter narrows a graph. Select takes them, and the functions below are the only
// way to build one. A Filter is therefore always a question about the graph, never
// about how it was extracted.
//
// A filter sees the whole graph. One that follows edges has to look past what is
// currently kept to know what it is cutting off.
type Filter struct {
	// wiring marks a filter that takes away a kind of wiring rather than a set of
	// nodes. Those run first, whatever order they were given in, because they
	// change what "reachable" means.
	//
	// Hiding method calls has to hide what only a method call reached. Otherwise
	// the answer would depend on which filter the caller wrote down first.
	wiring bool
	apply  func(*Graph, *selection)
}

func newFilter(apply func(*Graph, *selection)) Filter {
	return Filter{apply: apply}
}

func newWiringFilter(apply func(*Graph, *selection)) Filter {
	return Filter{wiring: true, apply: apply}
}

// unlimited is a hop count with no limit. unset is a direction the caller did not
// mention.
const (
	unlimited = -1
	unset     = -2
)

// FocusOption limits how far Focus follows the wiring.
type FocusOption func(*reach)

type reach struct{ consumers, dependencies int }

// Dependencies follows what the selection asks for, for at most hops edges.
func Dependencies(hops int) FocusOption { return func(r *reach) { r.dependencies = hops } }

// Consumers follows what asks for the selection, for at most hops edges.
func Consumers(hops int) FocusOption { return func(r *reach) { r.consumers = hops } }

// Focus keeps the matched nodes and what surrounds them, dropping the rest.
//
// With no options it follows the wiring as far as it goes in both directions,
// which is the whole connected component. Naming one direction takes the other
// out. Focus(match, Dependencies(3)) is "this and the three levels it is built
// from", not "and everything that uses it as well".
func Focus(match Matcher, opts ...FocusOption) Filter {
	r := reach{consumers: unset, dependencies: unset}
	for _, opt := range opts {
		opt(&r)
	}
	switch {
	case r.consumers == unset && r.dependencies == unset:
		r.consumers, r.dependencies = unlimited, unlimited
	case r.consumers == unset:
		r.consumers = 0
	case r.dependencies == unset:
		r.dependencies = 0
	}

	return newFilter(func(g *Graph, s *selection) {
		var seeds []NodeID
		for _, n := range g.Nodes {
			if s.has(n.ID) && match(n) {
				seeds = append(seeds, n.ID)
			}
		}

		keep := s.neighbourhood(g, seeds, r)
		for _, n := range g.Nodes {
			if s.has(n.ID) && !keep[n.ID] {
				s.cut(n.ID)
			}
		}
	})
}

// Exclude drops the nodes the matcher accepts. Unlike Focus it says nothing about
// what it removed. You named these, so their absence is not news.
func Exclude(match Matcher) Filter {
	return newFilter(func(g *Graph, s *selection) {
		for _, n := range g.Nodes {
			if match(n) {
				s.drop(n.ID)
			}
		}
	})
}

// ExcludeTypes drops services by the type they provide.
func ExcludeTypes(patterns ...string) Filter { return Exclude(ByType(patterns...)) }

// ExcludeLabels drops nodes carrying any of the given labels.
func ExcludeLabels(patterns ...string) Filter { return Exclude(ByLabel(patterns...)) }

// OnlyScope keeps the nodes of the matching scopes. A scope matches on its ID,
// on the container's own name for it, or on the name a reader would see.
func OnlyScope(patterns ...string) Filter {
	return newFilter(func(g *Graph, s *selection) {
		wanted := make(map[ScopeID]bool)
		for _, scope := range g.Scopes {
			if matches(patterns, string(scope.ID)) ||
				matches(patterns, scope.Name) ||
				matches(patterns, scope.Label()) {
				wanted[scope.ID] = true
			}
		}
		for _, n := range g.Nodes {
			if !wanted[n.Scope] {
				s.drop(n.ID)
			}
		}
	})
}

// OnlyScopeTree keeps the named scopes and everything nested inside them.
//
// It takes IDs rather than patterns. The caller usually has a scope in hand, and
// a scope ID is a path full of punctuation that a pattern would read as
// wildcards.
func OnlyScopeTree(ids ...ScopeID) Filter {
	return newFilter(func(g *Graph, s *selection) {
		roots := make(map[ScopeID]bool, len(ids))
		for _, id := range ids {
			roots[id] = true
		}

		// Walked from each scope up to the root, rather than down from the
		// named ones, so the answer does not depend on the order of g.Scopes.
		within := func(id ScopeID) bool {
			for id != "" {
				if roots[id] {
					return true
				}
				scope, ok := g.Scope(id)
				if !ok {
					return false
				}
				id = scope.Parent
			}
			return false
		}

		keep := make(map[ScopeID]bool, len(g.Scopes))
		for _, scope := range g.Scopes {
			keep[scope.ID] = within(scope.ID)
		}
		for _, n := range g.Nodes {
			if !keep[n.Scope] {
				s.drop(n.ID)
			}
		}
	})
}

// OnlyRoots keeps the nodes nothing injects: the top of every dependency tree.
// That is how you find the entry points of an application, and the wiring nothing
// uses. The two look the same from here.
func OnlyRoots() Filter {
	return newFilter(func(g *Graph, s *selection) {
		for _, n := range g.Nodes {
			if !n.Root {
				s.drop(n.ID)
			}
		}
	})
}

// HideMethodCalls drops the arguments injected through method calls, and the edges
// into them. The services stay: only that way of reaching them goes.
//
// A service nothing else asks for is then left standing on its own. That is what
// hiding the wiring actually does.
func HideMethodCalls() Filter {
	return newWiringFilter(func(g *Graph, s *selection) {
		for _, n := range g.Nodes {
			for _, p := range n.Params {
				if p.Kind == InjectMethodArg || p.Kind == InjectMethodReceiver {
					delete(s.params, p.ID)
				}
			}
		}
	})
}

// MaxNodes keeps at most n nodes, the most connected first, and says on each
// survivor how many of its neighbours went.
//
// It is a last resort for a graph too big to draw at all. Which n nodes matter is
// a question only you can answer, and Focus is how you answer it.
func MaxNodes(n int) Filter {
	return newFilter(func(g *Graph, s *selection) {
		alive := make([]*Node, 0, len(g.Nodes))
		for _, node := range g.Nodes {
			if s.has(node.ID) {
				alive = append(alive, node)
			}
		}
		if len(alive) <= n {
			return
		}

		// Ties break on ID so that the same graph always yields the same view.
		slices.SortFunc(alive, func(a, b *Node) int {
			if c := cmp.Compare(b.InDegree+b.OutDegree, a.InDegree+a.OutDegree); c != 0 {
				return c
			}
			return cmp.Compare(a.ID, b.ID)
		})
		for _, node := range alive[max(n, 0):] {
			s.cut(node.ID)
		}
	})
}

// -------------------------------------------------------------- selection ---

// selection is what survives so far. Filters only ever remove from it, so none of
// them has to know about the others.
type selection struct {
	nodes  map[NodeID]bool
	params map[ParamID]bool
	// gone records nodes a limit removed, as opposed to ones the caller named. It
	// is what lets the result say the graph continues past its edge.
	gone map[NodeID]bool
}

func newSelection(g *Graph) *selection {
	s := &selection{
		nodes:  make(map[NodeID]bool, len(g.Nodes)),
		params: make(map[ParamID]bool),
		gone:   make(map[NodeID]bool),
	}
	for _, n := range g.Nodes {
		s.nodes[n.ID] = true
		for _, p := range n.Params {
			s.params[p.ID] = true
		}
	}
	return s
}

func (s *selection) has(id NodeID) bool { return s.nodes[id] }

func (s *selection) drop(id NodeID) { delete(s.nodes, id) }

func (s *selection) cut(id NodeID) {
	delete(s.nodes, id)
	s.gone[id] = true
}

// neighbourhood walks out from the seeds along the edges between nodes that are
// still selected, up to the given number of hops in each direction.
//
// Each direction is walked on its own. Following both at once would let a path
// change direction partway: down to a dependency, then back up to something else
// that uses it. That reaches the seed's siblings, which are not on any path
// through it.
func (s *selection) neighbourhood(g *Graph, seeds []NodeID, r reach) map[NodeID]bool {
	seen := make(map[NodeID]bool, len(seeds))
	for _, id := range seeds {
		seen[id] = true
	}

	walk := func(hops int, next func(NodeID) []*Edge, far func(*Edge) NodeID) {
		// Its own record of where it has been, so that a node the other
		// direction already found does not stop this one walking through it.
		reached := make(map[NodeID]bool, len(seeds))
		frontier := seeds
		for _, id := range seeds {
			reached[id] = true
		}

		for i := 0; hops == unlimited || i < hops; i++ {
			var found []NodeID
			for _, id := range frontier {
				for _, e := range next(id) {
					to := far(e)
					if !reached[to] && s.has(to) && s.params[e.Param] {
						reached[to] = true
						seen[to] = true
						found = append(found, to)
					}
				}
			}
			if len(found) == 0 {
				return
			}
			frontier = found
		}
	}

	walk(r.dependencies, g.OutEdges, func(e *Edge) NodeID { return e.To })
	walk(r.consumers, g.InEdges, func(e *Edge) NodeID { return e.From })

	return seen
}

// ---------------------------------------------------------------- rebuild ---

// Select returns the graph narrowed by the given filters.
//
// Narrowing is a separate step from extraction. What a literal looks like is
// settled while the graph is built and cannot be revisited here, which is why an
// extraction Option is not a Filter.
//
// The result is a new graph. Nodes and params are copied, because their counts
// change. Edges and scopes are shared, because they do not.
func (g *Graph) Select(filters ...Filter) *Graph {
	// Nothing to narrow, and nothing to narrow it with. Encode guards against a
	// nil graph too: these are the two things you do to one, and neither should
	// be the call that panics.
	if g == nil || len(filters) == 0 {
		return g
	}

	sel := newSelection(g)
	// Two passes, so that a filter taking away a kind of wiring runs before any
	// filter that follows it. A zero Filter is skipped: only graph.Filter{}
	// produces one, and that should do nothing rather than panic.
	for _, wiring := range []bool{true, false} {
		for _, f := range filters {
			if f.apply != nil && f.wiring == wiring {
				f.apply(g, sel)
			}
		}
	}
	return g.rebuild(sel)
}

func (g *Graph) rebuild(sel *selection) *Graph {
	out := &Graph{
		Schema:           g.Schema,
		SourceRoot:       g.SourceRoot,
		GraphDiagnostics: g.GraphDiagnostics,
		Snapshot:         g.Snapshot,
	}

	kept := make(map[NodeID]*Node, len(sel.nodes))
	for _, n := range g.Nodes {
		if !sel.has(n.ID) {
			continue
		}

		node := *n
		node.InDegree, node.OutDegree, node.Elided = 0, 0, 0
		node.Params = nil
		for _, p := range n.Params {
			if !sel.params[p.ID] {
				continue
			}
			param := *p
			param.EdgeCount = 0
			node.Params = append(node.Params, &param)
		}

		kept[n.ID] = &node
		out.Nodes = append(out.Nodes, &node)
	}

	params := make(map[ParamID]*Param, len(sel.params))
	for _, n := range out.Nodes {
		for _, p := range n.Params {
			params[p.ID] = p
		}
	}

	for _, e := range g.Edges {
		from, to := kept[e.From], kept[e.To]

		// An edge whose other end went is not drawn, but it is the reason a
		// surviving node can say the graph carries on past it.
		if from != nil && sel.gone[e.To] {
			from.Elided++
		}
		if to != nil && sel.gone[e.From] {
			to.Elided++
		}

		if from == nil || to == nil || !sel.params[e.Param] {
			continue
		}
		from.OutDegree++
		to.InDegree++
		if p := params[e.Param]; p != nil {
			p.EdgeCount++
		}
		out.Edges = append(out.Edges, e)
	}

	out.Scopes = g.keptScopes(kept)
	out.Bindings = keptBindings(g.Bindings, out.Edges, kept)
	return out
}

// keptScopes keeps the scopes that still hold a node, and the ancestors that
// contain them. A scope box with nothing in it says nothing, but a gap in the
// middle of a path would misplace the scopes below it.
func (g *Graph) keptScopes(kept map[NodeID]*Node) []*Scope {
	wanted := make(map[ScopeID]bool)
	for _, n := range kept {
		for id := n.Scope; id != ""; {
			if wanted[id] {
				break
			}
			wanted[id] = true

			scope, ok := g.Scope(id)
			if !ok {
				break
			}
			id = scope.Parent
		}
	}

	var out []*Scope
	for _, scope := range g.Scopes {
		if wanted[scope.ID] {
			out = append(out, scope)
		}
	}
	return out
}

// keptBindings drops the bindings of scopes that went, points the rest at the
// targets that remain, and counts them against the edges still drawn. A binding
// reported as unused in a narrowed graph is unused in that graph.
func keptBindings(bindings []*Binding, edges []*Edge, kept map[NodeID]*Node) []*Binding {
	type hop struct {
		scope ScopeID
		iface string
	}

	used := make(map[hop]int)
	for _, e := range edges {
		for _, b := range e.Bindings {
			used[hop{b.Scope, b.Interface}]++
		}
	}

	var out []*Binding
	for _, b := range bindings {
		targets := make([]NodeID, 0, len(b.Targets))
		for _, id := range b.Targets {
			if kept[id] != nil {
				targets = append(targets, id)
			}
		}
		if len(targets) == 0 && len(b.Targets) > 0 {
			continue
		}

		binding := *b
		binding.Targets = targets
		binding.EdgeCount = used[hop{b.Scope, b.Interface}]
		out = append(out, &binding)
	}
	return out
}
