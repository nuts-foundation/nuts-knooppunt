package fhirutil

import (
	"log/slog"
	"net/http"

	"github.com/SanteonNL/go-fhir-client"
)

func ClientConfig() *fhirclient.Config {
	config := fhirclient.DefaultConfig()
	config.DefaultOptions = []fhirclient.Option{
		fhirclient.RequestHeaders(map[string][]string{
			"Cache-Control": {"no-cache"},
		}),
	}
	config.Non2xxStatusHandler = func(response *http.Response, responseBody []byte) {
		slog.DebugContext(response.Request.Context(), "Non-2xx status code from FHIR server",
			"method", response.Request.Method,
			"url", response.Request.URL,
			"status", response.StatusCode,
			"content", string(responseBody))
	}
	return &config
}
