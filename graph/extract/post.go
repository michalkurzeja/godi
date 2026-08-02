package extract

import (
	"errors"
	"iter"
	"path"
	"slices"
	"strings"

	dbgraph "github.com/dominikbraun/graph"

	"github.com/michalkurzeja/godi/v2/di"
	"github.com/michalkurzeja/godi/v2/graph"
	"github.com/michalkurzeja/godi/v2/internal/util"
)

// markCollected flags the edges that feed a slot taking more than one dependency.
// An encoder can then draw a collection as such, even when only one service
// happens to match today.
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
			origin, pass := binding.Origin()
			entry := &graph.Binding{
				Scope:      scopeID,
				Interface:  util.Signature(binding.Interface()),
				Origin:     x.bindOriginOf(origin),
				OriginPass: pass,
				BoundTo:    binding.BoundTo().String(),
			}
			for _, id := range di.ResolveArgIDs(scope, binding.BoundTo()) {
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

// markUnwired says so about an argument nothing filled once nothing is going to
// fill it.
//
// An argument naming something that is not there said so where it failed to
// resolve. One that nothing filled fails no lookup, so this is where it gets its
// words.
//
// Before autowiring runs, an unfilled argument is work outstanding rather than a
// fault. Every argument is unwired then, so saying it of each would say nothing.
func (x *extractor) markUnwired() {
	if x.snapshot != nil && !x.snapshot.Autowired {
		return
	}

	for _, node := range x.out.Nodes {
		for _, p := range node.Params {
			if p.Unwired() {
				x.unresolved(p, p.Position()+" is not set")
			}
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
	// The diagnostics need no sorting. They are read off the elements they belong
	// to, and those are sorted here.
}

// locations yields every place in the source the graph points at, so that the
// common root can be taken out of all of them and put back onto any of them.
func (x *extractor) locations() iter.Seq[*graph.Location] {
	return func(yield func(*graph.Location) bool) {
		for i := range x.out.Diagnostics {
			if !yield(&x.out.Diagnostics[i].Location) {
				return
			}
		}
		for _, scope := range x.out.Scopes {
			for i := range scope.Diagnostics {
				if !yield(&scope.Diagnostics[i].Location) {
					return
				}
			}
		}
		for _, node := range x.out.Nodes {
			if !yield(&node.Registered) || !yield(&node.Declared) {
				return
			}
			for i := range node.Diagnostics {
				if !yield(&node.Diagnostics[i].Location) {
					return
				}
			}
			for _, p := range node.Params {
				for i := range p.Diagnostics {
					if !yield(&p.Diagnostics[i].Location) {
						return
					}
				}
			}
		}
	}
}

// trimSourceRoot takes the directory every path shares out of the paths and
// records it once. Absolute build-machine paths make the output long, unstable
// between machines and noisy to diff. Joining the root back on returns the
// original path.
func (x *extractor) trimSourceRoot() {
	var paths []string
	for loc := range x.locations() {
		if loc.File != "" {
			paths = append(paths, loc.File)
		}
	}

	root := x.commonDir(paths)
	if root == "" {
		return
	}

	x.out.SourceRoot = root
	for loc := range x.locations() {
		loc.File = strings.TrimPrefix(loc.File, root+"/")
	}
}

// commonDir is the longest directory prefix shared by every path. It is empty when
// there is nothing worth trimming: a container wired partly from the standard
// library shares only the filesystem root.
func (x *extractor) commonDir(paths []string) string {
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
