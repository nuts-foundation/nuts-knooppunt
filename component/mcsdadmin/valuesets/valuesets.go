package valuesets

import (
	"embed"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

// EndpointConnectionTypeCodings contains the codings from https://hl7.org/fhir/R4/valueset-endpoint-connection-type.html
var EndpointConnectionTypeCodings = mustGetValueSet("endpoint-connection-type")

// EndpointPayloadTypeCodings contains the codings from:
// - http://hl7.org/fhir/R4/ValueSet/endpoint-payload-type
// - http://nuts-foundation.github.io/nl-generic-functions-ig/CodeSystem/nl-gf-data-exchange-capabilities
var EndpointPayloadTypeCodings = mustGetValueSet("endpoint-payload-type")

// EndpointStatusCodings contains the codings from https://hl7.org/fhir/R4/valueset-endpoint-status.html
var EndpointStatusCodings = mustGetValueSet("endpoint-status")

// LocationPhysicalTypeCodings contains the codings from  https://terminology.hl7.org/6.3.0/ValueSet-v3-ServiceDeliveryLocationRoleType.html
var LocationPhysicalTypeCodings = mustGetValueSet("location-physical-type")

// LocationStatusCodings contains the codings from https://hl7.org/fhir/R4/valueset-location-status.html
var LocationStatusCodings = mustGetValueSet("location-status")

// LocationTypeCodings contains the codings from https://terminology.hl7.org/6.3.0/ValueSet-v3-ServiceDeliveryLocationRoleType.html
var LocationTypeCodings = mustGetValueSet("location-type")

// OrganizationTypeCodings contains the codings from https://hl7.org/fhir/R4/valueset-organization-type.html
var OrganizationTypeCodings = mustGetValueSet("organization-type")

// PurposeOfUseCodings contains the codings from https://terminology.hl7.org/6.3.0/ValueSet-v3-PurposeOfUse.html
var PurposeOfUseCodings = mustGetValueSet("purpose-of-use")

// ServiceTypeCodings contains the codings from https://hl7.org/fhir/R4/valueset-service-type.html
var ServiceTypeCodings = mustGetValueSet("service-type")

// PractitionerRoleCodings contains the codings from https://terminology.hl7.org/6.5.0/CodeSystem-practitioner-role.html
var PractitionerRoleCodings = mustGetValueSet("practitioner-role")

func mustGetValueSet(name string) []fhir.Coding {
	result := getValueSets()[name]
	if result == nil {
		panic("Value set " + name + " not found")
	}
	return result
}

var codings = make(map[string][]fhir.Coding)

//go:embed *.json
var setsFS embed.FS

func getValueSets() map[string][]fhir.Coding {
	if len(codings) > 0 {
		return codings
	}
	var err error
	codings, err = readValueSets()
	if err != nil {
		panic("Unable to read value sets: " + err.Error())
	}
	return codings
}

func readValueSets() (map[string][]fhir.Coding, error) {
	// Read all JSON files as codings
	dirEntries, err := setsFS.ReadDir(".")
	if err != nil {
		return nil, err
	}
	var result = make(map[string][]fhir.Coding)
	for _, entry := range dirEntries {
		if entry.IsDir() {
			continue
		}
		fileName := entry.Name()
		if !strings.HasSuffix(fileName, ".json") {
			continue
		}
		data, err := setsFS.ReadFile(fileName)
		if err != nil {
			return nil, err
		}
		var currCodings []fhir.Coding
		if err := json.Unmarshal(data, &currCodings); err != nil {
			return nil, fmt.Errorf("invalid JSON in %s: %w", fileName, err)
		}
		setId := strings.TrimSuffix(fileName, ".json")
		result[setId] = currCodings
	}
	return result, nil
}

func CodingFrom(set []fhir.Coding, codeId string) (fhir.Coding, bool) {
	for _, c := range set {
		if c.Code != nil && *c.Code == codeId {
			return c, true
		}
	}
	return fhir.Coding{}, false
}

func CodableFrom(set []fhir.Coding, codeId string) (fhir.CodeableConcept, bool) {
	var out fhir.CodeableConcept
	for _, c := range set {
		if c.Code != nil && *c.Code == codeId {
			out.Coding = []fhir.Coding{c}
			out.Text = c.Display
			return out, true
		}
	}
	return out, false
}

func CodablesFrom(set []fhir.Coding, codeIds []string) ([]fhir.CodeableConcept, bool) {
	outOk := true
	out := make([]fhir.CodeableConcept, 0, len(set))
	for _, codeId := range codeIds {
		codable, ok := CodableFrom(set, codeId)
		if !ok {
			outOk = false
		} else {
			out = append(out, codable)
		}
	}
	return out, outOk
}

func EndpointStatusFrom(code string) (out fhir.EndpointStatus, ok bool) {
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

func LocationStatusFrom(code string) (out fhir.LocationStatus, ok bool) {
	switch code {
	case "active":
		return fhir.LocationStatusActive, true
	case "suspended":
		return fhir.LocationStatusSuspended, true
	case "inactive":
		return fhir.LocationStatusInactive, true
	default:
		return fhir.LocationStatusActive, false
	}
}
