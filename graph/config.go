package graph

import "reflect"

// Config tells the extractor what to put in the graph. Build it with NewConfig,
// or leave it zero for the defaults.
//
// It says nothing about which nodes to keep: a source is asked for the whole
// graph, and narrowing it is a separate step. See Filter and Graph.Select.
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
}

// Option configures extraction. Narrowing a graph down is a Filter instead.
type Option func(*Config)

// NewConfig builds a Config from the given options.
func NewConfig(opts ...Option) Config {
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
