package mcsdadmin

import (
	"encoding/json"
	"net/http"
	"net/url"

	fhirClient "github.com/SanteonNL/go-fhir-client"
	fhir "github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

// TODO: Make this configurable
var baseURL = url.URL{
	Scheme: "HTTP",
	Host:   "localhost:7050",
	Path:   "/fhir/DEFAULT",
}

const accept = "Accept: application/fhir+json;q=1.0, application/json+fhir;q=0.9"
const contentType = "application/fhir+json; charset=UTF-8"

var httpClient = &http.Client{}
var client = fhirClient.New(&baseURL, httpClient, nil)

type FhirData struct {
	Id string `json:"id"`
}

func CreateHealthcareService(service fhir.HealthcareService) (out fhir.HealthcareService, err error) {
	err = client.Create(service, out)
	return out, err
}

func CreateOrganisation(organisation fhir.Organization) (out fhir.HealthcareService, err error) {
	err = client.Create(organisation, out)
	return out, err
}

func CreateEndpoint(service fhir.Endpoint) (out fhir.Endpoint, err error) {
	err = client.Create(service, out)
	return out, err
}

func findAll(resourceType string) (fhir.Bundle, error) {
	var result fhir.Bundle
	err := client.Search(resourceType, url.Values{}, &result, nil)

	// I don't think it is possible to set custom headers in the fhir client
	// We need to set the no-cache header to read our own writes and make the app responsive
	// TODO: Make it possible in the fhir client to pass headers
	//
	//req.Header.Add("Accept", accept)
	//req.Header.Add("Cache-Control", "no-cache")

	if err != nil {
		return fhir.Bundle{}, err
	}

	return result, nil
}

func FindAllServices() ([]fhir.HealthcareService, error) {
	bundle, err := findAll("HealthcareService")
	if err != nil {
		return nil, err
	}

	var hb []fhir.HealthcareService
	for _, entry := range bundle.Entry {
		var h fhir.HealthcareService
		err := json.Unmarshal(entry.Resource, &h)
		if err != nil {
			return hb, err
		}

		hb = append(hb, h)
	}

	return hb, nil
}

func FindAllOrganizations() ([]fhir.Organization, error) {
	bundle, err := findAll("Organization")
	if err != nil {
		return nil, err
	}

	var ob []fhir.Organization
	for _, entry := range bundle.Entry {
		var o fhir.Organization
		err := json.Unmarshal(entry.Resource, &o)
		if err != nil {
			return ob, err
		}

		ob = append(ob, o)
	}

	return ob, nil
}

func FindAllEndpoints() ([]fhir.Endpoint, error) {
	bundle, err := findAll("Endpoint")
	if err != nil {
		return nil, err
	}

	var es []fhir.Endpoint
	for _, entry := range bundle.Entry {
		var e fhir.Endpoint
		err := json.Unmarshal(entry.Resource, &e)
		if err != nil {
			return es, err
		}

		es = append(es, e)
	}

	return es, nil
}
