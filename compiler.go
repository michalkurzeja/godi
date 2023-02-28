package di

import (
	"github.com/samber/lo"
)

type CompilerPassStage uint8

const (
	PreOptimisation CompilerPassStage = iota
	Optimisation
	PreValidation
	Validation
	PostValidation
	compilerPassStageNumber
)

// compilerPassConfig contains an ordered list of compiler passes.
// It is organised into stages and priorities. This makes it possible
// to control when the pass is executed.
// The stages are executed sequentially, and the passes within a stage
// are executed by their priority: the higher the priority, the earlier
// the pass will run. If two passes have the same priority, they will
// be executed in the order they were added.
type compilerPassConfig [compilerPassStageNumber]map[int][]CompilerPass

func newCompilerPassConfig() compilerPassConfig {
	return compilerPassConfig{
		Optimisation: {
			0: {
				NewInterfaceResolutionPass(),
				NewAutowirePass(),
			},
		},
		Validation: {
			0: {
				NewAliasValidationPass(),
				NewReferenceValidationPass(),
				NewCycleValidationPass(),
			},
		},
		PostValidation: {
			0: {
				NewEagerInitPass(),
			},
		},
	}
}

func (c compilerPassConfig) AddPass(stage CompilerPassStage, priority int, pass CompilerPass) {
	c[stage][priority] = append(c[stage][priority], pass)
}

func (c compilerPassConfig) ForEach(fn func(pass CompilerPass) error) error {
	for _, stage := range c {
		priorities := sortedDesc(lo.Keys(stage), func(priority int) int {
			return priority
		})

		for _, priority := range priorities {
			for _, pass := range stage[priority] {
				err := fn(pass)
				if err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// compiler is responsible for configuration of the container after all user changes are done.
// It allows the user to hook into the compilation process using compiler passes, making
// it possible to create services dynamically and automatically.
type compiler struct {
	config compilerPassConfig
}

func newCompiler() *compiler {
	return &compiler{config: newCompilerPassConfig()}
}

func (c *compiler) AddPass(stage CompilerPassStage, priority int, pass CompilerPass) {
	c.config.AddPass(stage, priority, pass)
}

func (c *compiler) Compile(builder *ContainerBuilder) error {
	err := c.config.ForEach(func(pass CompilerPass) error {
		return pass.Compile(builder)
	})
	if err != nil {
		return err
	}

	builder.container.finalise()

	return nil
}
