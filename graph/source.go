package graph

import "fmt"

// Source is where a graph comes from.
//
// It is a function rather than an interface, so nothing has to implement it to
// produce a graph. The container engine knows nothing about graphs, and
// graph/extract supplies the adapters that read it.
//
// Anything holding a Source can be handed a live container, a builder partway
// through compilation, or a graph read from a file. See Static.
type Source func(cfg Config) (*Graph, error)

// Static presents a graph you already have as a Source. Anything that takes one,
// graph/serve above all, then works on a graph read from a file as readily as on
// a live container.
//
// The config is ignored. Extraction has already happened, and the config only
// says what to extract.
func Static(g *Graph) Source {
	return func(Config) (*Graph, error) {
		if g == nil {
			return nil, fmt.Errorf("graph: no graph")
		}
		return g, nil
	}
}

// Extract asks the source for the whole graph, with the given options.
//
// Narrow it afterwards with Graph.Select. That is a separate call, so one
// extraction can answer several questions.
func Extract(src Source, opts ...Option) (*Graph, error) {
	if src == nil {
		return nil, fmt.Errorf("graph: nil source")
	}

	g, err := src(NewConfig(opts...))
	if err != nil {
		return nil, err
	}
	if g == nil {
		return nil, fmt.Errorf("graph: the source produced no graph")
	}
	return g, nil
}
