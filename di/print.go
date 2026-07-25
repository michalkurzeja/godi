package di

import (
	"io"

	"github.com/michalkurzeja/godi/v2/graph"
	"github.com/michalkurzeja/godi/v2/graph/text"
)

// Print writes the contents of the scope, and of every scope nested in it, to
// the given writer.
//
// Deprecated: use the graph package with the text encoder, which gives you the
// whole container, the other formats, and the filters:
//
//	g, err := graph.Extract(c)
//	err = g.Encode(w, text.New())
func Print(s *Scope, w io.Writer) {
	if s == nil || s.container == nil {
		return
	}

	g := s.container.Graph(graph.Config{})
	if id, ok := scopeID(g, s); ok {
		g = g.Select(graph.OnlyScopeTree(id))
	}

	// The signature has nowhere to report a failure, and the only one there can
	// be is the writer's own.
	_ = g.Encode(w, text.New())
}

// scopeID finds a scope in the graph. The container's own name for a scope is
// carried through to the model, and it is either "root" or a uuid, so it
// identifies the scope on its own.
func scopeID(g *graph.Graph, s *Scope) (graph.ScopeID, bool) {
	for _, scope := range g.Scopes {
		if scope.Name == s.Name() {
			return scope.ID, true
		}
	}
	return "", false
}
