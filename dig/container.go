package dig

import (
	"errors"
	"io"

	di "github.com/michalkurzeja/godi"
)

// roContainer is a read-only container implementation.
type roContainer struct {
	c di.Container
}

func (c roContainer) Register(_ di.Node) error {
	return errors.New("cannot register nodes in read-only container")
}

func (c roContainer) Get(id string) (di.Node, error) {
	return c.c.Get(id)
}

func (c roContainer) Compile() error {
	return errors.New("cannot compile a read-only container")
}

func (c roContainer) Export(w io.Writer) error {
	return c.c.Export(w)
}
