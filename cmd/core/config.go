package core

type Config struct {
	StrictMode bool `koanf:"strictmode"`
}

func DefaultConfig() Config {
	return Config{
		StrictMode: true,
	}
}
