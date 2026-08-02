package di

import (
	"errors"
	"fmt"

	"github.com/michalkurzeja/godi/v2/di"
	"github.com/michalkurzeja/godi/v2/graph"
	"github.com/michalkurzeja/godi/v2/graph/extract"
)

// Unwrapper is a Container standing in front of another one. Implement it and
// godi can still find the container underneath, so a decorator does not cost you
// the graph.
//
// Unwrap must get closer to the container godi built. One that returns itself,
// or two that return each other, never arrive.
type Unwrapper interface {
	Unwrap() Container
}

// Graph is the dependency graph of a container godi built: what depends on what,
// what is passed where, and how each dependency got there.
//
//	c, err := di.New().Services(...).Build()
//
//	g, err := di.Graph(c)
//	err = g.Encode(os.Stdout, dot.New())
//
// It returns the whole graph. Narrow it afterwards with graph.Graph.Select, so
// that one extraction can answer several questions.
func Graph(c Container, opts ...graph.Option) (*graph.Graph, error) {
	container, err := engineContainer(c)
	if err != nil {
		return nil, err
	}
	return extract.From(container, opts...)
}

// LiveGraph presents a container as a graph.Source, extracting again on every
// call. It is what serves a graph over HTTP: the wiring can change under it, and
// each request should see what is there now. See graph/serve.
func LiveGraph(c Container) graph.Source {
	return graph.SourceFunc(func(cfg graph.Config) (*graph.Graph, error) {
		container, err := engineContainer(c)
		if err != nil {
			return nil, err
		}
		return extract.Live(container).Graph(cfg)
	})
}

// engineContainer is the container godi built, out of whatever stands in front
// of it. Build hands back the Container interface, and reading a graph needs the
// container itself.
func engineContainer(c Container) (*di.Container, error) {
	for {
		switch v := c.(type) {
		case *di.Container:
			if v == nil {
				return nil, errors.New("godi: no container")
			}
			return v, nil
		case Unwrapper:
			c = v.Unwrap()
		case nil:
			return nil, errors.New("godi: no container")
		default:
			return nil, fmt.Errorf("godi: cannot graph a %T: godi did not build it", c)
		}
	}
}
