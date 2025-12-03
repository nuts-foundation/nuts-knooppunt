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
			slog.String("method", response.Request.Method),
			slog.String("url", response.Request.URL.String()),
			slog.Int("status", response.StatusCode),
			slog.String("content", string(responseBody)))
	}
	return &config
}
