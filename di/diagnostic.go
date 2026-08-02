package di

import (
	"errors"
	"fmt"

	"github.com/michalkurzeja/godi/v2/internal/errorsx"
)

// Severity says how much a Diagnostic matters. An error stops compilation; a
// warning and an info note do not.
type Severity uint8

const (
	SeverityInfo    Severity = iota // Worth knowing; nothing is wrong.
	SeverityWarning                 // Something will not work as read.
	SeverityError                   // Something is broken.
)

func (s Severity) String() string {
	switch s {
	case SeverityInfo:
		return "info"
	case SeverityWarning:
		return "warning"
	case SeverityError:
		return "error"
	default:
		return fmt.Sprintf("severity %d", uint8(s))
	}
}

// Site is what a Diagnostic is about: the container, a scope, a definition, or
// one argument of one. The narrowest thing it names is where the dependency
// graph shows it.
//
// The sites are the levels the graph can store a diagnostic on, which is why
// there are these and no others. Build one with the At functions below; the
// fields are unexported so that nothing can name a place godi cannot find.
type Site struct {
	scope *Scope
	svc   *ServiceDefinition
	fun   *FunctionDefinition
	slot  *Slot
}

// AtContainer is for a fault that belongs to no one definition: a pass that
// could not be scheduled, a definition that never made it into the container.
func AtContainer() Site { return Site{} }

func AtScope(scope *Scope) Site { return Site{scope: scope} }

func AtService(def *ServiceDefinition) Site { return Site{svc: def} }

func AtFunction(def *FunctionDefinition) Site { return Site{fun: def} }

// AtServiceArg is one argument of a service: of its factory or of one of its
// method calls. The slot says which; the definition is what owns it.
func AtServiceArg(def *ServiceDefinition, slot *Slot) Site {
	return Site{svc: def, slot: slot}
}

func AtFunctionArg(def *FunctionDefinition, slot *Slot) Site {
	return Site{fun: def, slot: slot}
}

func (s Site) Scope() *Scope                 { return s.scope }
func (s Site) Service() *ServiceDefinition   { return s.svc }
func (s Site) Function() *FunctionDefinition { return s.fun }
func (s Site) Slot() *Slot                   { return s.slot }

// Diagnostic is something a compiler pass has to say about the container: an
// argument it could not resolve, a service whose factory failed, a warning about
// wiring that will work but probably should not.
//
// Message is what a reader is told, and it is what the dependency graph carries.
// Err is what Build returns for this diagnostic, wrapped however the pass likes.
// The two are separate because they answer to different readers: the graph shows
// the message beside the thing it is about, and the error has to stand alone in a
// terminal.
//
// A pass reports one with ContainerBuilder.Report. It does not fill in Pass: the
// Compiler credits whoever was running.
type Diagnostic struct {
	Severity Severity
	Site     Site
	Message  string
	// Err is what Build returns for this diagnostic. It is nil for anything that
	// does not fail the build, and an error-severity diagnostic without one is
	// reported as its message.
	Err error
	// At is where in the source this is about, for a diagnostic whose site cannot
	// say. A definition that would not parse never became a node with a location
	// of its own, and the file and line are then all a reader has to go on.
	//
	// It is a location rather than words in the message so that a reader can be
	// sent there: the HTML page makes it a link into an editor.
	At   Location
	Pass string
}

// err is what compilation fails with. An error-severity diagnostic always has
// one, whether or not the pass supplied it.
func (d Diagnostic) err() error {
	if d.Err != nil {
		return d.Err
	}
	return errors.New(d.Message)
}

// newDiagnosticError builds a diagnostic from an error the pass has in hand,
// wrapping it the way that pass words its failures. The message stays the bare
// fault, so the graph shows it against an element that already says where it is.
func newDiagnosticError(site Site, err error, wrapFormat string, a ...any) Diagnostic {
	return Diagnostic{
		Severity: SeverityError,
		Site:     site,
		Message:  err.Error(),
		Err:      errorsx.Wrapf(err, wrapFormat, a...),
	}
}
