package graph

import "reflect"

// Config tells the extractor what to put in the graph. Build it with NewConfig,
// or leave it zero for the defaults.
//
// It says nothing about which nodes to keep. A source is asked for the whole
// graph, and narrowing it is a separate step. See Filter and Graph.Select.
type Config struct {
	// Literals says how much of a literal argument to put in the graph.
	Literals LiteralMode
	// LiteralMax truncates included values to this many runes.
	LiteralMax int
	// Redactor, when set, is consulted for every literal value that would be
	// included. Returning redact replaces the value with the given text.
	Redactor func(typ reflect.Type, value any) (replacement string, redact bool)

	// Diagnostics says how much of what the compiler objected to a graph carries.
	Diagnostics DiagnosticMode
	// DiagnosticRedactor, when set, is consulted for every diagnostic message
	// that would be included. Returning redact replaces the message with the
	// given text.
	DiagnosticRedactor func(message string) (replacement string, redact bool)
}

// DiagnosticMode says how much of a diagnostic a graph carries.
//
// It is a setting of its own rather than part of LiteralMode. A diagnostic
// message is written by whoever wrote the code that failed, so it is a different
// question from what a literal holds, and the answers need not match.
type DiagnosticMode uint8

const (
	// DiagnosticMessages carries what the compiler said. It is the default: a
	// graph of a failed build that will not say what failed is no use.
	//
	// A message is arbitrary text. An error from a factory carries whatever that
	// factory put in it, which can be a connection string or a token, and graphs
	// get committed and pasted into issues.
	DiagnosticMessages DiagnosticMode = iota
	// DiagnosticMarks carries the severity and the pass, and no message. What is
	// broken still shows as broken; why is left out.
	DiagnosticMarks
	// DiagnosticNone leaves diagnostics out of the graph entirely.
	DiagnosticNone
)

// LiteralMode says how much of a literal argument a graph carries. It is one
// setting rather than two flags, which could contradict each other.
type LiteralMode uint8

const (
	// LiteralTypes carries the type of each literal and not its value. It is the
	// default, because literals often carry connection strings and API keys, and
	// graphs get committed and pasted into issues.
	LiteralTypes LiteralMode = iota
	// LiteralValues carries the values too.
	//
	// Values that format as a machine address are left out even so, unless the
	// type formats itself. A func, a channel or a plain pointer prints an
	// address, which says nothing and differs on every run.
	LiteralValues
	// LiteralNone leaves literal arguments out of the graph entirely.
	LiteralNone
)

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
// Consider what ends up in the output before turning this on. A literal is often
// a DSN or a token.
func WithLiteralValues(maxRunes int) Option {
	return func(cfg *Config) {
		cfg.Literals = LiteralValues
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
	return func(cfg *Config) { cfg.Literals = LiteralNone }
}

// WithDiagnosticMarks keeps what is broken and leaves out what a compiler pass
// said about it.
//
// Consider it for a graph that leaves the machine it was taken on. A message
// from a factory carries whatever that factory put in its error.
func WithDiagnosticMarks() Option {
	return func(cfg *Config) { cfg.Diagnostics = DiagnosticMarks }
}

// WithoutDiagnostics leaves diagnostics out of the graph entirely.
func WithoutDiagnostics() Option {
	return func(cfg *Config) { cfg.Diagnostics = DiagnosticNone }
}

// WithDiagnosticRedactor filters diagnostic messages before they reach the
// graph. It is only consulted when messages are included at all.
func WithDiagnosticRedactor(fn func(message string) (replacement string, redact bool)) Option {
	return func(cfg *Config) { cfg.DiagnosticRedactor = fn }
}
