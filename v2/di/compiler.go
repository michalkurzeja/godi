package di

import (
	"github.com/samber/lo"
)

type CompilerPassStage uint8

const (
	PreOptimisation CompilerPassStage = iota
	optimisation
	PostOptimisation
	PreValidation
	validation
	PostValidation
	compilerPassStageNumber
)

type compilerPassConfig [compilerPassStageNumber]map[int][]CompilerPass

func newCompilerPassConfig() compilerPassConfig {
	return compilerPassConfig{
		optimisation: {
			0: {
				NewInterfaceResolutionPass(),
				NewAutowirePass(),
			},
		},
		validation: {
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

func (c compilerPassConfig) Add(stage CompilerPassStage, priority int, pass CompilerPass) {
	c[stage][priority] = append(c[stage][priority], pass)
}

func (c compilerPassConfig) ForEach(fn func(pass CompilerPass) error) error {
	for _, stage := range c {
		priorities := sorted(lo.Keys(stage), func(prio int) int {
			return prio
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

type compiler struct {
	config compilerPassConfig
}

func newCompiler() *compiler {
	return &compiler{config: newCompilerPassConfig()}
}

func (c *compiler) Add(stage CompilerPassStage, priority int, pass CompilerPass) {
	c.config.Add(stage, priority, pass)
}

func (c *compiler) Compile(builder *ContainerBuilder) error {
	return c.config.ForEach(func(pass CompilerPass) error {
		return pass.Compile(builder)
	})
}
