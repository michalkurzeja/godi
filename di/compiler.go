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

	// seq is when this pass was added, and the last word on the order two
	// passes run in. Sorting is not stable, and stability would say nothing
	// across repeated sorts of a slice that has grown in between, so the order
	// the docs promise is written down rather than hoped for.
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

// withArgOrigin marks the arguments this pass fills as one of godi's own
// behaviours rather than a third-party extension. Unexported on purpose:
// reading provenance is open to a pass, claiming godi's own is not.
func (p *CompilerPass) withArgOrigin(origin ArgOrigin) *CompilerPass {
	p.argOrigin = origin
	return p
}

// withBindOrigin marks the bindings this pass creates as one of godi's own
// behaviours rather than a third-party extension.
func (p *CompilerPass) withBindOrigin(origin BindOrigin) *CompilerPass {
	p.bindOrigin = origin
	return p
}

// Name is what the pass is called. It is not unique and not stable: two passes
// may share a name, so it is worth reading and worth printing, and never worth
// identifying a pass by.
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
	// once it returns, rather than being appended to a slice the loop has
	// already taken the header of and would never look at again.
	pending Passes
	// added counts the passes registered so far, and is what each of them is
	// stamped with.
	added int

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
	c := &Compiler{}
	for _, pass := range BasePasses(conf.SkipCycleValidation) {
		c.AddPass(pass)
	}
	return c
}

// AddPass registers a pass. Called from inside a pass - which is a fair thing
// to want: discover the services, then schedule work over them - the new pass
// joins the queue as soon as the one adding it returns.
func (c *Compiler) AddPass(pass *CompilerPass) {
	pass.seq = c.added
	c.added++

	if c.running != nil {
		c.pending = append(c.pending, pass)
		return
	}
	c.passes = append(c.passes, pass)
}

// Passes yields the passes registered so far, in the order they were added.
// Sorting into running order happens when the compiler runs, so a pass reading
// this mid-run sees the queue as it stands, including itself.
//
// It is there to be read. What the built-in passes do is godi's own, and
// turning behaviour off is expressed where it belongs - per definition, with
// NotAutowired and Lazy, and per container with SkipCycleValidation - rather
// than by dismantling the pipeline.
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

	// By index, and re-reading the length: a pass may add a pass, and ranging
	// would take the slice header once and never see it.
	for i := 0; i < len(c.passes); i++ {
		pass := c.passes[i]

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

// schedulePending puts the passes that pass registered into the queue, keeping
// what is left of it in order. next is where the part that has not run yet
// begins.
//
// A pass whose place is behind the one that added it is refused: running it now
// would run the stages out of order, and running it "later" would not be the
// place it asked for. There is no honest answer to "insert me before the pass
// that is already running".
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

// CompilerProgress says how far compilation has got. Anything reading a
// container mid-compilation needs it: wiring a later pass would have added is
// simply not there yet, and without this it reads as wiring that is missing.
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

// creditPendingWiring names whoever is responsible for the wiring changed since
// the last call, and only that wiring: each pass is credited with its own work
// and nothing else. Running it before the first pass, and again after each one,
// is what tells a hand-written argument apart from one godi or an extension
// supplied.
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
