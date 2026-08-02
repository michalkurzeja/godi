package di

import (
	"fmt"
	"reflect"
	"runtime"
	"slices"
	"strings"
	"sync"

	"github.com/michalkurzeja/godi/v2/internal/util"
)

// godiModule is godi's own import path.
const godiModule = "github.com/michalkurzeja/godi/v2"

// Location is a place in the source: where a definition was registered, or
// where the function behind it is declared. An empty File means there is
// nothing worth pointing at.
type Location struct {
	File string
	Line int
	Func string
}

// IsZero reports whether nothing was recorded.
func (l Location) IsZero() bool { return l.File == "" && l.Line == 0 }

func (l Location) String() string {
	switch {
	case l.File == "":
		return ""
	case l.Line == 0:
		return l.File
	default:
		return fmt.Sprintf("%s:%d", l.File, l.Line)
	}
}

// wiringSet holds the packages that register definitions on someone's behalf. A
// frame in one of them says how a service got registered, not where, so the
// search walks past it to the caller.
//
// Only exact packages, never their subpackages: godi's own examples and tests
// register services too, and they are the caller, not the machinery.
//
// Registration usually happens once, from an init. Reading happens when a
// definition is asked where it came from. Neither is hot, so the lock costs
// nothing.
type wiringSet struct {
	mu    sync.RWMutex
	paths []string
}

var wiring = wiringSet{paths: []string{
	godiModule,             // The facade: Svc, Func, SvcVal.
	godiModule + "/di",     // The engine.
	godiModule + "/extras", // Helpers that build definitions.
}}

func (w *wiringSet) mark(path string) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if !slices.Contains(w.paths, path) {
		w.paths = append(w.paths, path)
	}
}

func (w *wiringSet) contains(fn string) bool {
	w.mu.RLock()
	defer w.mu.RUnlock()

	return slices.Contains(w.paths, w.packageOf(fn))
}

// packageOf takes the import path out of a qualified function name. Names look
// like "path/to/pkg.Func" or "path/to/pkg.(*Type).Method", so the path ends at
// the first dot after the last slash.
func (w *wiringSet) packageOf(fn string) string {
	slash := strings.LastIndexByte(fn, '/')
	dot := strings.IndexByte(fn[slash+1:], '.')
	if dot < 0 {
		return fn
	}
	return fn[:slash+1+dot]
}

// MarkWiringPackage says that the package at path registers definitions on
// someone else's behalf. A definition it creates is then reported against the
// caller, not against the helper.
//
// Without this, every service such a library registers looks as if it was
// declared on one line of that library.
//
//	func init() { di.MarkWiringPackage("github.com/acme/wiring") }
//
// The path must match exactly, never as a prefix. A package's own examples and
// tests register services too, and they are the caller, not the machinery.
func MarkWiringPackage(path string) {
	wiring.mark(path)
}

// sourceDepth is how far up the stack to keep. Three frames cover the deepest
// route into godi: SvcVal, Svc, the constructor. The rest is room for a helper of
// the caller's own. A fixed array keeps the capture off the heap.
const sourceDepth = 8

// source is where a definition was registered, held as program counters and
// turned into a file and a line only if something asks.
//
// Capturing costs about 150ns and resolving costs more, so resolving is
// deferred. Most containers are never graphed at all.
type source struct {
	pcs [sourceDepth]uintptr
	n   uint8
}

// captureSource records the stack where a definition is being created. Every
// definition goes through one of the two constructors that call this, so nothing
// has to opt in.
func captureSource() source {
	var s source
	s.n = uint8(runtime.Callers(2, s.pcs[:])) //nolint:gosec // G115: bounded by sourceDepth.
	return s
}

// Location resolves the captured stack to the first frame outside godi, which
// is whoever asked for the service. It is empty if nothing was captured.
func (s source) Location() Location {
	if s.n == 0 {
		return Location{}
	}

	var innermost runtime.Frame
	frames := runtime.CallersFrames(s.pcs[:s.n])
	for i := 0; ; i++ {
		frame, more := frames.Next()
		if i == 0 {
			innermost = frame
		}
		if !wiring.contains(frame.Function) {
			return Location{File: frame.File, Line: frame.Line, Func: frame.Function}
		}
		if !more {
			break
		}
	}

	// Registered by godi itself, with nobody else on the stack. There is nothing
	// else to report.
	return Location{File: innermost.File, Line: innermost.Line, Func: innermost.Function}
}

// declaredAt is where a function is written. It is empty when that function
// belongs to a package that wires on someone else's behalf.
//
// SvcVal is why: it wraps a value in a closure godi declared, and a reader sent
// there lands in godi instead of their own code.
func declaredAt(fn reflect.Value) Location {
	file, line := util.FuncLocation(fn)
	if file == "" || wiring.contains(util.FuncName(fn)) {
		return Location{}
	}
	return Location{File: file, Line: line}
}
