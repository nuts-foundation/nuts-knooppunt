package nvi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
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

// A patient gets one registration per custodian, and the NVI does not index the
// custodian: it lives in an extension, and the tenant header scopes
// pseudonymization rather than the result set. So this search returns both
// registrations however many organizations registered them, and deleting all of
// them made the seed destroy its own work. Registering De Zonnebloem's
// localization deleted De Plataan's, leaving the patient findable from one side
// where pool.NVILists promises either.
func TestDeleteListsForSubjectLeavesOtherCustodiansAlone(t *testing.T) {
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
	client := fhirclient.New(base, http.DefaultClient, nil)

	if err := deleteListsForSubject(context.Background(), client, "00000020", "999900006"); err != nil {
		t.Fatalf("deleteListsForSubject: %v", err)
	}

	if len(deleted) != 1 || deleted[0] != "zonnebloem-list" {
		t.Fatalf("expected only the caller's own registration to be deleted, got %v", deleted)
	}
}

// custodianOf reports a missing or malformed extension as "", and must not
// panic on a nil ValueReference or Identifier along the way. That "" is why
// deleteListsForSubject refuses an empty custodian outright rather than
// comparing against one: see TestDeleteListsForSubjectRefusesAnEmptyCustodian.
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

// An empty custodian must not be usable as a wildcard: custodianOf reports a
// missing extension as "" too, so an unguarded empty URA would delete every
// List that carries no custodian.
func TestDeleteListsForSubjectRefusesAnEmptyCustodian(t *testing.T) {
	var deleted []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			deleted = append(deleted, r.URL.Path)
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.Header().Set("Content-Type", "application/fhir+json")
		_ = json.NewEncoder(w).Encode(searchSetOf(fhir.List{Id: to.Ptr("no-custodian")}))
	}))
	t.Cleanup(srv.Close)

	base, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	err = deleteListsForSubject(context.Background(), fhirclient.New(base, http.DefaultClient, nil), "", "999900006")
	if err == nil {
		t.Fatal("expected an empty custodian to be refused")
	}
	if len(deleted) != 0 {
		t.Fatalf("nothing may be deleted for an empty custodian, deleted %v", deleted)
	}
}
