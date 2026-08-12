package tenants

import (
	"github.com/nuts-foundation/nuts-knooppunt/lib/coding"
	"github.com/nuts-foundation/nuts-knooppunt/lib/fhirapi"
	"github.com/nuts-foundation/nuts-knooppunt/lib/fhirutil"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

const tenantIDHeader = "X-Tenant-ID"

// IDFromHeaderValue validates and parses the X-Tenant-ID header value, as bound by the
// generated OpenAPI request-parameter code.
func IDFromHeaderValue(tenantID string) (*fhir.Identifier, error) {
	if tenantID == "" {
		return nil, &fhirapi.Error{
			Message:   "missing tenant request header: " + tenantIDHeader,
			IssueType: fhir.IssueTypeValue,
		}
	}

	identifier, err := fhirutil.TokenToIdentifier(tenantID)
	if err != nil {
		return nil, &fhirapi.Error{
			Message:   "invalid tenant ID in request header",
			Cause:     err,
			IssueType: fhir.IssueTypeValue,
		}
	}
	if identifier.System == nil || *identifier.System != coding.URANamingSystem {
		return nil, &fhirapi.Error{
			Message:   "invalid tenant ID in request header, expected system: " + coding.URANamingSystem,
			IssueType: fhir.IssueTypeValue,
		}
	}
	if identifier.Value == nil || *identifier.Value == "" {
		return nil, &fhirapi.Error{
			Message:   "invalid tenant ID in request header, missing value",
			IssueType: fhir.IssueTypeValue,
		}
	}
	return identifier, nil
}
