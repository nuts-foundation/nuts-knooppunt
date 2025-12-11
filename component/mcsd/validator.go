package mcsd

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"slices"
	"strings"

	"github.com/nuts-foundation/nuts-knooppunt/lib/coding"
	"github.com/nuts-foundation/nuts-knooppunt/lib/fhirutil"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

type ValidationRules struct {
	// AllowedResourceTypes is a list of FHIR resource types that are allowed to be created/updated.
	AllowedResourceTypes []string
}

// ValidateParentOrganizations validates all parent organizations in the map.
func ValidateParentOrganizations(parentOrganizationMap map[*fhir.Organization][]*fhir.Organization) error {
	for parentOrg := range parentOrganizationMap {
		if err := validateOrganizationResource(parentOrg); err != nil {
			return fmt.Errorf("parent organization failed to validate: %w", err)
		}
	}
	return nil
}

// ValidateUpdate validates a FHIR resource create/update from a mCSD Administration Directory,
// according to the rules specified by https://nuts-foundation.github.io/nl-generic-functions-ig/care-services.html#update-client
func ValidateUpdate(ctx context.Context, rules ValidationRules, resourceJSON []byte, parentOrganizationMap map[*fhir.Organization][]*fhir.Organization) error {
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
		return unmarshalAndVisitOrganizationResource(resourceJSON)
	case "Location":
		return unmarshalAndVisitResource[fhir.Location](ctx, resourceJSON, parentOrganizationMap, validateLocationResource)
	case "PractitionerRole":
		return unmarshalAndVisitResource[fhir.PractitionerRole](ctx, resourceJSON, parentOrganizationMap, validatePractitionerRoleResource)
	case "HealthcareService":
		return unmarshalAndVisitResource[fhir.HealthcareService](ctx, resourceJSON, parentOrganizationMap, validateHealthcareServiceResource)
	case "Endpoint":
		return unmarshalAndVisitResource[fhir.Endpoint](ctx, resourceJSON, parentOrganizationMap, validateEndpointResource)
	}
	return nil
}

func unmarshalAndVisitResource[ResType any](ctx context.Context, resourceJSON []byte, parentOrganizationMap map[*fhir.Organization][]*fhir.Organization, visitor func(ctx context.Context, resource *ResType, parentOrganizationMap map[*fhir.Organization][]*fhir.Organization) error) error {
	resource := new(ResType)
	if err := json.Unmarshal(resourceJSON, resource); err != nil {
		return fmt.Errorf("failed to unmarshal resource JSON: %w", err)
	}
	return visitor(ctx, resource, parentOrganizationMap)
}

func unmarshalAndVisitOrganizationResource(resourceJSON []byte) error {
	resource := new(fhir.Organization)
	if err := json.Unmarshal(resourceJSON, resource); err != nil {
		return fmt.Errorf("failed to unmarshal resource JSON: %w", err)
	}
	return validateOrganizationResource(resource)
}

func validateOrganizationResource(resource *fhir.Organization) error {
	if resource == nil {
		return nil // No validation needed if resource is nil
	}

	uraIdentifiers := fhirutil.FilterIdentifiersBySystem(resource.Identifier, coding.URANamingSystem)
	if len(uraIdentifiers) > 1 {
		slog.Warn("Organization has multiple URA identifiers", slog.String("system", coding.URANamingSystem), slog.Int("count", len(uraIdentifiers)))
		return fmt.Errorf("organization can't have multiple identifiers with system %s", coding.URANamingSystem)
	}

	if len(uraIdentifiers) == 0 && resource.PartOf == nil {
		slog.Warn("Organization missing URA identifier and partOf reference", slog.String("system", coding.URANamingSystem))
		return fmt.Errorf("organization must have an identifier with system %s or refer to another organization through 'partOf'", coding.URANamingSystem)
	}

	return nil
}

func validateHealthcareServiceResource(ctx context.Context, resource *fhir.HealthcareService, parentOrganizationMap map[*fhir.Organization][]*fhir.Organization) error {
	if resource.ProvidedBy == nil {
		slog.WarnContext(ctx, "Healthcare service missing providedBy reference")
		return fmt.Errorf("healthcare service must have a 'providedBy' referencing an Organization")
	}

	return assertReferencePointsToValidOrganization(resource.ProvidedBy, parentOrganizationMap, "healthcareService.providedBy")
}

