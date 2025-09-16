package profile

import (
	"embed"
	"encoding/json"
	"errors"

	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

//go:embed *.json
var definitionsFS embed.FS

func GetStructureDefinition(profileURL string) (*fhir.StructureDefinition, error) {
	switch profileURL {
	case NLGenericFunctionOrganization:
		return readStructureDefinition("StructureDefinition-nl-gf-organization.json")
	}
	return nil, errors.New("profile definition not found")
}

func readStructureDefinition(fileName string) (*fhir.StructureDefinition, error) {
	data, err := definitionsFS.ReadFile(fileName)
	if err != nil {
		return nil, err
	}
	var sd fhir.StructureDefinition
	if err := json.Unmarshal(data, &sd); err != nil {
		return nil, err
	}
	return &sd, nil
}
