package di

import (
	"runtime"
	"slices"
	"strings"
)

// godiModule is godi's own import path.
const godiModule = "github.com/michalkurzeja/godi/v2"

// wiringPackages are the packages that register definitions on someone's
// behalf. A frame in one of them says how a service got registered, not where,
// so the search walks past it to the caller.
//
// Only these exact packages, never their subpackages: godi's own examples and
// tests register services too, and they are the caller, not the machinery.
var wiringPackages = []string{
	godiModule,             // The facade: Svc, Func, SvcVal.
	godiModule + "/di",     // The engine.
	godiModule + "/extras", // Helpers that build definitions.
}

// sourceDepth is how far up the stack to keep. Three frames covers the deepest
// route in - SvcVal to Svc to the constructor - and the rest is room for a
// helper of the caller's own. A fixed array keeps the capture off the heap.
const sourceDepth = 8

// source is where a definition was registered, held as program counters and
// turned into a file and a line only if something asks.
//
// Capturing costs about 150ns and resolving costs more again, so the work is
// deferred: almost no container is ever graphed, and the ones that are are
// graphed once.
type source struct {
	pcs [sourceDepth]uintptr
	n   uint8
}

// captureSource records the stack where a definition is being created. Every
// definition passes through the two constructors that call this - the facade's,
// an extension's and a compiler pass's alike - so nothing has to opt in.
func captureSource() source {
	var s source
	s.n = uint8(runtime.Callers(2, s.pcs[:])) //nolint:gosec // G115: bounded by sourceDepth.
	return s
}

// Location resolves the captured stack to the first frame outside godi, which
// is whoever asked for the service. It returns zeroes if nothing was captured.
func (s source) Location() (file string, line int, fn string) {
	if s.n == 0 {
		return "", 0, ""
	}

	var innermost runtime.Frame
	frames := runtime.CallersFrames(s.pcs[:s.n])
	for i := 0; ; i++ {
		frame, more := frames.Next()
		if i == 0 {
			innermost = frame
		}
		if !isWiring(frame.Function) {
			return frame.File, frame.Line, frame.Function
		}
		if !more {
			break
		}
	}

	// Registered by godi itself, with nobody else on the stack. Nothing else to
	// report, so report that.
	return innermost.File, innermost.Line, innermost.Function
}

func isWiring(fn string) bool {
	return slices.Contains(wiringPackages, packageOf(fn))
}

// packageOf takes the import path out of a qualified function name. Names look
// like "path/to/pkg.Func" or "path/to/pkg.(*Type).Method", so the path ends at
// the first dot after the last slash.
func packageOf(fn string) string {
	slash := strings.LastIndexByte(fn, '/')
	dot := strings.IndexByte(fn[slash+1:], '.')
	if dot < 0 {
		return fn
	}
	return fn[:slash+1+dot]
}