func validatePractitionerRoleResource(ctx context.Context, resource *fhir.PractitionerRole, parentOrganizationMap map[*fhir.Organization][]*fhir.Organization) error {
	if resource.Organization == nil {
		slog.WarnContext(ctx, "Practitioner role missing organization reference")
		return fmt.Errorf("practitioner role must have an organization reference")
	}

	return assertReferencePointsToValidOrganization(resource.Organization, parentOrganizationMap, "practitionerRole.organization")
}

func validateEndpointResource(ctx context.Context, resource *fhir.Endpoint, parentOrganizationMap map[*fhir.Organization][]*fhir.Organization) error {
	if resource.Id == nil {
		return fmt.Errorf("endpoint must have an ID")
	}

	// Check that at least one of the organizations has this endpoint in their endpoint references
	return assertOrganizationHasEndpointReference(resource.Id, parentOrganizationMap)
}

func validateLocationResource(ctx context.Context, resource *fhir.Location, parentOrganizationMap map[*fhir.Organization][]*fhir.Organization) error {
	if resource.ManagingOrganization == nil {
		slog.WarnContext(ctx, "Location missing managingOrganization reference")
		return fmt.Errorf("location must have a 'managingOrganization' referencing an Organization")
	}

	return assertReferencePointsToValidOrganization(resource.ManagingOrganization, parentOrganizationMap, "location.managingOrganization")
}

// assertOrganizationHasEndpointReference validates that at least one of the organizations (parent or in allOrganizations)
// has this endpoint ID in their endpoint references.
func assertOrganizationHasEndpointReference(endpointID *string, parentOrganizationMap map[*fhir.Organization][]*fhir.Organization) error {
	if endpointID == nil {
		return fmt.Errorf("endpoint ID is nil")
	}

	for parentOrganization := range parentOrganizationMap {
		// Check if parent organization has this endpoint
		if parentOrganization != nil && organizationHasEndpointReference(parentOrganization, endpointID) {
			return nil
		}

		allOrganizations := parentOrganizationMap[parentOrganization]

		// Check if any organization in allOrganizations has this endpoint
		for _, org := range allOrganizations {
			if organizationHasEndpointReference(org, endpointID) {
				return nil
			}
		}
	}

	// No organization has this endpoint
	slog.Warn("Endpoint not referenced by any organization", slog.String("endpointID", *endpointID))
	return fmt.Errorf("endpoint must be referenced in at least one organization's endpoint field (endpoint ID: %s)", *endpointID)
}

// organizationHasEndpointReference checks if an organization has the given endpoint ID in its endpoint references.
func organizationHasEndpointReference(org *fhir.Organization, endpointID *string) bool {
	if org == nil || endpointID == nil {
		return false
	}

	for _, endpointRef := range org.Endpoint {
		if endpointRef.Reference == nil {
			continue
		}

		// Extract the ID from the reference
		refID := extractReferenceID(endpointRef.Reference)
		if refID == *endpointID {
			return true
		}
	}

	return false
}

// assertReferencePointsToValidOrganization validates that a reference points to either the parent organization
// or one of the organizations in the allOrganizations list.
func assertReferencePointsToValidOrganization(ref *fhir.Reference, parentOrganizationMap map[*fhir.Organization][]*fhir.Organization, fieldName string) error {
	if ref == nil {
		return fmt.Errorf("%s reference is nil", fieldName)
	}

	// Extract the ID from the reference
	refID := extractReferenceID(ref.Reference)
	if refID == "" {
		return fmt.Errorf("%s reference does not contain a valid ID", fieldName)
	}

	for parentOrganization := range parentOrganizationMap {
		// Check if it references the parent organization
		if parentOrganization != nil && parentOrganization.Id != nil && refID == *parentOrganization.Id {
			return nil
		}

		allOrganizations := parentOrganizationMap[parentOrganization]

		// Check if it references any of the organizations in allOrganizations
		for _, org := range allOrganizations {
			if org.Id != nil && refID == *org.Id {
				return nil
			}
		}

	}

	slog.Warn("Reference does not point to a valid organization", slog.String("field", fieldName), slog.String("referenceID", refID))
	return fmt.Errorf("%s must reference a valid organization (got %s)", fieldName, refID)
}

// extractReferenceID extracts the resource ID from a FHIR reference string.
// For example, "Organization/123" returns "123".
func extractReferenceID(ref *string) string {
	if ref == nil {
		return ""
	}
	parts := strings.Split(*ref, "/")
	if len(parts) < 2 {
		return *ref // Return the whole reference if it doesn't contain a slash
	}
	return parts[len(parts)-1] // Return the last part (the ID)
}
