// Package json encodes a dependency graph as the interchange format.
//
// It is what the library writes when a build fails and what the godi CLI reads,
// so the same file is both the record of a failure and something to draw. The
// writing itself lives on the model - a failed build must be able to write one
// without linking an encoder - and this package is that, reachable the same way
// as any other format.
package json

import (
	"io"

	"github.com/michalkurzeja/godi/v2/graph"
)

// Encoder writes a graph as JSON.
type Encoder struct {
	cfg config
}

// New returns a JSON encoder.
func New(opts ...Option) *Encoder {
	var cfg config
	for _, opt := range opts {
		opt(&cfg)
	}
	return &Encoder{cfg: cfg}
}

func (e *Encoder) Format() graph.Format {
	return graph.Format{Name: "json", Ext: "json", MediaType: "application/json"}
}

func (e *Encoder) Encode(g *graph.Graph, w io.Writer) error {
	return g.WriteJSONIndented(w, e.cfg.indent)
}

type config struct {
	indent string
}

// Option configures the encoder.
type Option func(*config)

// Indent writes each level of nesting with the given string. Defaults to none:
// the format is written far more often than it is read by eye.
func Indent(indent string) Option {
	return func(cfg *config) { cfg.indent = indent }
}
