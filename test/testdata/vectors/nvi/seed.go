package nvi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"

	fhirclient "github.com/SanteonNL/go-fhir-client"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/caramel/to"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

// Naming systems and code systems used for seeded NVI registrations. Kept as
// local constants so the testdata module stays free of the root module's
// lib/coding package (module boundary, see plan Decisions).
const (
	bsnNamingSystem       = "http://fhir.nl/fhir/NamingSystem/bsn"
	uraNamingSystem       = "http://fhir.nl/fhir/NamingSystem/ura"
	custodianExtensionURL = "http://minvws.github.io/generiekefuncties-docs/StructureDefinition/nl-gf-localization-custodian"
	emptyReasonSystem     = "http://terminology.hl7.org/CodeSystem/list-empty-reason"

	// DataCategorySystem is the CodeSystem behind nl-gf-zorgcontext-vs, the
	// required binding for List.code. Despite that value set's name, its codes
	// are data categories at FHIR resource granularity.
	DataCategorySystem = "http://minvws.github.io/generiekefuncties-docs/CodeSystem/nl-gf-data-categories-cs"

	// OAuthClientIDSystem is a Required Pattern on List.source.identifier.system
	// in the localization List profile: the source is the OAuth client id of the
	// installation that registered the record, not a Device identifier.
	OAuthClientIDSystem = "http://minvws.github.io/generiekefuncties-docs/NamingSystem/oauth-client-id"
)

// Data categories used by this demo, all members of nl-gf-zorgcontext-vs.
//
// There is deliberately no BGZ code. The value set lists data categories, not
// care contexts, so a patient summary is the set of categories it contains. An
// earlier IG used one aggregate LOINC code and the published IG replaced it.
const (
	CategoryPatient            = "Patient"
	CategoryCondition          = "Condition"
	CategoryMedicationRequest  = "MedicationRequest"
	CategoryAllergyIntolerance = "AllergyIntolerance"
)

// Registration says which data categories one custodian holds for one patient,
// registered by one installation. It produces one List per category, because the
// IG registers "the presence of data concerning a specific patient and data
// category".
//
// The BSN is passed in the clear: the Knooppunt's /nvi endpoint pseudonymizes it
// before it reaches the NVI store.
type Registration struct {
	CustodianURA string
	BSN          string
	ClientID     string   // OAuth client id of the registering installation
	Categories   []string // from the constants above
}

// BuildList returns the FHIR List for one category of a Registration.
func BuildList(reg Registration, category string) fhir.List {
	return fhir.List{
		Status: fhir.ListStatusCurrent,
		Mode:   fhir.ListModeWorking,
		Extension: []fhir.Extension{{
			Url: custodianExtensionURL,
			ValueReference: &fhir.Reference{Identifier: &fhir.Identifier{
				System: to.Ptr(uraNamingSystem), Value: to.Ptr(reg.CustodianURA),
			}},
		}},
		Subject: &fhir.Reference{Identifier: &fhir.Identifier{
			System: to.Ptr(bsnNamingSystem), Value: to.Ptr(reg.BSN),
		}},
		Source: &fhir.Reference{Identifier: &fhir.Identifier{
			System: to.Ptr(OAuthClientIDSystem), Value: to.Ptr(reg.ClientID),
		}},
		Code: &fhir.CodeableConcept{Coding: []fhir.Coding{{
			System: to.Ptr(DataCategorySystem), Code: to.Ptr(category),
		}}},
		// The List points at the existence of data and never contains it, so it
		// is empty by design and the profile requires saying why.
		EmptyReason: &fhir.CodeableConcept{Coding: []fhir.Coding{{
			System: to.Ptr(emptyReasonSystem), Code: to.Ptr("withheld"),
		}}},
	}
}

// tenantHeader returns the X-Tenant-ID request header for a custodian URA, so
// the Knooppunt scopes pseudonymization to that tenant.
func tenantHeader(custodianURA string) fhirclient.PreRequestOption {
	return fhirclient.RequestHeaders(http.Header{
		"X-Tenant-ID": {uraNamingSystem + "|" + custodianURA},
	})
}

