package fhirutil

import (
	"log/slog"
	"net/http"

	"github.com/SanteonNL/go-fhir-client"
	"github.com/nuts-foundation/nuts-knooppunt/lib/logging"
)

func ClientConfig() *fhirclient.Config {
	config := fhirclient.DefaultConfig()
	config.DefaultOptions = []fhirclient.Option{
		fhirclient.RequestHeaders(map[string][]string{
			"Cache-Control": {"no-cache"},
		}),
	}
	// The response body and the request query are not logged: both may carry patient data (search
	// values, resource contents echoed in an OperationOutcome).
	config.Non2xxStatusHandler = func(response *http.Response, _ []byte) {
		slog.DebugContext(response.Request.Context(), "Non-2xx status code from FHIR server",
			slog.String("method", response.Request.Method),
			slog.String("url", logging.RedactURL(response.Request.URL)),
			slog.Int("status", response.StatusCode))
	}
	return &config
}
