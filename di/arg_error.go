package di

import "errors"

// argError pins a failure to the argument that caused it, so that the dependency
// graph can show it there rather than against the whole definition.
//
// Resolving an argument fails deep inside a chain of factories, and by the time
// the failure reaches a compiler pass the only thing left of "which argument" is
// the wording. This carries it as data.
//
// It holds the fault twice over, because two readers want different amounts of
// it: cause is what is wrong with this argument, which is what the graph shows
// beside it, and err is the chain the build failed with, which has to stand alone
// in a terminal.
type argError struct {
	site  Site
	cause error
	err   error
}

func (e *argError) Error() string { return e.err.Error() }

func (e *argError) Unwrap() error { return e.err }

// deepestArgError is the innermost argument a failure can be pinned to, and nil
// for a failure that is about no argument at all.
//
// A factory that failed because its dependency failed produces one of these per
// level: the outer says an argument would not resolve, the inner says why. The
// inner one names the argument to change, and its cause is the fault itself
// rather than another definition's whole failure.
//
// errors.As answers with the outermost, so the search carries on from inside each
// one it finds. It walks joined errors too, which is how a slice argument that
// collected several services reports the one that failed.
func deepestArgError(err error) *argError {
	var deepest *argError
	for {
		var found *argError
		if !errors.As(err, &found) {
			return deepest
		}
		deepest, err = found, found.Unwrap()
	}
}
