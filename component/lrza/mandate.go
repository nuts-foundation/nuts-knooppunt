package lrza

import (
	"context"
	"errors"
	"fmt"
	"net/url"

	fhirclient "github.com/SanteonNL/go-fhir-client"
	"github.com/nuts-foundation/nuts-knooppunt/lib/fhirapi"
	libfhir "github.com/nuts-foundation/nuts-knooppunt/lib/fhirutil"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

// The mandate. The LRZA only accepts writes by an IT vendor that the care provider authorized, through
// an OrganizationAffiliation the care provider registered itself: organization is the care provider,
// participatingOrganization the vendor, and code the kind of mandate. The LRZA checks this on every
// write; when configured (lrza.registration.mandate), the Knooppunt checks it first, so a tenant that
// hasn't granted the mandate gets a 403 saying so, instead of whatever the LRZA answers.

// mandate is the parsed MandateConfig.
type mandate struct {
	codeSystem, code     string
	vendorSystem, vendor string
}

func parseMandate(config MandateConfig) (*mandate, error) {
	if config.Code == "" {
		return nil, nil
	}
	code, err := libfhir.TokenToIdentifier(config.Code)
	if err != nil || code.System == nil || code.Value == nil {
		return nil, fmt.Errorf("invalid lrza.registration.mandate.code %q, expected system|code", config.Code)
	}
	vendor, err := libfhir.TokenToIdentifier(config.Vendor)
	if err != nil || vendor.System == nil || vendor.Value == nil {
		return nil, fmt.Errorf("invalid lrza.registration.mandate.vendor %q, expected system|value (required with mandate.code)", config.Vendor)
	}
	return &mandate{codeSystem: *code.System, code: *code.Value, vendorSystem: *vendor.System, vendor: *vendor.Value}, nil
}

// check returns a 403 unless the tenant granted the mandate. organization is the tenant's Organization
// in the LRZA, nil if it isn't registered. resolve follows a reference to the vendor's Organization.
func (m *mandate) check(ctx context.Context, client fhirclient.Client, resolve resolveFunc, tenantURA string, organization libfhir.Resource) error {
	if organization == nil {
		// The tenant's Organization isn't registered yet, so no mandate can exist either. Its very first
		// registration has to be done by the care provider itself.
		return forbiddenError(fmt.Sprintf("the tenant (URA %s) has not granted the mandate: its Organization is not registered in the LRZA", tenantURA))
	}
	var searchSet fhir.Bundle
	query := url.Values{"primary-organization": []string{organization.Reference()}}
	if err := client.SearchWithContext(ctx, "OrganizationAffiliation", query, &searchSet); err != nil {
		return &fhirapi.Error{Message: "Failed to search the tenant's mandates in the LRZA", Cause: err, IssueType: fhir.IssueTypeTransient}
	}
	for _, entry := range searchSet.Entry {
		affiliation, err := libfhir.ParseResource(entry.Resource)
		if err != nil || affiliation.ResourceType() != "OrganizationAffiliation" {
			continue
		}
		granted, err := m.grants(ctx, resolve, affiliation)
		if err != nil {
			return err
		}
		if granted {
			return nil
		}
	}
	return forbiddenError(fmt.Sprintf("the tenant (URA %s) has not granted the mandate (%s|%s) to %s|%s; the care provider registers it as an OrganizationAffiliation in the LRZA",
		tenantURA, m.codeSystem, m.code, m.vendorSystem, m.vendor))
}

// resolveFunc follows a reference to a resource of the expected type (see ownershipChecker.resolve).
type resolveFunc func(ctx context.Context, expectedType string, ref map[string]any) (libfhir.Resource, error)

// grants reports whether an affiliation is the mandate: active, with the mandate's code, and with the
// vendor as participatingOrganization.
func (m *mandate) grants(ctx context.Context, resolve resolveFunc, affiliation libfhir.Resource) (bool, error) {
	if active, ok := affiliation["active"].(bool); ok && !active {
		return false, nil
	}
	hasCode := false
	codes, _ := affiliation["code"].([]any)
	for _, item := range codes {
		concept, _ := item.(map[string]any)
		codings, _ := concept["coding"].([]any)
		for _, c := range codings {
			coding, _ := c.(map[string]any)
			if coding["system"] == m.codeSystem && coding["code"] == m.code {
				hasCode = true
			}
		}
	}
	if !hasCode {
		return false, nil
	}
	participant, _ := affiliation["participatingOrganization"].(map[string]any)
	if participant == nil {
		return false, nil
	}
	if identifier, ok := participant["identifier"].(map[string]any); ok {
		return identifier["system"] == m.vendorSystem && identifier["value"] == m.vendor, nil
	}
	vendor, err := resolve(ctx, "Organization", participant)
	if errors.Is(err, errUnresolvable) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	for _, identifier := range vendor.Identifiers() {
		if identifier["system"] == m.vendorSystem && identifier["value"] == m.vendor {
			return true, nil
		}
	}
	return false, nil
}
