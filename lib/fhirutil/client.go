package fhirutil

import (
	"net/http"

	"github.com/SanteonNL/go-fhir-client"
	"github.com/rs/zerolog/log"
)

func ClientConfig() *fhirclient.Config {
	config := fhirclient.DefaultConfig()
	config.DefaultOptions = []fhirclient.Option{
		fhirclient.RequestHeaders(map[string][]string{
			"Cache-Control": {"no-cache"},
		}),
	}
	config.Non2xxStatusHandler = func(response *http.Response, responseBody []byte) {
		log.Debug().Msgf("Non-2xx status code from FHIR server (%s %s, status=%d), content: %s", response.Request.Method, response.Request.URL, response.StatusCode, string(responseBody))
	}
	return &config
}
