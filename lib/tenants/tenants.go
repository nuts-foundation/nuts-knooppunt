package tenants

import (
	"github.com/nuts-foundation/nuts-knooppunt/lib/coding"
	"github.com/nuts-foundation/nuts-knooppunt/lib/fhirapi"
	"github.com/nuts-foundation/nuts-knooppunt/lib/fhirutil"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

// ID identifies the local care organization on whose behalf a request is made, as a FHIR
// Identifier restricted to the URA naming system. It's a named type over fhir.Identifier, not an
// alias, so it can implement encoding.TextUnmarshaler: oapi-codegen's generated parameter binder
// (see openapi.yaml's TenantID parameter, mapped to this type via x-go-type) calls UnmarshalText
// automatically when parsing the X-Tenant-ID header, so a malformed value is rejected before it
// ever reaches the operation - the ErrorHandlerFunc wired up in strictAPIServer.RegisterHttpHandlers
// in package cmd turns the failure into the same FHIR OperationOutcome response this validation
// always produced. A missing header is rejected even earlier, by the generated binder itself,
// before UnmarshalText is called at all.
type ID fhir.Identifier

// URA returns the tenant's URA number. Safe to call unconditionally: UnmarshalText guarantees a
// non-nil, non-empty Value.
func (id ID) URA() string {
	return *id.Value
}

func (id *ID) UnmarshalText(text []byte) error {
	identifier, err := fhirutil.TokenToIdentifier(string(text))
	if err != nil {
		return &fhirapi.Error{
			Message:   "invalid tenant ID",
			Cause:     err,
			IssueType: fhir.IssueTypeValue,
		}
	}
	if identifier.System == nil || *identifier.System != coding.URANamingSystem {
		return &fhirapi.Error{
			Message:   "invalid tenant ID, expected system: " + coding.URANamingSystem,
			IssueType: fhir.IssueTypeValue,
		}
	}
	if identifier.Value == nil || *identifier.Value == "" {
		return &fhirapi.Error{
			Message:   "invalid tenant ID, missing value",
			IssueType: fhir.IssueTypeValue,
		}
	}
	*id = ID(*identifier)
	return nil
}
