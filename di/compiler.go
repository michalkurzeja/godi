package di

import (
	"cmp"
	"slices"

	"github.com/michalkurzeja/godi/v2/internal/errorsx"
)

type CompilerPass struct {
	name     string
	stage    CompilerStage
	priority int
	op       CompilerOp
}

func NewCompilerPass(name string, stage CompilerStage, op CompilerOp) *CompilerPass {
	return &CompilerPass{name: name, stage: stage, op: op}
}

func (p *CompilerPass) WithPriority(priority int) *CompilerPass {
	p.priority = priority
	return p
}

func (p *CompilerPass) Run(builder *ContainerBuilder) error {
	return p.op.Run(builder)
}

func (p *CompilerPass) String() string {
	return p.name
}

// CompilerOp is an operation, executed by the Compiler, that can modify the container.
type CompilerOp interface {
	Run(builder *ContainerBuilder) error
}

type CompilerOpFunc func(builder *ContainerBuilder) error

func (fn CompilerOpFunc) Run(builder *ContainerBuilder) error {
	return fn(builder)
}

type CompilerStage uint8

const (
	PreAutomation CompilerStage = iota
	Automation
	PreValidation
	Validation
	PreFinalization
	Finalization
	PostFinalization
	compilerPassStageCount
)

// Passes contains an ordered list of Compiler passes.
// It is organised into stages and priorities. This makes it possible
// to control when the pass is executed.
// The stages are executed sequentially, and the passes within a stage
// are executed by their priority: the higher the priority, the earlier
// the pass will run. If two passes have the same priority, they will
// be executed in the order they were added.
type Passes []*CompilerPass

func BasePasses(skipCycleValidation bool) Passes {
	passes := Passes{
		NewCompilerPass("interface binding", Automation, NewInterfaceBindingPass()),
		NewCompilerPass("autowiring", Automation, NewAutowiringPass()),
		NewCompilerPass("argument validation", Validation, NewArgValidationPass()),
		NewCompilerPass("eager initialization", Finalization, NewEagerInitPass()),
	}
	if !skipCycleValidation {
		passes = append(passes, NewCompilerPass("cycle validation", Validation, NewCycleValidationPass()))
	}
	return passes
}

func (passes Passes) sort() {
	slices.SortFunc(passes, func(a, b *CompilerPass) int {
		if a.stage != b.stage {
			return cmp.Compare(a.stage, b.stage)
		}
		if a.priority != b.priority {
			return cmp.Compare(a.priority, b.priority)
		}
		return 0
	})
}

// Compiler is responsible for configuration of the container after all user changes are done.
// It allows the user to hook into the compilation process using Compiler passes, making
// it possible to create services dynamically and automatically.
type Compiler struct {
	passes Passes
}

func NewCompiler(conf CompilerConfig) *Compiler {
	return &Compiler{passes: BasePasses(conf.SkipCycleValidation)}
}

func (c *Compiler) AddPass(pass *CompilerPass) {
	c.passes = append(c.passes, pass)
}

func (c *Compiler) Run(builder *ContainerBuilder) error {
	c.passes.sort()
	for _, pass := range c.passes {
		err := pass.Run(builder)
		if err != nil {
			return errorsx.Wrapf(err, "compiler pass (%s) returned an error", pass)
		}
	}
	return nil
}

type CompilerConfig struct {
	// SkipCycleValidation disables the cycle validation compiler pass.
	// In general, it's recommended to keep the cycle validation enabled, as it can detect user misconfiguration.
	// It is, however, a costly operation, so it can be disabled to increase the performance of the container building process.
	SkipCycleValidation bool
}

func NewCompilerConfig() CompilerConfig {
	return CompilerConfig{
		SkipCycleValidation: false,
	}
}
