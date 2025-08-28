package valuesets

import (
	"embed"
	"encoding/json"

	"github.com/rs/zerolog/log"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

var codingSystemIndex = map[string]string{
	"endpoint-status":          "http://hl7.org/fhir/endpoint-status",
	"endpoint-payload-type":    "http://terminology.hl7.org/CodeSystem/endpoint-payload-type",
	"endpoint-connection-type": "http://terminology.hl7.org/CodeSystem/endpoint-connection-type",
	"organization-type":        "http://terminology.hl7.org/CodeSystem/organization-type",
	"purpose-of-use":           "http://terminology.hl7.org/CodeSystem/v3-ActReason",
	"service-type":             "http://terminology.hl7.org/CodeSystem/service-type",
}

var codingIndex = make(map[string]map[string]fhir.Coding)

//go:embed *.json
var setsFS embed.FS

func CodingsFrom(setId string) (out []fhir.Coding, err error) {
	filename := setId + ".json"
	bytes, err := setsFS.ReadFile(filename)
	if err != nil {
		log.Warn().Err(err).Msg("Could not load file with values in set")
		return out, err
	}

	var codings []fhir.Coding
	err = json.Unmarshal(bytes, &codings)
	if err != nil {
		log.Warn().Err(err).Msg("Invalid values in file")
		return out, err
	}

	// We add codings to and index here...
	// ... so it's easy to retrieve without parsing the data again
	for _, coding := range codings {
		if coding.Code == nil {
			log.Warn().Msg("Value in set is missing code")
		} else {
			code := *coding.Code
			if codingIndex[setId] == nil {
				codingIndex[setId] = make(map[string]fhir.Coding)
			}
			codingIndex[setId][code] = coding
		}
	}

	return codings, nil
}

func CodingFrom(setId string, codeId string) (fhir.Coding, bool) {
	codeMap, ok := codingIndex[setId]
	if !ok {
		return fhir.Coding{}, false
	}
	code, ok := codeMap[codeId]
	if !ok {
		return fhir.Coding{}, false
	}
	system, ok := codingSystemIndex[setId]
	if !ok {
		return fhir.Coding{}, false
	}

	code.System = &system

	return code, true
}

func CodableFrom(setId string, codeId string) (out fhir.CodeableConcept, ok bool) {
	coding, ok := CodingFrom(setId, codeId)
	if !ok {
		return out, false
	}

	out.Coding = []fhir.Coding{
		coding,
	}

	out.Text = coding.Display
	return out, true
}

func StatusFrom(code string) (out fhir.EndpointStatus, ok bool) {
	switch code {
	case "active":
		return fhir.EndpointStatusActive, true
	case "suspended":
		return fhir.EndpointStatusSuspended, true
	case "error":
		return fhir.EndpointStatusError, true
	case "off":
		return fhir.EndpointStatusOff, true
	case "entered-in-error":
		return fhir.EndpointStatusEnteredInError, true
	default:
		return fhir.EndpointStatusActive, false
	}
}
