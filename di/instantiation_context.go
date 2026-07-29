package di

import (
	"fmt"
	"slices"
	"strings"

	"github.com/michalkurzeja/godi/v2/internal/errorsx"
)

// instantiationContext is one call into the container: the factories it
// currently has running, and the method calls it owes.
//
// Every factory of a call runs before any of its method calls does. A method
// call therefore never sees a service that is still being built, so wiring that
// loops back through one gives the same instance whichever end is asked for
// first.
type instantiationContext struct {
	svcDefStack      []*ServiceDefinition // Factories currently running, outermost first.
	methodCallsQueue []pendingCall        // Method calls owed, in the order their services were built.
}

func newInstantiationContext() *instantiationContext {
	return new(instantiationContext)
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
		ic.methodCallsQueue = append(ic.methodCallsQueue, pendingCall{def: def, svc: svc, call: call, scope: scope})
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

// pendingCall is a method call waiting for the factory chain to finish.
type pendingCall struct {
	def   *ServiceDefinition
	svc   any // The instance built, not the definition: a service that is not shared has several.
	call  *Method
	scope *Scope
}

// withInstantiationContext runs fn as one call into the container: everything
// fn builds is constructed first, then the method calls it queued run.
func withInstantiationContext[T any](fn func(ic *instantiationContext) (T, error)) (T, error) {
	ic := newInstantiationContext()

	v, err := fn(ic)
	if err != nil {
		return v, err
	}

	err = ic.executeAllMethodCalls()
	if err != nil {
		var zero T
		return zero, err
	}
	return v, nil
}
