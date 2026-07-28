package di

import (
	"cmp"
	"fmt"
	"iter"
	"slices"

	"github.com/michalkurzeja/godi/v2/internal/errorsx"
)

type CompilerPass struct {
	name     string
	stage    CompilerStage
	priority int
	op       CompilerOp

	// seq is when this pass was added. It is the last word on the order two
	// passes run in.
	//
	// The sort is not stable, and the queue is re-sorted whenever a pass is
	// added, so the order has to be recorded rather than relied on.
	seq int

	argOrigin  ArgOrigin  // What a slot filled by this pass means.
	bindOrigin BindOrigin // What a binding created by this pass means.
}

func NewCompilerPass(name string, stage CompilerStage, op CompilerOp) *CompilerPass {
	return &CompilerPass{
		name:       name,
		stage:      stage,
		op:         op,
		argOrigin:  ArgOriginCompilerPass,
		bindOrigin: BindOriginCompilerPass,
	}
}

func (p *CompilerPass) WithPriority(priority int) *CompilerPass {
	p.priority = priority
	return p
}

// withArgOrigin marks the arguments this pass fills as godi's own automation
// rather than a third-party extension.
//
// It is unexported on purpose. A pass may read an origin. It may not claim one
// of godi's.
func (p *CompilerPass) withArgOrigin(origin ArgOrigin) *CompilerPass {
	p.argOrigin = origin
	return p
}

// withBindOrigin marks the bindings this pass creates as godi's own automation
// rather than a third-party extension.
func (p *CompilerPass) withBindOrigin(origin BindOrigin) *CompilerPass {
	p.bindOrigin = origin
	return p
}

// Name is what the pass is called. Names are neither unique nor stable, so print
// it but never identify a pass by it.
func (p *CompilerPass) Name() string {
	return p.name
}

// Stage is when the pass runs.
func (p *CompilerPass) Stage() CompilerStage {
	return p.stage
}

// Priority orders the pass within its stage: the lower the number, the earlier
// it runs.
func (p *CompilerPass) Priority() int {
	return p.priority
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
		NewCompilerPass("interface binding", Automation, NewInterfaceBindingPass()).withBindOrigin(BindOriginAutobinding),
		NewCompilerPass("autowiring", Automation, NewAutowiringPass()).withArgOrigin(ArgOriginAutowiring),
		NewCompilerPass("argument validation", Validation, NewArgValidationPass()),
		NewCompilerPass("eager initialization", Finalization, NewEagerInitPass()),
	}
	if !skipCycleValidation {
		passes = append(passes, NewCompilerPass("cycle validation", Validation, NewCycleValidationPass()))
	}
	return passes
}

func (passes Passes) sort() {
	slices.SortFunc(passes, comparePasses)
}

// comparePasses is the whole of the order: the stage, then the priority, then
// when the pass was added.
func comparePasses(a, b *CompilerPass) int {
	if c := cmp.Compare(a.stage, b.stage); c != 0 {
		return c
	}
	if c := cmp.Compare(a.priority, b.priority); c != 0 {
		return c
	}
	return cmp.Compare(a.seq, b.seq)
}

// Compiler is responsible for configuration of the container after all user changes are done.
// It allows the user to hook into the compilation process using Compiler passes, making
// it possible to create services dynamically and automatically.
type Compiler struct {
	passes Passes
	// pending holds the passes a running pass registered. They join the queue
	// once that pass returns.
	pending Passes
	// added counts the passes registered so far. Each pass is stamped with it.
	added int

	// running is the pass in progress, done names the ones that have finished,
	// and failed the one that stopped compilation.
	//
	// A graph taken mid-build reports all three. Without them it reads as a
	// finished container with wiring missing.
	running   *CompilerPass
	done      []string
	failed    string
	autowired bool
}

func NewCompiler(conf CompilerConfig) *Compiler {
	c := &Compiler{}
	for _, pass := range BasePasses(conf.SkipCycleValidation) {
		c.AddPass(pass)
	}
	return c
}

