package http

func TestConfig() Config {
	return Config{
		InternalInterface: InterfaceConfig{
			Listener: ":8081",
			BaseURL:  "http://localhost:8081",
		},
		PublicInterface: InterfaceConfig{
			Listener: ":8080",
			BaseURL:  "http://localhost:8080",
		},
	}
}
