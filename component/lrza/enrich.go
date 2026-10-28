package lrza

import (
	"context"
	"strings"

	"github.com/nuts-foundation/nuts-knooppunt/lib/coding"
	libfhir "github.com/nuts-foundation/nuts-knooppunt/lib/fhirutil"
	"github.com/nuts-foundation/nuts-knooppunt/lib/profile"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

// Enrichment fills in what the GF Care Service Directory profiles require and an EHR shouldn't have
// to know about. It only adds: an element the client set is never overwritten.

// csdProfiles maps each writable resource type to its CSD profile canonical.
var csdProfiles = map[string]string{
	"Organization":      profile.CSDOrganization,
	"Location":          profile.CSDLocation,
	"HealthcareService": profile.CSDHealthcareService,
	"Endpoint":          profile.CSDEndpoint,
}

// referenceElements are the elements of the writable resource types that refer to an Organization,
// whose URA identifier (for a logical reference) is normalized like the resource's own.
var referenceElements = []string{"partOf", "managingOrganization", "providedBy"}

// enrichOptions is what enrichment needs to know beyond the resource itself.
type enrichOptions struct {
	tenantURA string
	// uraAssignerKVK is the assigner of URA identifiers (see RegistrationConfig.URAAssignerKVK).
	uraAssignerKVK string
	// tenantOrganization returns the reference to the tenant's URA Organization. It is only called
	// when needed (an Endpoint without managingOrganization), as it may have to search the LRZA.
	tenantOrganization func(ctx context.Context) (string, error)
	newUUID            func() string
}

// enrich adds what the profiles require to res. current is the version in the LRZA, or nil for a
// create.
func enrich(ctx context.Context, res, current libfhir.Resource, opts enrichOptions) error {
	addProfile(res)
	normalizeURAIdentifiers(res, opts.uraAssignerKVK)
	switch res.ResourceType() {
	case "Location", "HealthcareService":
		ensureAssignedID(res, current, opts)
	case "Organization":
		// An Organization with a URA is identified by it; only a department without one needs a
		// custodian-assigned identifier of its own.
		if len(uraValues(res)) == 0 {
			ensureAssignedID(res, current, opts)
		}
	case "Endpoint":
		if _, ok := res["managingOrganization"]; !ok {
			reference, err := opts.tenantOrganization(ctx)
			if err != nil {
				return err
			}
			res["managingOrganization"] = map[string]any{"reference": reference, "type": "Organization"}
		}
		ensureFHIRVersion(res)
	}
	return nil
}

// addProfile adds the CSD profile to meta.profile, keeping any profiles the client declared.
func addProfile(res libfhir.Resource) {
	canonical := csdProfiles[res.ResourceType()]
	if canonical == "" {
		return
	}
	meta, _ := res["meta"].(map[string]any)
	if meta == nil {
		meta = map[string]any{}
		res["meta"] = meta
	}
	profiles, _ := meta["profile"].([]any)
	for _, p := range profiles {
		if p == canonical {
			return
		}
	}
	meta["profile"] = append(profiles, canonical)
}

// normalizeURAIdentifiers moves URA identifiers from the system the rest of the Knooppunt (and the
// X-Tenant-ID header) uses to the OID system the profiles require, and gives each URA identifier the
// use and assigner the CustodianAssignedIdentifier profile requires. Logical references by URA are
// moved to the OID system too.
func normalizeURAIdentifiers(res libfhir.Resource, uraAssignerKVK string) {
	for _, identifier := range res.Identifiers() {
		if identifier["system"] == coding.URANamingSystem {
			identifier["system"] = coding.URAOIDNamingSystem
		}
		if identifier["system"] != coding.URAOIDNamingSystem {
			continue
		}
		if _, ok := identifier["use"]; !ok {
			identifier["use"] = fhir.IdentifierUseOfficial.Code()
		}
		if _, ok := identifier["assigner"]; !ok {
			identifier["assigner"] = libfhir.ToJSONMap(coding.Custodian(coding.KVKOIDNamingSystem, uraAssignerKVK))
		}
	}
	for _, element := range referenceElements {
		ref, _ := res[element].(map[string]any)
		identifier, _ := ref["identifier"].(map[string]any)
		if identifier != nil && identifier["system"] == coding.URANamingSystem {
			identifier["system"] = coding.URAOIDNamingSystem
		}
	}
}

// isAssignedID reports whether an identifier is a custodian-assigned identifier other than a URA:
// one naming the party that assigned it.
func isAssignedID(identifier map[string]any) bool {
	if identifier["system"] == coding.URAOIDNamingSystem {
		return false
	}
	assigner, _ := identifier["assigner"].(map[string]any)
	assignerIdentifier, _ := assigner["identifier"].(map[string]any)
	return assignerIdentifier != nil
}

// ensureAssignedID makes sure res has a custodian-assigned identifier (the profiles' AssignedId
// slice), in order of preference:
//
//  1. one the client sent, with its assigner, as is;
//  2. the client's own identifier for the resource, sent without an assigner: the tenant is its
//     custodian, so it is completed with the tenant as assigner. This lets the EHR identify the
//     resource in the LRZA by the id it already has;
//  3. on an update that leaves it out, the current version's, so the resource stays identifiable;
//  4. otherwise a generated urn:uuid, assigned by the tenant.
func ensureAssignedID(res, current libfhir.Resource, opts enrichOptions) {
	identifiers := res.Identifiers()
	for _, identifier := range identifiers {
		if isAssignedID(identifier) {
			return
		}
	}
	for _, identifier := range identifiers {
		system, _ := identifier["system"].(string)
		value, _ := identifier["value"].(string)
		if _, hasAssigner := identifier["assigner"]; hasAssigner || system == "" || value == "" || system == coding.URAOIDNamingSystem {
			continue
		}
		if _, ok := identifier["use"]; !ok {
			identifier["use"] = fhir.IdentifierUseOfficial.Code()
		}
		identifier["assigner"] = libfhir.ToJSONMap(coding.Custodian(coding.URAOIDNamingSystem, opts.tenantURA))
		return
	}
	list, _ := res["identifier"].([]any)
	if current != nil {
		carried := false
		for _, identifier := range current.Identifiers() {
			if isAssignedID(identifier) {
				list = append(list, identifier)
				carried = true
			}
		}
		if carried {
			res["identifier"] = list
			return
		}
	}
	res["identifier"] = append(list, libfhir.ToJSONMap(fhir.Identifier{
		Use:      new(fhir.IdentifierUseOfficial),
		System:   new(coding.RFC3986NamingSystem),
		Value:    new("urn:uuid:" + opts.newUUID()),
		Assigner: coding.Custodian(coding.URAOIDNamingSystem, opts.tenantURA),
	}))
}

// ensureFHIRVersion declares the FHIR version of a FHIR REST Endpoint that doesn't declare one, as the
// IG requires it in payloadMimeType.
func ensureFHIRVersion(res libfhir.Resource) {
	connectionType, _ := res["connectionType"].(map[string]any)
	if connectionType["code"] != "hl7-fhir-rest" {
		return
	}
	mimeTypes, _ := res["payloadMimeType"].([]any)
	for _, mimeType := range mimeTypes {
		if s, ok := mimeType.(string); ok && strings.Contains(s, "fhirVersion=") {
			return
		}
	}
	res["payloadMimeType"] = append(mimeTypes, coding.FHIRR4JSONMimeType)
}
