package lrza

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	fhirclient "github.com/SanteonNL/go-fhir-client"
	"github.com/nuts-foundation/nuts-knooppunt/lib/coding"
	"github.com/nuts-foundation/nuts-knooppunt/lib/fhirapi"
	libfhir "github.com/nuts-foundation/nuts-knooppunt/lib/fhirutil"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

// Tenant isolation. The LRZA authorizes the Knooppunt's certificate, which serves every tenant, so it
// can't tell tenants apart: the Knooppunt has to make sure each tenant only writes its own resources.
//
// An Organization is tenant-owned when it carries the tenant's URA, or when its partOf chain leads to
// one that does (a department of the care provider). Every other resource type is tenant-owned when
// the Organization responsible for it is: Location.managingOrganization,
// HealthcareService.providedBy and Endpoint.managingOrganization.

// maxPartOfDepth caps how far up an Organization's partOf chain is followed, so a long or cyclic
// chain can't make one request read the LRZA without end.
const maxPartOfDepth = 10

// errUnresolvable is returned when a reference can't be followed: it points outside the LRZA, to a
// resource that doesn't exist, or to an entry missing from the transaction.
var errUnresolvable = errors.New("reference can't be resolved")

// ownershipChecker checks, for one request, which resources belong to the tenant.
type ownershipChecker struct {
	// client reads the LRZA; a caching client, since the same Organization is often followed more
	// than once in a request.
	client fhirclient.Client
	// baseURL is the LRZA's FHIR base URL, to recognize absolute references to it.
	baseURL   string
	tenantURA string
	// bundle holds the resources of a transaction by urn: fullUrl and, for updates, by relative
	// reference, so references between entries resolve to the version being written.
	bundle map[string]libfhir.Resource
}

// owningOrganizationElement names the element that refers to the Organization responsible for a
// resource. It is empty for an Organization, which is responsible for itself.
func owningOrganizationElement(resourceType string) string {
	switch resourceType {
	case "Location", "Endpoint":
		return "managingOrganization"
	case "HealthcareService":
		return "providedBy"
	default:
		return ""
	}
}

// requireOwned returns a 403 with the given message unless res belongs to the tenant.
func (o *ownershipChecker) requireOwned(ctx context.Context, res libfhir.Resource, message string) error {
	reason, err := o.ownership(ctx, res)
	if err != nil {
		return err
	}
	if reason != "" {
		return forbiddenError(message + ": " + reason)
	}
	return nil
}

// ownership checks whether res belongs to the tenant. It returns why not, or "" when it does. An
// error is only returned when the LRZA couldn't be asked.
func (o *ownershipChecker) ownership(ctx context.Context, res libfhir.Resource) (string, error) {
	if res.ResourceType() == "Organization" {
		return o.organizationOwnership(ctx, res, 0)
	}
	element := owningOrganizationElement(res.ResourceType())
	ref, _ := res[element].(map[string]any)
	if ref == nil {
		return element + " is missing", nil
	}
	organization, err := o.resolve(ctx, "Organization", ref)
	if errors.Is(err, errUnresolvable) {
		return fmt.Sprintf("%s (%s) does not refer to an Organization in the LRZA", element, describeReference(ref)), nil
	}
	if err != nil {
		return "", err
	}
	reason, err := o.organizationOwnership(ctx, organization, 0)
	if reason != "" {
		reason = fmt.Sprintf("%s (%s): %s", element, describeReference(ref), reason)
	}
	return reason, err
}

// organizationOwnership checks whether an Organization belongs to the tenant: by its URA, or else by
// its partOf chain.
func (o *ownershipChecker) organizationOwnership(ctx context.Context, organization libfhir.Resource, depth int) (string, error) {
	uras := uraValues(organization)
	if len(uras) > 0 {
		for _, ura := range uras {
			if ura != o.tenantURA {
				return fmt.Sprintf("Organization has URA %s, not the tenant's", ura), nil
			}
		}
		return "", nil
	}
	partOf, _ := organization["partOf"].(map[string]any)
	if partOf == nil {
		return "Organization has neither the tenant's URA nor a partOf leading to it", nil
	}
	if depth >= maxPartOfDepth {
		return fmt.Sprintf("Organization's partOf chain is longer than %d", maxPartOfDepth), nil
	}
	parent, err := o.resolve(ctx, "Organization", partOf)
	if errors.Is(err, errUnresolvable) {
		return fmt.Sprintf("Organization.partOf (%s) does not refer to an Organization in the LRZA", describeReference(partOf)), nil
	}
	if err != nil {
		return "", err
	}
	return o.organizationOwnership(ctx, parent, depth+1)
}

