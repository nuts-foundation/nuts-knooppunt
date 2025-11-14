package tenants

import (
	"net/http"

	"github.com/nuts-foundation/nuts-knooppunt/lib/coding"
	"github.com/nuts-foundation/nuts-knooppunt/lib/fhirapi"
	"github.com/nuts-foundation/nuts-knooppunt/lib/fhirutil"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

const tenantIDHeader = "X-Tenant-ID"

func IDFromRequest(httpRequest *http.Request) (*fhir.Identifier, error) {
	tenantID := httpRequest.Header.Get(tenantIDHeader)
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
	return identifier, nil
}
