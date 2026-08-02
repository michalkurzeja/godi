package di

import (
	"errors"
	"reflect"
	"runtime"
)

// errResolveFromUserCode is what a factory gets for asking the container to
// build something while it is itself being built. It would otherwise wait for
// the build lock its own caller holds, which never comes free.
var errResolveFromUserCode = errors.New(
	"a factory, method call or function must not resolve from the container that is building it: declare the dependency as an argument",
)

// callUserCode is where the container hands control to code the caller wrote: a
// factory, a method call or a function.
//
// insideUserCode finds a resolve from inside a factory by looking for this frame
// on the stack. So it must not be inlined away, and it must stay the only route
// into user code, or the check goes blind.
//
//go:noinline
func callUserCode(fn reflect.Value, args []reflect.Value, variadic bool) []reflect.Value {
	if variadic {
		return fn.CallSlice(args)
	}
	return fn.Call(args)
}

// userCodeFrame is the fully qualified name of callUserCode, as the runtime
// reports it.
const userCodeFrame = "github.com/michalkurzeja/godi/v2/di.callUserCode"

// stackProbeLimit caps how far insideUserCode walks. A dependency chain deeper
// than this escapes the check; the rule it enforces holds either way.
const stackProbeLimit = 512

// insideUserCode reports whether this goroutine is already running a factory, a
// method call or a function the container invoked.
//
// runtime.Callers reads the stack of the goroutine that calls it, so this asks
// about ourselves. Go exposes no goroutine identity, and none is needed here.
func insideUserCode() bool {
	pcs := make([]uintptr, 64)

	for skip := 2; skip < stackProbeLimit; {
		n := runtime.Callers(skip, pcs)
		if n == 0 {
			return false
		}

		frames := runtime.CallersFrames(pcs[:n])
		for {
			frame, more := frames.Next()
			if frame.Function == userCodeFrame {
				return true
			}
			if !more {
				break
			}
		}

		skip += n
	}

	return false
}
