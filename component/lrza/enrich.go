package lrza

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"slices"
	"strings"

	"github.com/nuts-foundation/nuts-knooppunt/lib/coding"
	"github.com/nuts-foundation/nuts-knooppunt/lib/fhirapi"
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

// enrich adds what the profiles require to res. current is the version in the LRZA, or nil for a
// create.
func (w *writeContext) enrich(ctx context.Context, res resource, current resource) error {
	addProfile(res)
	w.normalizeURAIdentifiers(res)
	switch res.resourceType() {
	case "Location", "HealthcareService":
		w.ensureAssignedID(res, current)
	case "Organization":
		// An Organization with a URA is identified by it; only a department without one needs a
		// custodian-assigned identifier of its own.
		if len(uraValues(res)) == 0 {
			w.ensureAssignedID(res, current)
		}
	case "Endpoint":
		if err := w.ensureManagingOrganization(ctx, res); err != nil {
			return err
		}
		ensureFHIRVersion(res)
	}
	return nil
}

// addProfile adds the CSD profile to meta.profile, keeping any profiles the client declared.
func addProfile(res resource) {
	canonical := csdProfiles[res.resourceType()]
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
func (w *writeContext) normalizeURAIdentifiers(res resource) {
	for _, identifier := range identifiers(res) {
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
			identifier["assigner"] = toJSONMap(coding.Custodian(coding.KVKOIDNamingSystem, w.config.URAAssignerKVK))
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
// slice). An update that leaves it out keeps the current version's, so the resource stays
// identifiable by it; otherwise one is generated, assigned by the tenant.
func (w *writeContext) ensureAssignedID(res resource, current resource) {
	for _, identifier := range identifiers(res) {
		if isAssignedID(identifier) {
			return
		}
	}
	list, _ := res["identifier"].([]any)
	if current != nil {
		carried := false
		for _, identifier := range identifiers(current) {
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
	res["identifier"] = append(list, toJSONMap(fhir.Identifier{
		Use:      new(fhir.IdentifierUseOfficial),
		System:   new(coding.RFC3986NamingSystem),
		Value:    new("urn:uuid:" + w.newUUID()),
		Assigner: coding.Custodian(coding.URAOIDNamingSystem, w.tenantURA),
	}))
}

// ensureManagingOrganization defaults an Endpoint's managingOrganization to the tenant's URA
// Organization.
func (w *writeContext) ensureManagingOrganization(ctx context.Context, res resource) error {
	if _, ok := res["managingOrganization"]; ok {
		return nil
	}
	reference, err := w.tenantOrganizationReference(ctx)
	if err != nil {
		return err
	}
	res["managingOrganization"] = map[string]any{"reference": reference, "type": "Organization"}
	return nil
}

// tenantOrganizationReference returns the reference to the tenant's URA Organization: an entry of the
// transaction being processed, or else the Organization in the LRZA.
func (w *writeContext) tenantOrganizationReference(ctx context.Context) (string, error) {
	for _, key := range w.bundleKeys() {
		res := w.bundle[key]
		if res.resourceType() == "Organization" && slices.Contains(uraValues(res), w.tenantURA) {
			return key, nil
		}
	}
	organization, err := w.findTenantOrganization(ctx)
	if err != nil {
		return "", err
	}
	return organization.reference(), nil
}

// bundleKeys returns the keys of the transaction's resources, sorted for a deterministic choice.
func (w *writeContext) bundleKeys() []string {
	keys := make([]string, 0, len(w.bundle))
	for key := range w.bundle {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
}

// findTenantOrganization finds the tenant's Organization in the LRZA by its URA.
func (w *writeContext) findTenantOrganization(ctx context.Context) (resource, error) {
	if w.tenantOrganization != nil {
		return w.tenantOrganization, nil
	}
	var searchSet fhir.Bundle
	query := url.Values{"identifier": []string{coding.URAOIDNamingSystem + "|" + w.tenantURA}}
	if err := w.client.SearchWithContext(ctx, "Organization", query, &searchSet); err != nil {
		return nil, &fhirapi.Error{Message: "Failed to search the tenant's Organization in the LRZA", Cause: err, IssueType: fhir.IssueTypeTransient}
	}
	for _, entry := range searchSet.Entry {
		if entry.Search != nil && entry.Search.Mode != nil && *entry.Search.Mode != fhir.SearchEntryModeMatch {
			continue
		}
		res, err := parseResource(entry.Resource)
		if err != nil || res.resourceType() != "Organization" {
			continue
		}
		w.tenantOrganization = res
		w.cache[res.reference()] = res
		return res, nil
	}
	return nil, unprocessableError(fmt.Sprintf("the tenant's Organization (URA %s) is not registered in the LRZA; register it first", w.tenantURA))
}

// ensureFHIRVersion declares the FHIR version of a FHIR REST Endpoint that doesn't declare one, as the
// IG requires it in payloadMimeType.
func ensureFHIRVersion(res resource) {
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

// toJSONMap converts a typed FHIR element to the generic JSON form resources are processed in.
func toJSONMap(value any) map[string]any {
	data, _ := json.Marshal(value)
	var result map[string]any
	_ = json.Unmarshal(data, &result)
	return result
}
