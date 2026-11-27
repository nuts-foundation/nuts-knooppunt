package http

func TestConfig() Config {
	return Config{
		InternalInterface: InterfaceConfig{
			Address:     ":8081",
			BaseURL:     "http://localhost:8081",
			MaxBodySize: 10 << 20,
		},
		PublicInterface: InterfaceConfig{
			Address:     ":8080",
			BaseURL:     "http://localhost:8080",
			MaxBodySize: 1 << 20,
		},
	}
}
