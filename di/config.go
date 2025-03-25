package di

type Config struct {
	CompilerConfig
}

func NewConfig() Config {
	return Config{
		CompilerConfig: NewCompilerConfig(),
	}
}
