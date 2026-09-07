package nvi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"testing"

	fhirclient "github.com/SanteonNL/go-fhir-client"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/caramel/to"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

// listFor builds a stored List as the NVI would return it: the custodian in the
// localization extension, which is the only thing distinguishing two
// registrations for the same patient.
func listFor(id, custodianURA string) fhir.List {
	return fhir.List{
		Id: to.Ptr(id),
		Extension: []fhir.Extension{{
			Url: custodianExtensionURL,
			ValueReference: &fhir.Reference{
				Identifier: &fhir.Identifier{
					System: to.Ptr(uraNamingSystem),
					Value:  to.Ptr(custodianURA),
				},
			},
		}},
	}
}

// searchSetOf wraps Lists in the Bundle a search returns.
func searchSetOf(lists ...fhir.List) fhir.Bundle {
	bundle := fhir.Bundle{Type: fhir.BundleTypeSearchset}
	for _, list := range lists {
		raw, err := json.Marshal(list)
		if err != nil {
			panic(err)
		}
		bundle.Entry = append(bundle.Entry, fhir.BundleEntry{Resource: raw})
	}
	return bundle
}

func TestBuildList_IsProfileConformant(t *testing.T) {
	reg := Registration{CustodianURA: "00000010", BSN: "999900006", ClientID: "gf-sandbox-plataan"}

	list := BuildList(reg, CategoryCondition)

	if list.Status != fhir.ListStatusCurrent || list.Mode != fhir.ListModeWorking {
		t.Fatalf("status/mode = %v/%v, want current/working", list.Status, list.Mode)
	}
	if got := *list.Code.Coding[0].System; got != DataCategorySystem {
		t.Errorf("code system = %q, want %q", got, DataCategorySystem)
	}
	if got := *list.Code.Coding[0].Code; got != CategoryCondition {
		t.Errorf("code = %q, want %q", got, CategoryCondition)
	}
	// The profile puts a Required Pattern on List.source.identifier.system.
	if got := *list.Source.Identifier.System; got != OAuthClientIDSystem {
		t.Errorf("source system = %q, want %q", got, OAuthClientIDSystem)
	}
	if got := *list.Source.Identifier.Value; got != "gf-sandbox-plataan" {
		t.Errorf("source value = %q", got)
	}
	if list.EmptyReason == nil || *list.EmptyReason.Coding[0].Code != "withheld" {
		t.Error("emptyReason must be withheld; it is mandatory in the profile")
	}
	if got := *list.EmptyReason.Coding[0].System; got != emptyReasonSystem {
		t.Errorf("emptyReason system = %q", got)
	}
	if got := custodianOf(list); got != "00000010" {
		t.Errorf("custodian = %q", got)
	}
	if got := *list.Subject.Identifier.Value; got != "999900006" {
		t.Errorf("subject = %q", got)
	}
}

// A guard on the vocabulary, not on BuildList: nothing validates List.code at
// runtime, so the defence against reintroducing an aggregate code is that no
// such constant exists to reach for.
func TestCategoryConstants_ContainNoAggregateCode(t *testing.T) {
	for _, category := range []string{CategoryPatient, CategoryCondition, CategoryMedicationRequest, CategoryAllergyIntolerance} {
		if strings.Contains(category, "BGZ") || category == "MEDAFSPRAAK" {
			t.Errorf("category %q is not a member of nl-gf-zorgcontext-vs", category)
		}
	}
}

func TestCategoriesOf_SortedAndDeduplicated(t *testing.T) {
	reg := Registration{CustodianURA: "00000010", BSN: "999900006", ClientID: "c"}
	got := CategoriesOf([]fhir.List{
		BuildList(reg, CategoryMedicationRequest),
		BuildList(reg, CategoryCondition),
		BuildList(reg, CategoryCondition),
	})

	want := []string{CategoryCondition, CategoryMedicationRequest}
	if !slices.Equal(got, want) {
		t.Fatalf("CategoriesOf = %v, want %v", got, want)
	}
}

func TestCategoriesOf_IgnoresOtherCodeSystems(t *testing.T) {
	list := BuildList(Registration{CustodianURA: "00000010", BSN: "1", ClientID: "c"}, CategoryCondition)
	list.Code.Coding[0].System = to.Ptr("http://example.test/other")

	if got := CategoriesOf([]fhir.List{list}); len(got) != 0 {
		t.Fatalf("CategoriesOf = %v, want empty", got)
	}
}

