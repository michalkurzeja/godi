package graph

import "fmt"

// Source is where a graph comes from. It is a function rather than an interface
// so that producing one costs the producer nothing structural: the container
// engine knows nothing about graphs, and graph/extract supplies the adapters
// that read it.
//
// Anything holding a Source can be handed a live container, a builder partway
// through compilation, or a graph read from a file - see Static.
type Source func(cfg Config) (*Graph, error)

// Static presents a graph you already have as a Source, so that anything taking
// one - serve above all - works on a graph read from a file as readily as on a
// live container.
//
// The config is ignored: extraction has already happened, and the config only
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
// It returns the whole graph. Narrow it with Graph.Select, which is a separate
// call because one extraction can answer several questions.
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