// AddPass registers a pass.
//
// A pass may call this while it runs, to schedule work over what it found. The
// new pass joins the queue as soon as the one adding it returns.
func (c *Compiler) AddPass(pass *CompilerPass) {
	pass.seq = c.added
	c.added++

	if c.running != nil {
		c.pending = append(c.pending, pass)
		return
	}
	c.passes = append(c.passes, pass)
}

// Passes yields the passes registered so far, in the order they were added. The
// compiler sorts them into running order when it runs, so a pass reading this
// mid-run sees the queue as it stands, itself included.
//
// There is no way to remove a pass. Turn behaviour off per definition with
// NotAutowired or Lazy, or per container with SkipCycleValidation.
func (c *Compiler) Passes() iter.Seq[*CompilerPass] {
	return func(yield func(*CompilerPass) bool) {
		for _, pass := range c.passes {
			if !yield(pass) {
				return
			}
		}
		for _, pass := range c.pending {
			if !yield(pass) {
				return
			}
		}
	}
}

func (c *Compiler) Run(builder *ContainerBuilder) error {
	c.passes.sort()

	// Whatever is already wired, the user wired: the passes have not run yet.
	c.creditPendingWiring(builder.container, ArgOriginManual, BindOriginManual, "")

	// By index, re-reading the length: a pass may add a pass, and range would
	// take the slice header once.
	for i := 0; i < len(c.passes); i++ {
		pass := c.passes[i]

		c.running = pass
		err := pass.Run(builder)
		c.running = nil
		if err != nil {
			// Kept: the builder still stands, and its graph shows how far the
			// container got.
			c.failed = pass.name
			return errorsx.Wrapf(err, "compiler pass (%s) returned an error", pass)
		}
		c.done = append(c.done, pass.name)
		// Asked of the pass, not of its name. Nothing stops a user calling their
		// own pass "autowiring".
		c.autowired = c.autowired || pass.argOrigin == ArgOriginAutowiring
		c.creditPendingWiring(builder.container, pass.argOrigin, pass.bindOrigin, pass.name)

		err = c.schedulePending(pass, i+1)
		if err != nil {
			c.failed = pass.name
			return err
		}
	}
	return nil
}

// schedulePending puts the passes that pass registered into the queue, and keeps
// the part of the queue that has not run in order. next is where that part
// begins.
//
// A pass whose place is behind the one that added it is refused. Running it now
// would run the stages out of order, and running it later is not the place it
// asked for.
func (c *Compiler) schedulePending(pass *CompilerPass, next int) error {
	if len(c.pending) == 0 {
		return nil
	}

	added := c.pending
	c.pending = nil

	for _, p := range added {
		if comparePasses(p, pass) < 0 {
			return fmt.Errorf("compiler pass (%s) added pass (%s), which would have had to run before it", pass, p)
		}
	}

	c.passes = append(c.passes, added...)
	c.passes[next:].sort()
	return nil
}

// CompilerProgress says how far compilation has got.
//
// Anything reading a container mid-compilation needs it. Wiring that a later
// pass would add is not there yet, and without this it reads as wiring that is
// missing.
type CompilerProgress struct {
	// Stage in progress, and Pass within it. Both are empty outside a pass.
	Stage string
	Pass  string
	// Failed names the pass that returned an error, when compilation stopped
	// there.
	Failed string
	// Done names the passes that have finished, in the order they ran.
	Done []string
	// Autowired says godi's autowiring has run. After it, an argument still
	// unwired is one nothing is going to wire.
	Autowired bool
}

// Progress describes how far compilation has got, for anything reading the
// container while it is still going on.
func (c *Compiler) Progress() CompilerProgress {
	p := CompilerProgress{
		Failed:    c.failed,
		Done:      slices.Clone(c.done),
		Autowired: c.autowired,
	}
	if c.running != nil {
		p.Stage, p.Pass = c.running.stage.String(), c.running.name
	}
	return p
}

// creditPendingWiring names whoever changed the wiring since the last call, and
// only that wiring. Each pass is credited with its own work.
//
// It runs before the first pass and again after each one. That is what tells an
// argument the user wired from one a pass wired.
func (c *Compiler) creditPendingWiring(container *Container, args ArgOrigin, binds BindOrigin, pass string) {
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
