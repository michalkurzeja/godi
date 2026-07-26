package di

import (
	"cmp"
	"fmt"
	"slices"

	"github.com/michalkurzeja/godi/v2/graph"
	"github.com/michalkurzeja/godi/v2/internal/errorsx"
)

type CompilerPass struct {
	name     string
	stage    CompilerStage
	priority int
	op       CompilerOp

	argOrigin  argOrigin  // What a slot filled by this pass means.
	bindOrigin bindOrigin // What a binding created by this pass means.
}

func NewCompilerPass(name string, stage CompilerStage, op CompilerOp) *CompilerPass {
	return &CompilerPass{
		name:       name,
		stage:      stage,
		op:         op,
		argOrigin:  argOriginCompilerPass,
		bindOrigin: bindOriginCompilerPass,
	}
}

func (p *CompilerPass) WithPriority(priority int) *CompilerPass {
	p.priority = priority
	return p
}

// withArgOrigin marks the arguments this pass fills as one of godi's own
// behaviours rather than a third-party extension.
func (p *CompilerPass) withArgOrigin(origin argOrigin) *CompilerPass {
	p.argOrigin = origin
	return p
}

// withBindOrigin marks the bindings this pass creates as one of godi's own
// behaviours rather than a third-party extension.
func (p *CompilerPass) withBindOrigin(origin bindOrigin) *CompilerPass {
	p.bindOrigin = origin
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

func (s CompilerStage) String() string {
	switch s {
	case PreAutomation:
		return "pre-automation"
	case Automation:
		return "automation"
	case PreValidation:
		return "pre-validation"
	case Validation:
		return "validation"
	case PreFinalization:
		return "pre-finalization"
	case Finalization:
		return "finalization"
	case PostFinalization:
		return "post-finalization"
	default:
		// compilerPassStageCount lands here: it counts the stages rather than
		// naming one, and there is nothing to call it.
		return fmt.Sprintf("stage %d", uint8(s))
	}
}

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
		NewCompilerPass("interface binding", Automation, NewInterfaceBindingPass()).withBindOrigin(bindOriginAutobinding),
		NewCompilerPass("autowiring", Automation, NewAutowiringPass()).withArgOrigin(argOriginAutowiring),
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

	// running is the pass in progress, done names the ones that have finished,
	// and failed the one that stopped compilation. A graph taken mid-build is
	// only readable if it says how much of the wiring had happened, and this is
	// where that is known.
	running   *CompilerPass
	done      []string
	failed    string
	autowired bool
}

func NewCompiler(conf CompilerConfig) *Compiler {
	return &Compiler{passes: BasePasses(conf.SkipCycleValidation)}
}

func (c *Compiler) AddPass(pass *CompilerPass) {
	c.passes = append(c.passes, pass)
}

func (c *Compiler) Run(builder *ContainerBuilder) error {
	c.passes.sort()

	// Whatever is already wired, the user wired: the passes have not run yet.
	c.creditPendingWiring(builder.container, argOriginManual, bindOriginManual, "")

	for _, pass := range c.passes {
		c.running = pass
		err := pass.Run(builder)
		c.running = nil
		if err != nil {
			// Kept, because the builder is still standing and its graph is the
			// picture of exactly how far the container got.
			c.failed = pass.name
			return errorsx.Wrapf(err, "compiler pass (%s) returned an error", pass)
		}
		c.done = append(c.done, pass.name)
		// Asked of the pass rather than of its name: what a pass fills is what
		// it says it fills, and nothing stops a user calling theirs "autowiring".
		c.autowired = c.autowired || pass.argOrigin == argOriginAutowiring
		c.creditPendingWiring(builder.container, pass.argOrigin, pass.bindOrigin, pass.name)
	}
	return nil
}

// snapshot describes how far compilation has got, for a graph taken while it is
// still going on.
func (c *Compiler) snapshot() *graph.Snapshot {
	s := &graph.Snapshot{
		Failed:    c.failed,
		Done:      slices.Clone(c.done),
		Autowired: c.autowired,
	}
	if c.running != nil {
		s.Stage, s.Pass = c.running.stage.String(), c.running.name
	}
	return s
}

// creditPendingWiring names whoever is responsible for the wiring changed since
// the last call, and only that wiring: each pass is credited with its own work
// and nothing else. Running it before the first pass, and again after each one,
// is what tells a hand-written argument apart from one godi or an extension
// supplied.
func (c *Compiler) creditPendingWiring(container *Container, args argOrigin, binds bindOrigin, pass string) {
	for slot := range container.slotsSeq() {
		if slot.dirty {
			slot.creditTo(args, pass)
		}
	}
	for binding := range container.bindingsSeq() {
		if binding.dirty {
			binding.creditTo(binds, pass)
		}
	}
}

type CompilerConfig struct {
	// SkipCycleValidation disables the cycle validation compiler pass.
	// In general, it's recommended to keep the cycle validation enabled, as it can detect user misconfiguration.
	// It is, however, a costly operation, so it can be disabled to increase the performance of the container building process.
	// Be aware that disabling the cycle validation can lead to stack overflow errors if the user creates a cycle in the container.
	SkipCycleValidation bool
}

func NewCompilerConfig() CompilerConfig {
	return CompilerConfig{
		SkipCycleValidation: false,
	}
}
