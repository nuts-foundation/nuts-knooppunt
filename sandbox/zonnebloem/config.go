package main

import (
	"fmt"
	"net/url"
)

// config is what De Zonnebloem's EHR needs from its environment: where its own
// FHIR store is, and the port to listen on.
type config struct {
	port        string
	fhirBaseURL *url.URL
}

func configFromEnv(getenv func(string) string) (config, error) {
	cfg := config{port: getenv("PORT")}
	if cfg.port == "" {
		cfg.port = "3001"
	}
	raw := getenv("ZONNEBLOEM_FHIR_BASE_URL")
	if raw == "" {
		return config{}, fmt.Errorf("ZONNEBLOEM_FHIR_BASE_URL is not set: point it at De Zonnebloem's FHIR store, " +
			"for example http://hapi-fhir:7050/fhir/sunflower-patients")
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return config{}, fmt.Errorf("ZONNEBLOEM_FHIR_BASE_URL must be an absolute http(s) URL, got %q", raw)
	}
	cfg.fhirBaseURL = u
	return cfg, nil
}
