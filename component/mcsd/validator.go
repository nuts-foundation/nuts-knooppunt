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
		if err := validateOrganizationResource(parentOrg, parentOrganizationMap); err != nil {
			return fmt.Errorf("parent organization failed to validate: %w", err)
		}
	}
	return nil
}

// ValidateUpdate validates a FHIR resource create/update from a mCSD Administration Directory,
// according to the rules specified by https://nuts-foundation.github.io/nl-generic-functions-ig/care-services.html#update-client
func ValidateUpdate(ctx context.Context, rules ValidationRules, resourceJSON []byte, parentOrganizationMap map[*fhir.Organization][]*fhir.Organization, allHealthcareServices []fhir.HealthcareService) error {
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
		return unmarshalAndVisitOrganizationResource(resourceJSON, parentOrganizationMap)
	case "Location":
		return unmarshalAndVisitResource[fhir.Location](ctx, resourceJSON, parentOrganizationMap, allHealthcareServices, validateLocationResource)
	case "PractitionerRole":
		return unmarshalAndVisitResource[fhir.PractitionerRole](ctx, resourceJSON, parentOrganizationMap, allHealthcareServices, validatePractitionerRoleResource)
	case "HealthcareService":
		return unmarshalAndVisitResource[fhir.HealthcareService](ctx, resourceJSON, parentOrganizationMap, allHealthcareServices, validateHealthcareServiceResource)
	case "Endpoint":
		return unmarshalAndVisitResource[fhir.Endpoint](ctx, resourceJSON, parentOrganizationMap, allHealthcareServices, validateEndpointResource)
	}
	return nil
}

func unmarshalAndVisitResource[ResType any](ctx context.Context, resourceJSON []byte, parentOrganizationMap map[*fhir.Organization][]*fhir.Organization, allHealthcareServices []fhir.HealthcareService, visitor func(ctx context.Context, resource *ResType, parentOrganizationMap map[*fhir.Organization][]*fhir.Organization, allHealthcareServices []fhir.HealthcareService) error) error {
	resource := new(ResType)
	if err := json.Unmarshal(resourceJSON, resource); err != nil {
		return fmt.Errorf("failed to unmarshal resource JSON: %w", err)
	}
	return visitor(ctx, resource, parentOrganizationMap, allHealthcareServices)
}

func unmarshalAndVisitOrganizationResource(resourceJSON []byte, parentOrganizationMap map[*fhir.Organization][]*fhir.Organization) error {
	resource := new(fhir.Organization)
	if err := json.Unmarshal(resourceJSON, resource); err != nil {
		return fmt.Errorf("failed to unmarshal resource JSON: %w", err)
	}
	return validateOrganizationResource(resource, parentOrganizationMap)
}

func validateOrganizationResource(resource *fhir.Organization, parentOrganizationMap map[*fhir.Organization][]*fhir.Organization) error {
	if resource == nil {
		return nil // No validation needed if resource is nil
	}

	uraIdentifiers := fhirutil.FilterIdentifiersBySystem(resource.Identifier, coding.URANamingSystem)
	if len(uraIdentifiers) > 1 {
		// Only the authoritative organization should have a URA identifier, and only one
		slog.Warn("Organization has multiple URA identifiers", slog.String("system", coding.URANamingSystem), slog.Int("count", len(uraIdentifiers)))
		return fmt.Errorf("organization can't have multiple identifiers with system %s", coding.URANamingSystem)
	}

	// Collect all URA identifiers from parent organizations
	parentURAIdentifiers := make(map[string]bool)
	for parentOrg := range parentOrganizationMap {
		if parentOrg != nil {
			parentURAs := fhirutil.FilterIdentifiersBySystem(parentOrg.Identifier, coding.URANamingSystem)
			for _, ura := range parentURAs {
				if ura.Value != nil {
					parentURAIdentifiers[*ura.Value] = true
				}
			}
		}
	}

	if len(uraIdentifiers) > 0 {
		// If the resource has a URA identifier, it must match one from the parent organizations
		resourceURA := uraIdentifiers[0]
		if resourceURA.Value == nil {
			return fmt.Errorf("organization has a URA identifier with no value")
		}
		if !parentURAIdentifiers[*resourceURA.Value] {
			slog.Warn("Organization URA identifier does not match any parent organization URA", slog.String("ura", *resourceURA.Value))
			return fmt.Errorf("organization's URA identifier must match one of the authoritative parent organizations")
		}
	}

	if len(uraIdentifiers) == 0 {
		if resource.PartOf == nil {
			slog.Warn("Organization missing URA identifier and partOf reference", slog.String("system", coding.URANamingSystem))
			return fmt.Errorf("organization must have an identifier with system %s or refer to another organization through 'partOf'", coding.URANamingSystem)
		}

		// Validate that partOf references an authoritative organization (one with a URA identifier)
		if err := validatePartOfReferencesAuthoritativeOrg(resource.PartOf, parentOrganizationMap); err != nil {
			return err
		}
	}

	return nil
}

// validatePartOfReferencesAuthoritativeOrg validates that the partOf reference eventually points to an organization with a URA identifier
// by recursively following the partOf chain up the organization tree
func validatePartOfReferencesAuthoritativeOrg(partOfRef *fhir.Reference, parentOrganizationMap map[*fhir.Organization][]*fhir.Organization) error {
	if partOfRef == nil {
		return nil
	}

	visited := make(map[string]bool)
	return validatePartOfChain(partOfRef, parentOrganizationMap, visited)
}

