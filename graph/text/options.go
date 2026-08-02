package text

type config struct {
	locations bool
	maxType   int
}

// Option configures the text encoder.
type Option func(*config)

// WithoutLocations leaves out where each definition was registered and declared.
//
// Worth reaching for when the output is compared: paths depend on the machine
// that built the binary, so a golden file that includes them only matches on
// the machine that wrote it.
func WithoutLocations() Option {
	return func(cfg *config) { cfg.locations = false }
}

// MaxType truncates type names to this many runes. Pass zero or less to leave
// them whole, which generics make worth having as a choice: an instantiated
// type can run to several lines on its own.
func MaxType(runes int) Option {
	return func(cfg *config) { cfg.maxType = runes }
}