// The client scope has to reach the NVI as a search parameter. Filtering the
// results instead silently deletes nothing: the pseudonymization interceptor
// rewrites List.source.identifier into a Device reference on storage, so a List
// read back has no source.identifier to compare, and every count still looks
// plausible.
func TestDeleteForClientSearchesBySourceIdentifier(t *testing.T) {
	var query url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// The client's DefaultConfig sets UsePostSearch, so a search is
		// POST List/_search with the parameters form-encoded in the body, not in
		// the URL (go-fhir-client client.go, SearchWithContext).
		if err := r.ParseForm(); err != nil {
			t.Errorf("parse search body: %v", err)
		}
		query = r.PostForm
		w.Header().Set("Content-Type", "application/fhir+json")
		_ = json.NewEncoder(w).Encode(searchSetOf())
	}))
	t.Cleanup(srv.Close)
	base, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}

	if err := deleteForClient(context.Background(), fhirclient.New(base, http.DefaultClient, nil),
		"00000010", "999900006", "gf-sandbox-plataan"); err != nil {
		t.Fatalf("deleteForClient: %v", err)
	}

	if got := query.Get("source:identifier"); got != OAuthClientIDSystem+"|gf-sandbox-plataan" {
		t.Errorf("source:identifier = %q, want the client scope in the query", got)
	}
	if got := query.Get("subject:identifier"); got != bsnNamingSystem+"|999900006" {
		t.Errorf("subject:identifier = %q", got)
	}
}

// The custodian is not a search parameter, so it stays a filter, and a record
// belonging to another organization must survive.
func TestDeleteForClientLeavesOtherCustodiansAlone(t *testing.T) {
	var deleted []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			deleted = append(deleted, strings.TrimPrefix(r.URL.Path, "/List/"))
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.Header().Set("Content-Type", "application/fhir+json")
		_ = json.NewEncoder(w).Encode(searchSetOf(
			listFor("plataan-list", "00000010"),
			listFor("zonnebloem-list", "00000020"),
		))
	}))
	t.Cleanup(srv.Close)
	base, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}

	if err := deleteForClient(context.Background(), fhirclient.New(base, http.DefaultClient, nil),
		"00000020", "999900006", "zorgdossier-zonnebloem"); err != nil {
		t.Fatalf("deleteForClient: %v", err)
	}

	if len(deleted) != 1 || deleted[0] != "zonnebloem-list" {
		t.Fatalf("deleted %v, want only zonnebloem-list", deleted)
	}
}

// custodianOf reports a missing or malformed extension as "", and must not
// panic on a nil ValueReference or Identifier along the way. That "" is why
// deleteForClient refuses an empty custodian outright rather than
// comparing against one: see TestDeleteForClientRefusesAnEmptyCustodianOrClient.
func TestCustodianOfHandlesMissingAndMalformedExtensions(t *testing.T) {
	for name, list := range map[string]fhir.List{
		"no extensions":   {},
		"other extension": {Extension: []fhir.Extension{{Url: "http://example.test/other"}}},
		"nil reference":   {Extension: []fhir.Extension{{Url: custodianExtensionURL}}},
		"nil identifier":  {Extension: []fhir.Extension{{Url: custodianExtensionURL, ValueReference: &fhir.Reference{}}}},
	} {
		t.Run(name, func(t *testing.T) {
			if got := custodianOf(list); got != "" {
				t.Fatalf("expected no custodian, got %q", got)
			}
		})
	}

	if got := custodianOf(listFor("x", "00000010")); got != "00000010" {
		t.Fatalf("expected 00000010, got %q", got)
	}
}

// CountLists reports one custodian's registrations, not the subject's. The
// search behind it is by subject, so a patient registered at both De Plataan
// and De Zonnebloem comes back twice; counting the raw result would report two
// registrations for each custodian and make a double-seed look like a duplicate.
func TestCountListsCountsOneCustodian(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/fhir+json")
		_ = json.NewEncoder(w).Encode(searchSetOf(
			listFor("plataan-list", "00000010"),
			listFor("zonnebloem-list", "00000020"),
		))
	}))
	t.Cleanup(srv.Close)

	base, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	for _, custodian := range []string{"00000010", "00000020"} {
		got, err := CountLists(context.Background(), base, custodian, "999900006")
		if err != nil {
			t.Fatalf("CountLists(%s): %v", custodian, err)
		}
		if got != 1 {
			t.Fatalf("custodian %s: expected 1 registration, got %d", custodian, got)
		}
	}
}

// An empty custodian or client must not be usable as a wildcard: an unguarded
// empty value would widen the search instead of narrowing it.
func TestDeleteForClientRefusesAnEmptyCustodianOrClient(t *testing.T) {
	client := fhirclient.New(&url.URL{Scheme: "http", Host: "127.0.0.1:1"}, http.DefaultClient, nil)
	for _, tc := range []struct{ custodian, clientID string }{{"", "c"}, {"00000010", ""}} {
		if err := deleteForClient(context.Background(), client, tc.custodian, "999900006", tc.clientID); err == nil {
			t.Errorf("custodian=%q client=%q: want an error, got nil", tc.custodian, tc.clientID)
		}
	}
}
