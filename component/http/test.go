package http

func TestConfig() Config {
	return Config{
		InternalInterface: InterfaceConfig{
			Address: ":8081",
			BaseURL: "http://localhost:8081",
		},
		PublicInterface: InterfaceConfig{
			Address: ":8080",
			BaseURL: "http://localhost:8080",
		},
	}
}
