package mcsd

import (
	"errors"

	fhirclient "github.com/SanteonNL/go-fhir-client"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/caramel"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

// ResourceValidator checks whether the given resource is valid according to some rules.
type ResourceValidator interface {
	// Validate returns whether the given resource is valid, an OperationOutcome with details,
	// and an error if something went wrong during validation.
	Validate(resource any) (bool, fhir.OperationOutcome, error)
}

var _ ResourceValidator = &FHIRResourceValidator{}

// FHIRResourceValidator validates FHIR resources using a FHIR server's $validate operation.
type FHIRResourceValidator struct {
	Client fhirclient.Client
	// Profiles is a map of resource type (e.g. "Endpoint") to profile URL,
	// to validate against (e.g. "http://nuts-foundation.github.io/nl-generic-functions-ig/StructureDefinition/nl-gf-endpoint")
	Profiles map[string]string
}

func (f FHIRResourceValidator) Validate(resource any) (bool, fhir.OperationOutcome, error) {
	resourceType := caramel.ResourceType(resource)
	if resourceType == "" {
		return false, fhir.OperationOutcome{}, errors.New("validate: unknown resource type")
	}
	
}
