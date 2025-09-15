//go:generate go run ./codegen/main.go
package valuesets

import (
	"embed"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

var codings = make(map[string][]fhir.Coding)

//go:embed codegen/*.json
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
	dirEntries, err := setsFS.ReadDir("codegen")
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
		data, err := setsFS.ReadFile("codegen/" + fileName)
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

func CodableFrom(set []fhir.Coding, codeId string) (out fhir.CodeableConcept, ok bool) {
	for _, c := range set {
		if c.Code != nil && *c.Code == codeId {
			out.Coding = []fhir.Coding{c}
			out.Text = c.Display
			return out, true
		}
	}
	return out, false
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
