package errorsx

import (
	"fmt"
)

func Wrap(err error, msg string) error {
	return &wrappedError{err: err, msg: msg}
}

func Wrapf(err error, format string, a ...any) error {
	return Wrap(err, fmt.Sprintf(format, a...))
}

type wrappedError struct {
	err error
	msg string
}

func (w *wrappedError) Error() string {
	if w.msg == "" {
		return w.err.Error()
	}
	return w.msg + ": " + w.err.Error()
}

func (w *wrappedError) Unwrap() error {
	return w.err
}
