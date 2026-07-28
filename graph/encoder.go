package graph

import (
	"io"
)

// Format identifies an output format.
type Format struct {
	Name      string // Short lowercase name, e.g. "dot".
	Ext       string // File extension, without the dot.
	MediaType string
}

// Encoder writes a Graph in a concrete format.
//
// Implement it to add a format. The model is plain data, so an encoder needs
// nothing from godi beyond this package.
type Encoder interface {
	Format() Format
	Encode(g *Graph, w io.Writer) error
}

// Validator is an optional Encoder capability. When an encoder implements it,
// Encode checks the graph before writing anything. A format that cannot represent
// a given graph then fails before producing half a file.
type Validator interface {
	Check(g *Graph) error
}
