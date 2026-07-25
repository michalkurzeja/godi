package graph

import "fmt"

// Extract builds the dependency graph of src.
//
// src is a built container, or a *di.ContainerBuilder if you are inside a
// compiler pass and want to see the wiring before autowiring runs.
//
// It returns the whole graph. Narrow it with Graph.Select, which is a separate
// call because one extraction can answer several questions.
func Extract(src Source, opts ...Option) (*Graph, error) {
	if src == nil {
		return nil, fmt.Errorf("graph: nil source")
	}

	g := src.Graph(NewConfig(opts...))
	if g == nil {
		// A mock container satisfies Source but has nothing to describe.
		return nil, fmt.Errorf("graph: %T produced no graph", src)
	}

	return g, nil
}
