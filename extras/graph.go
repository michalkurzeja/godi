package extras

import (
	"errors"

	"github.com/michalkurzeja/godi/v2/di"
	"github.com/michalkurzeja/godi/v2/graph"
	"github.com/michalkurzeja/godi/v2/graph/extract"
)

// CaptureGraph is a compiler pass that hands the dependency graph, as it stands
// at the given stage, to capture. It is how you see the wiring partway through
// compilation: after autowiring but before validation, say.
//
//	var midway *graph.Graph
//	c, err := godi.New().
//		Services(...).
//		CompilerPasses(extras.CaptureGraph(di.PreValidation, func(g *graph.Graph) error {
//			midway = g
//			return nil
//		})).
//		Build()
//
// The graph says which passes had run when it was taken, so wiring a later pass
// would have added does not read as wiring that is missing.
//
// Stages run in order, and passes within a stage in priority order, so the
// stage - with WithPriority where a stage is not precise enough - is what
// chooses the moment. An error from capture fails the build, like any other
// pass.
func CaptureGraph(stage di.CompilerStage, capture func(*graph.Graph) error, opts ...graph.Option) *di.CompilerPass {
	return di.NewCompilerPass("graph snapshot", stage, di.CompilerOpFunc(func(builder *di.ContainerBuilder) error {
		if capture == nil {
			return errors.New("cannot capture the graph: no capture function")
		}
		g, err := extract.FromBuilder(builder, opts...)
		if err != nil {
			return err
		}
		return capture(g)
	}))
}
