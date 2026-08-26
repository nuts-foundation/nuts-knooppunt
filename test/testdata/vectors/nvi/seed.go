package nvi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	fhirclient "github.com/SanteonNL/go-fhir-client"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/caramel/to"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

// Naming systems and the zorgcontext code used for seeded NVI registrations.
// Kept as local constants so the testdata module stays free of the root
// module's lib/coding package (module boundary, see plan Decisions).
const (
	bsnNamingSystem       = "http://fhir.nl/fhir/NamingSystem/bsn"
	uraNamingSystem       = "http://fhir.nl/fhir/NamingSystem/ura"
	custodianExtensionURL = "http://minvws.github.io/generiekefuncties-docs/StructureDefinition/nl-gf-localization-custodian"
	zorgcontextSystem     = "http://minvws.github.io/generiekefuncties-docs/CodeSystem/nl-gf-zorgcontext-cs"

	// ZorgcontextCode is the placeholder zorgcontext for seeded BGZ data. DESIGN
	// §10 wants a real BGZ code; MEDAFSPRAAK is reused until the CodeSystem gains
	// one (see plan Decisions).
	ZorgcontextCode        = "MEDAFSPRAAK"
	zorgcontextCodeDisplay = "Medicatieafspraak"
)

// SeedList is a source-agnostic description of one NVI localization record: it
// says "custodian <CustodianURA> holds data for patient <BSN>". The pool
// produces one SeedList per (patient, custodian); SeedNVI turns each into a
// FHIR List and registers it through the Knooppunt.
//
// The BSN is passed in the clear: the Knooppunt's /nvi endpoint pseudonymizes
// it (tenant-scoped) before it reaches the NVI store.
type SeedList struct {
	CustodianURA string // the organization that holds the data (e.g. 00000020)
	BSN          string // patient BSN, in the clear
	SourceSystem string // Device identifier system of the registering source
	SourceValue  string // Device identifier value of the registering source
}

// List builds the FHIR List resource for this registration, matching the shape
// the NVI registration flow expects (custodian extension, BSN Subject, Device
// Source, MEDAFSPRAAK zorgcontext Code). See test/e2e/nvi/registration_test.go.
func (s SeedList) List() fhir.List {
	return fhir.List{
		Status: fhir.ListStatusCurrent,
		Mode:   fhir.ListModeWorking,
		Extension: []fhir.Extension{
			{
				Url: custodianExtensionURL,
				ValueReference: &fhir.Reference{
					Identifier: &fhir.Identifier{
						System: to.Ptr(uraNamingSystem),
						Value:  to.Ptr(s.CustodianURA),
					},
				},
			},
		},
		Subject: &fhir.Reference{
			Identifier: &fhir.Identifier{
				System: to.Ptr(bsnNamingSystem),
				Value:  to.Ptr(s.BSN),
			},
		},
		Source: &fhir.Reference{
			Type: to.Ptr("Device"),
			Identifier: &fhir.Identifier{
				System: to.Ptr(s.SourceSystem),
				Value:  to.Ptr(s.SourceValue),
			},
		},
		Code: &fhir.CodeableConcept{
			Coding: []fhir.Coding{
				{
					System:  to.Ptr(zorgcontextSystem),
					Code:    to.Ptr(ZorgcontextCode),
					Display: to.Ptr(zorgcontextCodeDisplay),
				},
			},
		},
	}
}

// tenantHeader returns the X-Tenant-ID request header for a custodian URA, so
// the Knooppunt scopes pseudonymization to that tenant.
func tenantHeader(custodianURA string) fhirclient.PreRequestOption {
	return fhirclient.RequestHeaders(http.Header{
		"X-Tenant-ID": {uraNamingSystem + "|" + custodianURA},
	})
}

// RegisterList performs a delete-then-create for a single SeedList against the
// Knooppunt's internal /nvi endpoint, making the registration idempotent: any
// existing List(s) for this subject under this custodian are removed first,
// then exactly one fresh List is created. Every call carries the custodian's
// tenant header so the BSN is pseudonymized consistently.
//
// The delete is done as search-then-delete-by-id rather than a single
// conditional delete: the fake-NVI store does not accept the ":identifier"
// modifier on a conditional delete (it is only wired for search), so we resolve
// the ids first and delete each one.
//
// Note: delete-then-create is not atomic. This is acceptable for a single
// writer seed (boot/reset only); concurrent live registration could interleave.
func RegisterList(ctx context.Context, nviBaseURL *url.URL, list SeedList) error {
	client := fhirclient.New(nviBaseURL, http.DefaultClient, nil)

	if err := deleteListsForSubject(ctx, client, list.CustodianURA, list.BSN); err != nil {
		return err
	}

	// Create the fresh List.
	var result fhir.List
	err := client.CreateWithContext(ctx, list.List(), &result,
		fhirclient.AtPath("List"),
		tenantHeader(list.CustodianURA))
	if err != nil {
		return fmt.Errorf("create NVI List (custodian=%s): %w", list.CustodianURA, err)
	}
	return nil
}

// deleteListsForSubject removes every NVI List for a subject under a custodian,
// via search (by BSN) then delete-by-id. The Knooppunt tokenizes the BSN on both
// the search and the delete.
func deleteListsForSubject(ctx context.Context, client fhirclient.Client, custodianURA, bsn string) error {
	var searchSet fhir.Bundle
	err := client.SearchWithContext(ctx, "List", url.Values{
		"subject:identifier": {bsnNamingSystem + "|" + bsn},
	}, &searchSet, tenantHeader(custodianURA))
	if err != nil {
		return fmt.Errorf("search existing NVI Lists (custodian=%s): %w", custodianURA, err)
	}
	for _, entry := range searchSet.Entry {
		var existing fhir.List
		if err := json.Unmarshal(entry.Resource, &existing); err != nil {
			return fmt.Errorf("parse existing NVI List (custodian=%s): %w", custodianURA, err)
		}
		if existing.Id == nil {
			continue
		}
		if err := client.DeleteWithContext(ctx, "List/"+*existing.Id, tenantHeader(custodianURA)); err != nil {
			return fmt.Errorf("delete existing NVI List %s (custodian=%s): %w", *existing.Id, custodianURA, err)
		}
	}
	return nil
}

// CountLists returns the number of NVI Lists registered for a subject under a
// custodian, as seen through the Knooppunt (pseudonymized). Used by tests to
// assert idempotency.
func CountLists(ctx context.Context, nviBaseURL *url.URL, custodianURA, bsn string) (int, error) {
	client := fhirclient.New(nviBaseURL, http.DefaultClient, nil)
	var searchSet fhir.Bundle
	err := client.SearchWithContext(ctx, "List", url.Values{
		"subject:identifier": {bsnNamingSystem + "|" + bsn},
	}, &searchSet, tenantHeader(custodianURA))
	if err != nil {
		return 0, fmt.Errorf("search NVI Lists (custodian=%s): %w", custodianURA, err)
	}
	return len(searchSet.Entry), nil
}
