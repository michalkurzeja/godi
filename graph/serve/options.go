package serve

import "github.com/michalkurzeja/godi/v2/graph"

type config struct {
	enc     graph.Encoder
	extract []graph.Option
}

// Option configures the handler.
type Option func(*config)

// WithEncoder chooses what the page is drawn with. Defaults to the HTML viewer,
// which is the only format a browser can do anything with.
func WithEncoder(enc graph.Encoder) Option {
	return func(cfg *config) { cfg.enc = enc }
}

// WithExtractOptions applies these to every extraction. They matter when the
// source is a live container, which is asked again on each request; a graph read
// from a file was extracted once, already, by whoever wrote it.
func WithExtractOptions(opts ...graph.Option) Option {
	return func(cfg *config) { cfg.extract = opts }
}
