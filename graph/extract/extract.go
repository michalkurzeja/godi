// Package extract reads a godi container and produces the graph model.
//
// It is the one place that knows both sides: the engine it reads and the model it
// writes. That is why it is a package of its own. The model stays free of the
// container engine, so anything that only reads a graph links none of it.
//
//	c, err := di.New().Services(...).Build()
//
//	g, err := extract.From(c.(*di.Container))
//	err = g.Encode(os.Stdout, dot.New())
package extract

import (
	"errors"

	"github.com/michalkurzeja/godi/v2/di"
	"github.com/michalkurzeja/godi/v2/graph"
)

// From builds the dependency graph of a built container.
//
// It returns the whole graph. Narrow it afterwards with Graph.Select, so that one
// extraction can answer several questions.
func From(c *di.Container, opts ...graph.Option) (*graph.Graph, error) {
	if c == nil {
		return nil, errors.New("extract: no container")
	}
	return newExtractor(c, graph.NewConfig(opts...), nil).extract(), nil
}

// FromBuilder builds the graph of a container that is still being built: from
// inside a compiler pass, or after a build that failed.
//
// The graph carries a graph.Snapshot saying how far compilation had got. Missing
// wiring then reads as work still to do, not as a broken container.
func FromBuilder(b *di.ContainerBuilder, opts ...graph.Option) (*graph.Graph, error) {
	if b == nil {
		return nil, errors.New("extract: no builder")
	}
	if b.Container() == nil {
		return nil, errors.New("extract: this builder has already been built; extract from the container it returned")
	}
	return newExtractor(b.Container(), graph.NewConfig(opts...), snapshotOf(b.Compiler().Progress())).extract(), nil
}

// Live presents a container as a graph.Source, extracting again on every call.
//
// It is what serves a graph over HTTP. The wiring can change under it, and each
// request should see what is there now.
func Live(c *di.Container) graph.Source {
	return func(cfg graph.Config) (*graph.Graph, error) {
		if c == nil {
			return nil, errors.New("extract: no container")
		}
		return newExtractor(c, cfg, nil).extract(), nil
	}
}

// LiveBuilder is Live for a container that is still being built.
func LiveBuilder(b *di.ContainerBuilder) graph.Source {
	return func(cfg graph.Config) (*graph.Graph, error) {
		if b == nil || b.Container() == nil {
			return nil, errors.New("extract: no container to graph")
		}
		return newExtractor(b.Container(), cfg, snapshotOf(b.Compiler().Progress())).extract(), nil
	}
}

// snapshotOf is how far compilation had got, in the model's own words.
func snapshotOf(p di.CompilerProgress) *graph.Snapshot {
	return &graph.Snapshot{
		Stage:     p.Stage,
		Pass:      p.Pass,
		Failed:    p.Failed,
		Done:      p.Done,
		Autowired: p.Autowired,
	}
}

type extractor struct {
	container *di.Container
	cfg       graph.Config
	// snapshot says how far compilation had got. It is nil for a container that
	// finished building.
	//
	// It decides what counts as missing. Before autowiring runs, an unwired
	// argument is work outstanding rather than a fault.
	snapshot *graph.Snapshot
	out      *graph.Graph

	scopeIDs map[*di.Scope]graph.ScopeID
	svcIDs   map[*di.ServiceDefinition]graph.NodeID
	funIDs   map[*di.FunctionDefinition]graph.NodeID
	byUUID   map[di.ID]graph.NodeID // Every definition, for resolving edge targets.
	minted   map[string]int         // Node ID collision counters.
}

func newExtractor(c *di.Container, cfg graph.Config, snapshot *graph.Snapshot) *extractor {
	return &extractor{
		container: c,
		cfg:       cfg,
		snapshot:  snapshot,
		out:       &graph.Graph{Schema: graph.Schema, Snapshot: snapshot},
		scopeIDs:  make(map[*di.Scope]graph.ScopeID),
		svcIDs:    make(map[*di.ServiceDefinition]graph.NodeID),
		funIDs:    make(map[*di.FunctionDefinition]graph.NodeID),
		byUUID:    make(map[di.ID]graph.NodeID),
		minted:    make(map[string]int),
	}
}

func (x *extractor) extract() *graph.Graph {
	x.assignIDs()
	x.buildNodes()
	x.markCollected()
	x.buildBindings()
	x.markCycles()
	x.markIncomplete()
	x.countDegrees()
	x.sortAll()
	x.trimSourceRoot()
	return x.out
}
