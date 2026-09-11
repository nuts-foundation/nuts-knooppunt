package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/nuts-foundation/nuts-knooppunt/lib/coding"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

// mcsdQueryTenant is the HAPI tenant holding the local replica of the LRZa
// directory. The Knooppunt syncs it (POST /mcsd/update) but exposes no query API
// of its own, so the sandbox reads the replica directly, which is what the
// Addressing spec has a Query Client do: matching happens on the local replica,
// never against the LRZa itself.
const mcsdQueryTenant = "knpt-mcsd-query"

// mcsdCallTimeout bounds the directory lookup. It sits on the retrieval path,
// where a hanging directory would otherwise stall the whole chain behind it.
const mcsdCallTimeout = 10 * time.Second

// sourceAddress is one data holder as the directory describes it: who they are,
// and where their data can be reached.
type sourceAddress struct {
	URA     string
	Name    string
	Address string
}

// mcsdResolveFunc builds the addressing step: a URA in, a reachable FHIR address
// out.
//
// Endpoint selection is narrower than the Addressing spec requires. It honours
// status and period, but does not match on connectionType and payloadType,
// because the seeded Endpoints carry neither in the form the spec's value sets
// define: connectionType uses a system that is not in NL GF Connection Types,
// and payloadType is absent although the profile makes it 1..*. Matching on them
// would therefore reject every endpoint in this demo. Recorded in
// sandbox/app/README.md as a known deviation rather than worked around silently.
func mcsdResolveFunc(hapiBaseURL *url.URL) func(context.Context, string) (sourceAddress, error) {
	directory := hapiBaseURL.JoinPath(mcsdQueryTenant, "Organization")
	client := &http.Client{Timeout: mcsdCallTimeout}

	return func(ctx context.Context, ura string) (sourceAddress, error) {
		query := *directory
		query.RawQuery = url.Values{
			"identifier": {coding.URANamingSystem + "|" + ura},
			// One round trip rather than two: the Addressing spec's own endpoint
			// discovery use case resolves an organization and its endpoints in a
			// single search.
			"_include": {"Organization:endpoint"},
		}.Encode()

		ctx, cancel := context.WithTimeout(ctx, mcsdCallTimeout)
		defer cancel()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, query.String(), nil)
		if err != nil {
			return sourceAddress{}, fmt.Errorf("build mCSD query: %w", err)
		}
		res, err := client.Do(req)
		if err != nil {
			return sourceAddress{}, fmt.Errorf("query the mCSD directory: %w", err)
		}
		defer res.Body.Close()
		if res.StatusCode != http.StatusOK {
			return sourceAddress{}, fmt.Errorf("the mCSD directory returned %s", res.Status)
		}

		var bundle fhir.Bundle
		if err := json.NewDecoder(res.Body).Decode(&bundle); err != nil {
			return sourceAddress{}, fmt.Errorf("parse the mCSD directory response: %w", err)
		}
		return selectSource(bundle, ura)
	}
}

// selectSource picks the organization and the endpoint to address it on.
func selectSource(bundle fhir.Bundle, ura string) (sourceAddress, error) {
	organization, endpoints := splitDirectoryBundle(bundle)
	if organization == nil {
		return sourceAddress{}, fmt.Errorf("the mCSD directory holds no organization with URA %s", ura)
	}

	// Only the endpoints this organization points at. _include is scoped to the
	// search, not to one resource, so a bundle can carry endpoints belonging to
	// something else entirely.
	for _, reference := range organization.Endpoint {
		if reference.Reference == nil {
			continue
		}
		endpoint, ok := endpoints[*reference.Reference]
		if !ok || !endpointUsable(endpoint) {
			continue
		}
		return sourceAddress{URA: ura, Name: organizationName(*organization), Address: endpoint.Address}, nil
	}
	return sourceAddress{}, fmt.Errorf("organization %s publishes no usable endpoint", ura)
}

// splitDirectoryBundle separates the matched Organization from the included
// Endpoints, keyed by the reference an Organization would use to point at one.
func splitDirectoryBundle(bundle fhir.Bundle) (*fhir.Organization, map[string]fhir.Endpoint) {
	var organization *fhir.Organization
	endpoints := map[string]fhir.Endpoint{}

	for _, entry := range bundle.Entry {
		var peek struct {
			ResourceType string `json:"resourceType"`
		}
		if err := json.Unmarshal(entry.Resource, &peek); err != nil {
			continue
		}
		switch peek.ResourceType {
		case "Organization":
			if organization != nil {
				continue
			}
			var candidate fhir.Organization
			if err := json.Unmarshal(entry.Resource, &candidate); err == nil {
				organization = &candidate
			}
		case "Endpoint":
			var endpoint fhir.Endpoint
			if err := json.Unmarshal(entry.Resource, &endpoint); err == nil && endpoint.Id != nil {
				endpoints["Endpoint/"+*endpoint.Id] = endpoint
			}
		}
	}
	return organization, endpoints
}

// endpointUsable applies the two selection rules the Addressing spec states as a
// SHALL and the seed can actually satisfy: the endpoint is active, and its
// period, when present, includes now.
func endpointUsable(endpoint fhir.Endpoint) bool {
	if endpoint.Status != fhir.EndpointStatusActive || endpoint.Address == "" {
		return false
	}
	if endpoint.Period == nil {
		return true
	}
	now := time.Now()
	if start, ok := parseFHIRInstant(endpoint.Period.Start); ok && now.Before(start) {
		return false
	}
	if end, ok := parseFHIRInstant(endpoint.Period.End); ok && now.After(end) {
		return false
	}
	return true
}

// parseFHIRInstant reads a period bound. FHIR dateTime admits shorter forms than
// RFC 3339 ("2025", "2025-01"), and a bound this cannot read is reported absent
// rather than assumed expired: refusing an endpoint over an unparsed date would
// fail the retrieval for a registration that is very likely still valid.
func parseFHIRInstant(value *string) (time.Time, bool) {
	if value == nil {
		return time.Time{}, false
	}
	parsed, err := time.Parse(time.RFC3339, *value)
	if err != nil {
		return time.Time{}, false
	}
	return parsed, true
}

func organizationName(organization fhir.Organization) string {
	if organization.Name == nil {
		return ""
	}
	return *organization.Name
}
