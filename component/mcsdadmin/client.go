package mcsdadmin

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	fhirClient "github.com/SanteonNL/go-fhir-client"
	"github.com/rs/zerolog/log"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/caramel"
	fhir "github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

func fhirClientConfig() *fhirClient.Config {
	config := fhirClient.DefaultConfig()
	config.DefaultOptions = []fhirClient.Option{
		fhirClient.RequestHeaders(map[string][]string{
			"Cache-Control": {"no-cache"},
		}),
	}
	config.Non2xxStatusHandler = func(response *http.Response, responseBody []byte) {
		log.Debug().Msgf("Non-2xx status code from FHIR server (%s %s, status=%d), content: %s", response.Request.Method, response.Request.URL, response.StatusCode, string(responseBody))
	}
	return &config
}

func FindAll[T any](fhirClient fhirClient.Client) ([]T, error) {
	var prototype T
	resourceType := caramel.ResourceType(prototype)

	var searchResponse fhir.Bundle
	err := fhirClient.Search(resourceType, url.Values{}, &searchResponse, nil)
	if err != nil {
		return nil, fmt.Errorf("search for resource type %s failed: %w", resourceType, err)
	}

	var result []T
	for i, entry := range searchResponse.Entry {
		var item T
		err := json.Unmarshal(entry.Resource, &item)
		if err != nil {
			return nil, fmt.Errorf("unmarshal of entry %d for resource type %s failed: %w", i, resourceType, err)
		}
		result = append(result, item)
	}

	return result, nil
}