// validatePartOfChain recursively validates the partOf chain until it finds an organization with a URA identifier
func validatePartOfChain(partOfRef *fhir.Reference, parentOrganizationMap map[*fhir.Organization][]*fhir.Organization, visited map[string]bool) error {
	if partOfRef == nil {
		return fmt.Errorf("reached end of partOf chain without finding an authoritative organization")
	}

	refID := extractReferenceID(partOfRef.Reference)
	if refID == "" {
		return fmt.Errorf("partOf reference does not contain a valid ID")
	}

	// Check for circular references
	if visited[refID] {
		return fmt.Errorf("circular reference detected in partOf chain at organization %s", refID)
	}
	visited[refID] = true

	// Search for the referenced organization in the parent organization map
	for parentOrg := range parentOrganizationMap {
		if parentOrg != nil && parentOrg.Id != nil && *parentOrg.Id == refID {
			// Check if this organization has a URA identifier (is authoritative)
			uraIdentifiers := fhirutil.FilterIdentifiersBySystem(parentOrg.Identifier, coding.URANamingSystem)
			if len(uraIdentifiers) > 0 {
				return nil // Found an authoritative organization
			}
			// No URA identifier, follow the partOf chain
			return validatePartOfChain(parentOrg.PartOf, parentOrganizationMap, visited)
		}

		// Also check in the allOrganizations list
		allOrganizations := parentOrganizationMap[parentOrg]
		for _, org := range allOrganizations {
			if org != nil && org.Id != nil && *org.Id == refID {
				// Check if this organization has a URA identifier (is authoritative)
				uraIdentifiers := fhirutil.FilterIdentifiersBySystem(org.Identifier, coding.URANamingSystem)
				if len(uraIdentifiers) > 0 {
					return nil // Found an authoritative organization
				}
				// No URA identifier, follow the partOf chain
				return validatePartOfChain(org.PartOf, parentOrganizationMap, visited)
			}
		}
	}

	// Referenced organization not found in parent organization map
	slog.Warn("Organization partOf reference not found in parent organization map", slog.String("refID", refID))
	return fmt.Errorf("organization's partOf reference could not be validated (organization %s not found within authoritative organizations)", refID)
}

func validateHealthcareServiceResource(ctx context.Context, resource *fhir.HealthcareService, parentOrganizationMap map[*fhir.Organization][]*fhir.Organization, allHealthcareServices []fhir.HealthcareService) error {
	if resource.ProvidedBy == nil {
		slog.WarnContext(ctx, "Healthcare service missing providedBy reference")
		return fmt.Errorf("healthcare service must have a 'providedBy' referencing an Organization")
	}

	return assertReferencePointsToValidOrganization(resource.ProvidedBy, parentOrganizationMap, "healthcareService.providedBy")
}

func validatePractitionerRoleResource(ctx context.Context, resource *fhir.PractitionerRole, parentOrganizationMap map[*fhir.Organization][]*fhir.Organization, allHealthcareServices []fhir.HealthcareService) error {
	if resource.Organization == nil {
		slog.WarnContext(ctx, "Practitioner role missing organization reference")
		return fmt.Errorf("practitioner role must have an organization reference")
	}

	return assertReferencePointsToValidOrganization(resource.Organization, parentOrganizationMap, "practitionerRole.organization")
}

func validateEndpointResource(ctx context.Context, resource *fhir.Endpoint, parentOrganizationMap map[*fhir.Organization][]*fhir.Organization, allHealthcareServices []fhir.HealthcareService) error {
	if resource.Id == nil {
		return fmt.Errorf("endpoint must have an ID")
	}

	// Check that at least one of the organizations or healthcare services has this endpoint in their endpoint references
	return assertOrganizationOrHealthcareServiceHasEndpointReference(resource.Id, parentOrganizationMap, allHealthcareServices)
}

func validateLocationResource(ctx context.Context, resource *fhir.Location, parentOrganizationMap map[*fhir.Organization][]*fhir.Organization, allHealthcareServices []fhir.HealthcareService) error {
	if resource.ManagingOrganization == nil {
		slog.WarnContext(ctx, "Location missing managingOrganization reference")
		return fmt.Errorf("location must have a 'managingOrganization' referencing an Organization")
	}

	return assertReferencePointsToValidOrganization(resource.ManagingOrganization, parentOrganizationMap, "location.managingOrganization")
}

// assertOrganizationOrHealthcareServiceHasEndpointReference validates that at least one of the organizations (parent or in allOrganizations)
// or healthcare services has this endpoint ID in their endpoint references.
func assertOrganizationOrHealthcareServiceHasEndpointReference(endpointID *string, parentOrganizationMap map[*fhir.Organization][]*fhir.Organization, allHealthcareServices []fhir.HealthcareService) error {
	if endpointID == nil {
		return fmt.Errorf("endpoint ID is nil")
	}

	// Check organizations first
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

	// Check healthcare services
	for _, healthcareService := range allHealthcareServices {
		if healthcareServiceHasEndpointReference(&healthcareService, endpointID) {
			// If the healthcare service references this endpoint, validate that the healthcare service itself is valid
			if err := validateHealthcareServiceResource(context.Background(), &healthcareService, parentOrganizationMap, allHealthcareServices); err == nil {
				// Found a valid healthcare service that references this endpoint
				return nil
			}
			// Otherwise, continue checking other healthcare services or organizations
		}
	}

	// No organization or valid healthcare service has this endpoint
	slog.Warn("Endpoint not referenced by any organization or valid healthcare service", slog.String("endpointID", *endpointID))
	return fmt.Errorf("endpoint must be referenced in at least one organization's or valid healthcare service's endpoint field (endpoint ID: %s)", *endpointID)
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

// healthcareServiceHasEndpointReference checks if a healthcareService has the given endpoint ID in its endpoint references.
func healthcareServiceHasEndpointReference(healthcareService *fhir.HealthcareService, endpointID *string) bool {
	if healthcareService == nil || endpointID == nil {
		return false
	}

	for _, endpointRef := range healthcareService.Endpoint {
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