// Register publishes a Registration: it removes the records this installation
// previously registered for this subject and custodian, then creates one List
// per category.
//
// The delete is scoped to this client id, not to the whole custodian: deleting
// every List under the custodian would erase records another installation of
// the same organization published, which was tolerable for a seed with one
// writer and is not now that the sandbox calls this live.
//
// Delete-then-create is what the GF Localization spec prescribes for correcting
// a record, and the NVI exposes no PUT, so there is no upsert. The sequence is
// not atomic: a transaction Bundle's conditional operations would be, but the
// Knooppunt does not pseudonymize entry.request.url and the fake NVI rejects the
// ":identifier" modifier on a conditional delete. Repeat registration converges
// when serialized, which the sandbox arranges per process; two processes can
// still duplicate.
func Register(ctx context.Context, nviBaseURL *url.URL, reg Registration) error {
	client := fhirclient.New(nviBaseURL, http.DefaultClient, nil)

	if err := deleteForClient(ctx, client, reg.CustodianURA, reg.BSN, reg.ClientID); err != nil {
		return err
	}
	for _, category := range reg.Categories {
		var result fhir.List
		err := client.CreateWithContext(ctx, BuildList(reg, category), &result,
			fhirclient.AtPath("List"), tenantHeader(reg.CustodianURA))
		if err != nil {
			return fmt.Errorf("create NVI List (custodian=%s, category=%s): %w",
				reg.CustodianURA, category, err)
		}
	}
	return nil
}

// DeleteForClient removes the Lists one installation registered for a subject
// under a custodian.
func DeleteForClient(ctx context.Context, nviBaseURL *url.URL, custodianURA, bsn, clientID string) error {
	return deleteForClient(ctx, fhirclient.New(nviBaseURL, http.DefaultClient, nil), custodianURA, bsn, clientID)
}

func deleteForClient(ctx context.Context, client fhirclient.Client, custodianURA, bsn, clientID string) error {
	// All three narrow the delete, and an empty value for any of them widens it
	// instead. custodianOf reports a missing extension as "", so an empty
	// custodian matches records that are not ours; an empty client id does the
	// same through the source scope; and an empty BSN makes the subject
	// parameter "<bsn-system>|", which FHIR R4 token search reads as "any value
	// in this system", so a delete meant for one patient would take every List
	// this custodian and client hold.
	if custodianURA == "" || clientID == "" || bsn == "" {
		return fmt.Errorf("refusing to delete NVI Lists without all three of custodian, client and BSN (custodian=%q, client=%q, bsn ending %s)",
			custodianURA, clientID, LastFourOfBSN(bsn))
	}

	// The client scope is a search parameter, not a filter on the results. The
	// NVI's pseudonymization interceptor rewrites source.identifier into a Device
	// reference on storage, precisely so that a source:identifier search can be
	// answered, so a List read back has no source.identifier to compare: a
	// client-side filter matches nothing and deletes nothing while every count
	// still looks plausible.
	var searchSet fhir.Bundle
	err := client.SearchWithContext(ctx, "List", url.Values{
		"subject:identifier": {bsnNamingSystem + "|" + bsn},
		"source:identifier":  {OAuthClientIDSystem + "|" + clientID},
	}, &searchSet, tenantHeader(custodianURA))
	if err != nil {
		return fmt.Errorf("search NVI Lists to delete (custodian=%s): %w", custodianURA, err)
	}

	for _, entry := range searchSet.Entry {
		var existing fhir.List
		if err := json.Unmarshal(entry.Resource, &existing); err != nil {
			return fmt.Errorf("parse NVI List to delete (custodian=%s): %w", custodianURA, err)
		}
		// The custodian stays a client-side filter: it lives in an extension the
		// NVI does not index, so there is no search parameter for it.
		if existing.Id == nil || custodianOf(existing) != custodianURA {
			continue
		}
		if err := client.DeleteWithContext(ctx, "List/"+*existing.Id, tenantHeader(custodianURA)); err != nil {
			return fmt.Errorf("delete NVI List %s (custodian=%s): %w", *existing.Id, custodianURA, err)
		}
	}
	return nil
}

