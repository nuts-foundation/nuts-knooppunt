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
			if err := json.Unmarshal(entry.Resource, &endpoint); err != nil {
				continue
			}
			if endpoint.Id != nil {
				endpoints["Endpoint/"+*endpoint.Id] = endpoint
			}
			// A reference may also be absolute, in which case it matches the
			// entry's fullUrl rather than a relative Type/id
			// (https://hl7.org/fhir/R4/references.html#literal).
			if entry.FullUrl != nil && *entry.FullUrl != "" {
				endpoints[*entry.FullUrl] = endpoint
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
	if start, ok := parseFHIRDateTime(endpoint.Period.Start, false); ok && now.Before(start) {
		return false
	}
	if end, ok := parseFHIRDateTime(endpoint.Period.End, true); ok && now.After(end) {
		return false
	}
	return true
}

// fhirDateTimeLayouts are the forms FHIR dateTime admits, longest first
// (https://hl7.org/fhir/R4/datatypes.html#dateTime). A directory commonly writes
// a period bound as a plain date, so accepting only RFC 3339 left an endpoint
// that went out of service years ago looking valid.
var fhirDateTimeLayouts = []struct {
	layout string
	// until advances a bound to the last instant its precision covers. A
	// period.end of "2021" runs to the end of 2021, not to its first instant.
	until func(time.Time) time.Time
}{
	{time.RFC3339, func(t time.Time) time.Time { return t }},
	{"2006-01-02", func(t time.Time) time.Time { return t.AddDate(0, 0, 1).Add(-time.Nanosecond) }},
	{"2006-01", func(t time.Time) time.Time { return t.AddDate(0, 1, 0).Add(-time.Nanosecond) }},
	{"2006", func(t time.Time) time.Time { return t.AddDate(1, 0, 0).Add(-time.Nanosecond) }},
}

// parseFHIRDateTime reads a period bound at whatever precision it was written.
// inclusiveEnd extends a coarse bound to the end of the period it names, so a
// date-only end is honoured through that whole day rather than from midnight.
//
// A bound this cannot read at all is reported absent rather than assumed
// expired: refusing an endpoint over an unparseable date would fail the
// retrieval for a registration that is very likely still valid.
func parseFHIRDateTime(value *string, inclusiveEnd bool) (time.Time, bool) {
	if value == nil {
		return time.Time{}, false
	}
	for _, form := range fhirDateTimeLayouts {
		parsed, err := time.Parse(form.layout, *value)
		if err != nil {
			continue
		}
		if inclusiveEnd {
			return form.until(parsed), true
		}
		return parsed, true
	}
	return time.Time{}, false
}

func organizationName(organization fhir.Organization) string {
	if organization.Name == nil {
		return ""
	}
	return *organization.Name
}
