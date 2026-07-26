package di

type Config struct {
	CompilerConfig
	// Defaults are the properties a definition takes when whoever registered it
	// did not choose. Per container, so two in one process can differ.
	Defaults Defaults
}

func NewConfig() Config {
	return Config{
		CompilerConfig: NewCompilerConfig(),
		Defaults:       NewDefaults(),
	}
}

// Defaults are the properties a definition takes when nothing else says.
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
