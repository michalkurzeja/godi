package di

type Config struct {
	CompilerConfig
	// Defaults are the properties a definition takes when its registration does
	// not set them. Each container has its own.
	Defaults Defaults
}

func NewConfig() Config {
	return Config{
		CompilerConfig: NewCompilerConfig(),
		Defaults:       NewDefaults(),
	}
}

// Defaults are the properties a definition takes when its registration does not
// set them.
type Defaults struct {
	Lazy      bool
	Shared    bool
	Autowired bool
}

// NewDefaults returns godi's defaults, as the process-wide SetDefault functions
// leave them.
func NewDefaults() Defaults {
	return Defaults{Lazy: defaultLazy, Shared: defaultShared, Autowired: defaultAutowired}
}
