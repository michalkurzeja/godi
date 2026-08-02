package di

import (
	"fmt"
	"slices"
	"strings"

	"github.com/michalkurzeja/godi/v2/internal/errorsx"
)

// instantiationContext is one call into the container: the factories it
// currently has running, the method calls it owes, and what it has built so far.
//
// Every factory of a call runs before any of its method calls does. A method
// call therefore never sees a service that is still being built.
//
// A call that builds anything holds the container's build lock until it ends, so
// only one call constructs at a time. What it builds is private to it until
// commit has run the method calls and published the instances.
type instantiationContext struct {
	container *Container

	svcDefStack      []*ServiceDefinition // Factories currently running, outermost first.
	methodCallsQueue []pendingMethodCall  // Method calls owed, in the order their services were built.

	staged     map[ID]stagedInstance // Built by this call, not yet visible to any other.
	holdsBuild bool                  // Whether this call has taken the build lock.
}

// stagedInstance is a shared service this call has built, and the scope that
// will hold it once it is published.
type stagedInstance struct {
	scope *Scope
	svc   any
}

func newInstantiationContext(container *Container) *instantiationContext {
	return &instantiationContext{container: container}
}

// beginBuild takes the build lock, so that one call constructs at a time. A call
// that builds a whole chain takes it once and keeps it until release.
//
// A caller already inside a factory is refused. It holds the lock itself, so
// waiting for it would never end.
func (ic *instantiationContext) beginBuild() error {
	if ic.holdsBuild {
		return nil
	}
	if insideUserCode() {
		return errResolveFromUserCode
	}

	ic.container.buildMu.Lock()
	ic.holdsBuild = true

	return nil
}

// release ends the call. Anything staged but never published is dropped, so a
// factory that failed or panicked leaves the container as it found it.
func (ic *instantiationContext) release() {
	clear(ic.staged)

	if ic.holdsBuild {
		ic.container.buildMu.Unlock()
		ic.holdsBuild = false
	}
}

// stage records a service this call has built. Only this call can see it until
// commit publishes it.
func (ic *instantiationContext) stage(scope *Scope, def *ServiceDefinition, svc any) {
	if ic.staged == nil {
		ic.staged = make(map[ID]stagedInstance)
	}
	ic.staged[def.ID()] = stagedInstance{scope: scope, svc: svc}
}

func (ic *instantiationContext) stagedInstance(id ID) (any, bool) {
	staged, ok := ic.staged[id]
	return staged.svc, ok
}

// commit runs the method calls this call owes, then publishes what it built.
// Nothing it built is visible anywhere else until this returns, so no other
// call can be handed a service whose method calls have not run.
func (ic *instantiationContext) commit() error {
	err := ic.executeAllMethodCalls()
	if err != nil {
		return err
	}

	ic.publish()

	return nil
}

func (ic *instantiationContext) publish() {
	if len(ic.staged) == 0 {
		return
	}

	ic.container.mu.Lock()
	defer ic.container.mu.Unlock()

	for id, staged := range ic.staged {
		staged.scope.instances[id] = staged.svc
	}
	clear(ic.staged)
}

// pushDefinition records that the definition's factory is about to run. It
// fails if that factory is already running: the service depends on itself.
func (ic *instantiationContext) pushDefinition(def *ServiceDefinition) error {
	if i := slices.Index(ic.svcDefStack, def); i >= 0 {
		return fmt.Errorf("circular dependency: %s", ic.cycle(i, def))
	}
	ic.svcDefStack = append(ic.svcDefStack, def)
	return nil
}

func (ic *instantiationContext) popDefinition() {
	ic.svcDefStack = ic.svcDefStack[:len(ic.svcDefStack)-1]
}

// cycle names the factories from the first sighting of def back to def.
func (ic *instantiationContext) cycle(from int, def *ServiceDefinition) string {
	names := make([]string, 0, len(ic.svcDefStack)-from+1)
	for _, d := range ic.svcDefStack[from:] {
		names = append(names, d.String())
	}
	return strings.Join(append(names, def.String()), " -> ")
}

// enqueueMethodCalls puts the method calls of a service godi has just built at
// the back of the queue.
func (ic *instantiationContext) enqueueMethodCalls(def *ServiceDefinition, svc any, scope *Scope) {
	for _, call := range def.MethodCalls() {
		ic.methodCallsQueue = append(ic.methodCallsQueue, pendingMethodCall{def: def, svc: svc, call: call, scope: scope})
	}
}

// executeAllMethodCalls runs the method calls owed, oldest first. A call may
// build services of its own; those add their calls to the back of the queue and
// are run in turn.
func (ic *instantiationContext) executeAllMethodCalls() error {
	for len(ic.methodCallsQueue) > 0 {
		p := ic.methodCallsQueue[0]
		ic.methodCallsQueue = ic.methodCallsQueue[1:]

		err := p.call.execute(ic, p.scope, p.svc)
		if err != nil {
			return errorsx.Wrapf(err, "failed to execute method %s of service %s", p.call, p.def)
		}
	}
	return nil
}

// pendingMethodCall is a method call waiting for the factory chain to finish.
type pendingMethodCall struct {
	def   *ServiceDefinition
	svc   any // The instance built, not the definition: a service that is not shared has several.
	call  *Method
	scope *Scope
}

// withInstantiationContext runs fn as one call into the container: everything
// fn builds is constructed first, then the method calls it queued run, then what
// it built becomes visible.
func withInstantiationContext[T any](container *Container, fn func(ic *instantiationContext) (T, error)) (T, error) {
	ic := newInstantiationContext(container)
	defer ic.release()

	v, err := fn(ic)
	if err != nil {
		return v, err
	}

	err = ic.commit()
	if err != nil {
		var zero T
		return zero, err
	}
	return v, nil
}