// checkNewVersion checks that the version about to be written belongs to the tenant, and that it
// only links Endpoints the tenant manages: otherwise a tenant could claim another tenant's Endpoint.
func (o *ownershipChecker) checkNewVersion(ctx context.Context, res libfhir.Resource) error {
	if err := o.requireOwned(ctx, res, res.ResourceType()+" does not belong to the tenant"); err != nil {
		return err
	}
	endpoints, _ := res["endpoint"].([]any)
	for i, item := range endpoints {
		ref, _ := item.(map[string]any)
		if ref == nil {
			continue
		}
		endpoint, err := o.resolve(ctx, "Endpoint", ref)
		if errors.Is(err, errUnresolvable) {
			return forbiddenError(fmt.Sprintf("%s.endpoint[%d] (%s) does not refer to an Endpoint in the LRZA", res.ResourceType(), i, describeReference(ref)))
		}
		if err != nil {
			return err
		}
		if err := o.requireOwned(ctx, endpoint, fmt.Sprintf("%s.endpoint[%d] (%s) refers to an Endpoint the tenant does not manage", res.ResourceType(), i, describeReference(ref))); err != nil {
			return err
		}
	}
	return nil
}

// resolve follows a reference to a resource of the expected type: to an entry of the transaction
// being processed, or else to the LRZA. A logical reference by URA identifier resolves to an
// Organization carrying just that identifier, which is all the ownership check needs.
func (o *ownershipChecker) resolve(ctx context.Context, expectedType string, ref map[string]any) (libfhir.Resource, error) {
	literal, _ := ref["reference"].(string)
	if literal == "" {
		identifier, _ := ref["identifier"].(map[string]any)
		if expectedType == "Organization" && identifier != nil && identifier["system"] == coding.URAOIDNamingSystem {
			return libfhir.Resource{"resourceType": "Organization", "identifier": []any{identifier}}, nil
		}
		return nil, errUnresolvable
	}
	if res, ok := o.bundle[literal]; ok {
		if res.ResourceType() != expectedType {
			return nil, errUnresolvable
		}
		return res, nil
	}
	if strings.HasPrefix(literal, "urn:") || strings.HasPrefix(literal, "#") {
		// Not an entry of this transaction, or a contained resource: neither is in the LRZA.
		return nil, errUnresolvable
	}
	relative, ok := o.relativeReference(literal)
	if !ok {
		return nil, errUnresolvable
	}
	resourceType, id, ok := libfhir.TypeAndIDFromReference(relative)
	if !ok || resourceType != expectedType {
		return nil, errUnresolvable
	}
	if res, ok := o.bundle[resourceType+"/"+id]; ok {
		return res, nil
	}
	return o.read(ctx, resourceType+"/"+id)
}

// read reads a resource from the LRZA, returning errUnresolvable if it doesn't exist.
func (o *ownershipChecker) read(ctx context.Context, reference string) (libfhir.Resource, error) {
	var data []byte
	var headers fhirclient.Headers
	var status int
	err := o.client.ReadWithContext(ctx, reference, &data, fhirclient.ResponseHeaders(&headers), fhirclient.ResponseStatusCode(&status))
	if status == http.StatusNotFound || status == http.StatusGone {
		return nil, errUnresolvable
	}
	if err != nil {
		return nil, upstreamError("read "+reference+" from", err, status, headers.Header)
	}
	res, err := libfhir.ParseResource(data)
	if err != nil {
		return nil, &fhirapi.Error{Message: "LRZA returned an invalid resource", Cause: err, IssueType: fhir.IssueTypeException}
	}
	return res, nil
}

// relativeReference turns a reference into one relative to the LRZA's base URL. An absolute
// reference to another server can't be resolved, so ok is false.
func (o *ownershipChecker) relativeReference(reference string) (string, bool) {
	parsed, err := url.Parse(reference)
	if err != nil {
		return "", false
	}
	if !parsed.IsAbs() {
		return reference, true
	}
	if o.baseURL != "" && strings.HasPrefix(reference, o.baseURL+"/") {
		return strings.TrimPrefix(reference, o.baseURL+"/"), true
	}
	return "", false
}

// uraValues returns the URAs an Organization carries.
func uraValues(organization libfhir.Resource) []string {
	var result []string
	for _, identifier := range organization.Identifiers() {
		if identifier["system"] == coding.URAOIDNamingSystem {
			value, _ := identifier["value"].(string)
			result = append(result, value)
		}
	}
	return result
}

// describeReference renders a reference for an error message.
func describeReference(ref map[string]any) string {
	if literal, ok := ref["reference"].(string); ok && literal != "" {
		return literal
	}
	if identifier, ok := ref["identifier"].(map[string]any); ok {
		system, _ := identifier["system"].(string)
		value, _ := identifier["value"].(string)
		return system + "|" + value
	}
	return "empty reference"
}
