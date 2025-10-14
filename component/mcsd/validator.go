package mcsd

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"

	"github.com/nuts-foundation/nuts-knooppunt/lib/coding"
	"github.com/nuts-foundation/nuts-knooppunt/lib/fhirutil"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

type ValidationRules struct {
	// AllowedResourceTypes is a list of FHIR resource types that are allowed to be created/updated.
	AllowedResourceTypes []string
}

// ValidateUpdate validates a FHIR resource create/update from a mCSD Administration Directory,
// according to the rules specified by https://nuts-foundation.github.io/nl-generic-functions-ig/care-services.html#update-client
func ValidateUpdate(ctx context.Context, rules ValidationRules, resourceJSON []byte) error {
	resourceAsMap := map[string]any{}
	if err := json.Unmarshal(resourceJSON, &resourceAsMap); err != nil {
		return fmt.Errorf("failed to unmarshal resource JSON: %w", err)
	}
	resourceType, ok := resourceAsMap["resourceType"].(string)
	if !ok {
		return fmt.Errorf("resource JSON does not contain a valid 'resourceType' field")
	}

	// Base validation
	if !slices.Contains(rules.AllowedResourceTypes, resourceType) {
		return fmt.Errorf("resource type %s not allowed", resourceType)
	}

	switch resourceType {
	case "Organization":
		return unmarshalAndVisitResource[fhir.Organization](ctx, resourceJSON, validateOrganizationResource)
	case "Location":
		return unmarshalAndVisitResource[fhir.Location](ctx, resourceJSON, validateLocationResource)
	case "PractitionerRole":
		return unmarshalAndVisitResource[fhir.PractitionerRole](ctx, resourceJSON, validatePractitionerRoleResource)
	case "HealthcareService":
		return unmarshalAndVisitResource[fhir.HealthcareService](ctx, resourceJSON, validateHealthcareServiceResource)
	case "Endpoint":
		return unmarshalAndVisitResource[fhir.Endpoint](ctx, resourceJSON, validateEndpointResource)
	default:
		return fmt.Errorf("resource type %s not allowed (missing validation rules)", resourceType)
	}
}

func unmarshalAndVisitResource[ResType any](ctx context.Context, resourceJSON []byte, visitor func(ctx context.Context, resource *ResType) error) error {
	resource := new(ResType)
	if err := json.Unmarshal(resourceJSON, resource); err != nil {
		return fmt.Errorf("failed to unmarshal resource JSON: %w", err)
	}
	return visitor(ctx, resource)
}

func validateOrganizationResource(ctx context.Context, resource *fhir.Organization) error {
	uraIdentifiers := fhirutil.FilterIdentifiersBySystem(resource.Identifier, coding.URANamingSystem)
	if len(uraIdentifiers) > 1 {
		return fmt.Errorf("organization can't have multiple identifiers with system %s", coding.URANamingSystem)
	}

	if len(uraIdentifiers) == 0 && resource.PartOf == nil {
		return fmt.Errorf("organization must have an identifier with system %s or refer to another organization through 'partOf'", coding.URANamingSystem)
	}

	// TODO: Support validation of organizations referring to a parent organization, without having a URA identifier

	// if len(uraIdentifiers) == 0 && resource.PartOf != nil {
	//	response, err := http.NewRequest(http.MethodGet, *resource.PartOf.Reference, bytes.NewReader())
	//	if err != nil {
	//		return fmt.Errorf("could not follow reference to parent Organization %s", resource.PartOf)
	//	}
	//	response.Body
	// }

	return nil
}

func validateHealthcareServiceResource(ctx context.Context, resource *fhir.HealthcareService) error {
	if resource.ProvidedBy == nil {
		return fmt.Errorf("healthcare service must have a 'providedBy' referencing an Organization")
	}

	return nil
}

func validatePractitionerRoleResource(ctx context.Context, resource *fhir.PractitionerRole) error {
	if resource.Organization == nil {
		return fmt.Errorf("practitioner role must have an organization reference")
	}

	return nil
}

func validateEndpointResource(ctx context.Context, resource *fhir.Endpoint) error {
	if resource.ManagingOrganization == nil {
		return fmt.Errorf("endpoint must have a 'managingOrganization' referencing an Organization")
	}

	return nil
}

func validateLocationResource(ctx context.Context, resource *fhir.Location) error {
	if resource.ManagingOrganization == nil {
		return fmt.Errorf("location must have a 'managingOrganization' referencing an Organization")
	}

	return nil
}
