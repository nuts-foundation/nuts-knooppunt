package mcsdadmin

import (
	"bytes"
	"encoding/json"
	"errors"
	"github.com/rs/zerolog/log"
	"io"
	"net/http"
	"strconv"
)

const baseURL = "http://localhost:7050/fhir/DEFAULT/"
const accept = "Accept: application/fhir+json;q=1.0, application/json+fhir;q=0.9"
const contentType = "application/fhir+json; charset=UTF-8"

//
//var client = &http.Client{}

type FhirData struct {
	Id string `json:"id"`
}

func CreateResource(resourceType string, content []byte) (id string, err error) {
	var url = baseURL + resourceType + "?format=json"
	resp, err := http.Post(url, contentType, bytes.NewReader(content))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode != 201 {
		desc := resp.Status + "\n" + string(respBody)
		return "", errors.New(desc)
	}

	var fd FhirData
	err = json.Unmarshal(respBody, &fd)
	if err != nil {
		return "", err
	}

	return fd.Id, nil
}

func findAll(resourceType string) ([]byte, error) {
	var url = baseURL + resourceType
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Add("Accept", accept)
	req.Header.Add("Cache-Control", "no-cache")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != 200 {
		desc := resp.Status + "\n" + string(respBody)
		return nil, errors.New(desc)
	}

	return respBody, nil
}

type HealthcareService struct {
	Id   string
	Name string
}

type ServiceBundle struct {
	ResourceType string
	Id           string
	Total        int
	Entry        []struct {
		FullUrl  string
		Resource HealthcareService
	}
}

func FindAllServices() ([]HealthcareService, error) {
	respBody, err := findAll("HealthcareService")
	if err != nil {
		return nil, err
	}

	var sb ServiceBundle
	err = json.Unmarshal(respBody, &sb)
	if err != nil {
		return nil, err
	}

	services := make([]HealthcareService, len(sb.Entry))
	for i, s := range sb.Entry {
		services[i] = s.Resource
	}

	count := strconv.Itoa(len(services))
	log.Debug().Msg("Found " + count + " resources")
	return services, nil
}

type Organization struct {
	Id     string
	Name   string
	Active bool
}

type OrganizationBundle struct {
	ResourceType string
	Entry        []struct {
		FullUrl  string
		Resource Organization
	}
}

func FindAllOrganizations() ([]Organization, error) {
	respBody, err := findAll("Organization")
	if err != nil {
		return nil, err
	}

	var ob OrganizationBundle
	err = json.Unmarshal(respBody, &ob)
	if err != nil {
		return nil, err
	}

	organizations := make([]Organization, len(ob.Entry))
	for i, o := range ob.Entry {
		organizations[i] = o.Resource
	}

	return organizations, nil
}
