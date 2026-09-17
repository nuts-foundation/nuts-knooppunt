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
// where their data can be reached, and which server authorizes access to it.
type sourceAddress struct {
	URA     string
	Name    string
	Address string

	// AuthorizationServer is the address published on an oauth-nuts Endpoint.
	// Empty when the holder publishes none: that is a true answer from the
	// directory, and the step that needs a token reports it rather than building
	// an address out of the URA and hoping a server answers there.
	AuthorizationServer string
}

// authorizationServerSystem is the GF code system whose codes name an
// authorization server on Endpoint.connectionType. Its oauth-nuts code is what
// this demo's nodes speak.
// https://minvws.github.io/generiekefuncties-docs/en/CodeSystem-nl-gf-authorization-server-cs.html
const authorizationServerSystem = "http://minvws.github.io/generiekefuncties-docs/CodeSystem/nl-gf-authorization-server-cs"

const authorizationServerNuts = "oauth-nuts"

// fhirDataConnectionTypes are the codings this chain accepts as "a FHIR base to
// read a patient summary from". Two entries, because the spec's connection types
// value set draws FHIR REST from the HL7 code system while the endpoints seeded
// in this repo carry a system that is in neither value set. Accepting only the
// seeded one would reject a conformant directory; accepting only the spec's
// would reject this demo's own.
//
// A list rather than "anything that is not an authorization server": a directory
// entry is not limited to the kinds this chain knows, and an organization that
// also publishes an imaging or document service would otherwise have that
// service handed the bearer token and a patient search carrying the BSN.
var fhirDataConnectionTypes = map[string]string{
	"http://fhir.nl/fhir/NamingSystem/endpoint-connection-type":      "fhir",
	"http://terminology.hl7.org/CodeSystem/endpoint-connection-type": "hl7-fhir-rest",
}

// mcsdResolveFunc builds the addressing step: a URA in, a reachable FHIR address
// out.
//
// Endpoint selection is narrower than the Addressing spec requires. It honours
// status and period, and matches connectionType against fhirDataConnectionTypes
// and the authorization-server coding, but it does not match payloadType at all:
// the seeded endpoints carry none, although the profile makes it 1..*, so
// matching on it would reject every data endpoint in this demo. Recorded in
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
	source := sourceAddress{URA: ura, Name: organizationName(*organization)}
	for _, reference := range organization.Endpoint {
		if reference.Reference == nil {
			continue
		}
		endpoint, ok := endpoints[*reference.Reference]
		if !ok || !endpointUsable(endpoint) {
			continue
		}
		// Which of the two an endpoint is has to be read from what it says it
		// is. Taking the first usable one would hand the retrieval whichever the
		// directory happened to list first, and an authorization server is not a
		// FHIR base.
		if isAuthorizationServer(endpoint) {
			if source.AuthorizationServer == "" {
				source.AuthorizationServer = endpoint.Address
			}
			continue
		}
		if source.Address == "" && servesFHIRData(endpoint) {
			source.Address = endpoint.Address
		}
	}
	if source.Address == "" {
		return sourceAddress{}, fmt.Errorf("organization %s publishes no usable endpoint", ura)
	}
	return source, nil
}

// isAuthorizationServer reports whether the endpoint names an authorization
// server rather than a place to read data from.
func isAuthorizationServer(endpoint fhir.Endpoint) bool {
	return endpoint.ConnectionType.System != nil &&
		*endpoint.ConnectionType.System == authorizationServerSystem &&
		endpoint.ConnectionType.Code != nil &&
		*endpoint.ConnectionType.Code == authorizationServerNuts
}

// servesFHIRData reports whether the endpoint is one this chain can read a
// patient summary from.
func servesFHIRData(endpoint fhir.Endpoint) bool {
	if endpoint.ConnectionType.System == nil || endpoint.ConnectionType.Code == nil {
		return false
	}
	code, known := fhirDataConnectionTypes[*endpoint.ConnectionType.System]
	return known && code == *endpoint.ConnectionType.Code
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
