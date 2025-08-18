package mcsdadmin

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"
)

const baseURL = "http://localhost:7050/fhir/DEFAULT/"
const accept = "Accept: application/fhir+json;q=1.0, application/json+fhir;q=0.9"
const contentType = "application/fhir+json; charset=UTF-8"

var client = &http.Client{}

type FhirData struct {
	Id string
}

func resourceCreate(resourceType string, content []byte) (id string, err error) {
	var url = baseURL + resourceType + "?format=json"
	resp, err := client.Post(url, contentType, bytes.NewReader(content))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	log.Print("Status " + resp.Status)
	if err != nil {
		return "", err
	}

	var fd FhirData
	err = json.Unmarshal(respBody, &fd)
	if err != nil {
		return "", err
	}

	return fd.Id, nil
}
