package graph

import "reflect"

// Source is anything a dependency graph can be extracted from. It is implemented
// by a built container and by *di.ContainerBuilder, so a compiler pass can graph
// the container mid-compilation, before autowiring has run.
type Source interface {
	Graph(cfg Config) *Graph
}

// Config tells the extractor what to put in the graph. Build it with New, or
// leave it zero for the defaults.
//
// The filtering options in select.go also arrive as an Option, but they are
// applied by Extract and Select rather than by the extractor: a Source is asked
// for the whole graph and narrowing it is a separate step. Passing a Config
// straight to Container.Graph therefore skips them.
type Config struct {
	// LiteralValues includes the values of literal arguments, not just their
	// types. Off by default: literals routinely carry connection strings and API
	// keys, and graphs get committed and pasted into issues.
	//
	// Values that format as a machine address - a func, a channel, a plain
	// pointer - are left out even when this is set, unless the type formats
	// itself: an address says nothing and differs on every run.
	LiteralValues bool
	// LiteralMax truncates included values to this many runes.
	LiteralMax int
	// Redactor, when set, is consulted for every literal value that would be
	// included. Returning redact replaces the value with the given text.
	Redactor func(typ reflect.Type, value any) (replacement string, redact bool)
	// NoLiterals drops literal arguments from the graph entirely.
	NoLiterals bool

	// filters narrow the graph once it has been extracted. They are unexported
	// because a Source never applies them: it is asked for the whole graph, and
	// Extract and Select are what narrow it. See select.go.
	filters []filter
}

// Option configures extraction.
type Option func(*Config)

// New builds a Config from the given options.
func New(opts ...Option) Config {
	var cfg Config
	for _, opt := range opts {
		opt(&cfg)
	}
	return cfg
}

// WithLiteralValues includes literal argument values, truncated to maxRunes.
// Pass zero or less for no truncation.
//
// Consider what ends up in the output before turning this on: a literal is often
// a DSN or a token.
func WithLiteralValues(maxRunes int) Option {
	return func(cfg *Config) {
		cfg.LiteralValues = true
		cfg.LiteralMax = maxRunes
	}
}

// WithRedactor filters literal values before they reach the graph. It is only
// consulted when values are included at all.
func WithRedactor(fn func(typ reflect.Type, value any) (replacement string, redact bool)) Option {
	return func(cfg *Config) { cfg.Redactor = fn }
}

// WithoutLiterals leaves literal arguments out of the graph entirely.
func WithoutLiterals() Option {
	return func(cfg *Config) { cfg.NoLiterals = true }
}