// ListsForCustodian returns a subject's Lists under one custodian. The search is
// by subject only: the tenant header scopes pseudonymization, not the result set,
// so the custodian is applied afterwards by FilterByCustodian.
func ListsForCustodian(ctx context.Context, nviBaseURL *url.URL, custodianURA, bsn string) ([]fhir.List, error) {
	return listsForCustodian(ctx, fhirclient.New(nviBaseURL, http.DefaultClient, nil), custodianURA, bsn)
}

func listsForCustodian(ctx context.Context, client fhirclient.Client, custodianURA, bsn string) ([]fhir.List, error) {
	var searchSet fhir.Bundle
	err := client.SearchWithContext(ctx, "List", url.Values{
		"subject:identifier": {bsnNamingSystem + "|" + bsn},
	}, &searchSet, tenantHeader(custodianURA))
	if err != nil {
		return nil, fmt.Errorf("search NVI Lists (custodian=%s): %w", custodianURA, err)
	}

	lists := make([]fhir.List, 0, len(searchSet.Entry))
	for _, entry := range searchSet.Entry {
		var list fhir.List
		if err := json.Unmarshal(entry.Resource, &list); err != nil {
			return nil, fmt.Errorf("parse NVI List (custodian=%s): %w", custodianURA, err)
		}
		lists = append(lists, list)
	}
	return FilterByCustodian(lists, custodianURA), nil
}

// FilterByCustodian keeps the Lists whose localization extension names
// custodianURA. The NVI does not index that extension, so this is how a subject's
// Lists are scoped to one custodian, here and in a test that reads the store
// directly.
func FilterByCustodian(lists []fhir.List, custodianURA string) []fhir.List {
	var mine []fhir.List
	for _, list := range lists {
		if custodianOf(list) == custodianURA {
			mine = append(mine, list)
		}
	}
	return mine
}

// CategoriesOf returns the sorted, deduplicated data categories a set of Lists
// carries. Codings from other systems are ignored.
func CategoriesOf(lists []fhir.List) []string {
	seen := map[string]bool{}
	for _, list := range lists {
		if category, ok := categoryOf(list); ok {
			seen[category] = true
		}
	}
	categories := make([]string, 0, len(seen))
	for category := range seen {
		categories = append(categories, category)
	}
	sort.Strings(categories)
	return categories
}

// UnnamedListCount counts the Lists carrying no data category this build
// recognizes. Not derivable by subtracting CategoriesOf from the number of
// Lists: that set is deduplicated, so the subtraction reports a second List of
// the same category as one whose category cannot be named.
func UnnamedListCount(lists []fhir.List) int {
	unnamed := 0
	for _, list := range lists {
		if _, ok := categoryOf(list); !ok {
			unnamed++
		}
	}
	return unnamed
}

// categoryOf returns the List's data category, if it carries one from the
// vocabulary this build knows.
func categoryOf(list fhir.List) (string, bool) {
	if list.Code == nil {
		return "", false
	}
	for _, coding := range list.Code.Coding {
		if coding.System != nil && *coding.System == DataCategorySystem && coding.Code != nil {
			return *coding.Code, true
		}
	}
	return "", false
}

// LastFourOfBSN renders just enough of a BSN to correlate an error or log line
// with a record without recording a national identifier.
func LastFourOfBSN(bsn string) string {
	if len(bsn) <= 4 {
		return "****"
	}
	return "****" + bsn[len(bsn)-4:]
}

// custodianOf returns the custodian URA carried by a List's localization
// extension, or "" when the List has none.
func custodianOf(list fhir.List) string {
	for _, ext := range list.Extension {
		if ext.Url != custodianExtensionURL {
			continue
		}
		if ext.ValueReference != nil && ext.ValueReference.Identifier != nil && ext.ValueReference.Identifier.Value != nil {
			return *ext.ValueReference.Identifier.Value
		}
	}
	return ""
}
