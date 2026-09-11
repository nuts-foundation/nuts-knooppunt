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
	"unicode"

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

// fakeNVI serves handler as the NVI and returns its base URL.
func fakeNVI(t *testing.T, handler http.HandlerFunc) *url.URL {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	base, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	return base
}

// answerSearch writes a searchset of the given Lists to every request.
func answerSearch(lists ...fhir.List) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/fhir+json")
		_ = json.NewEncoder(w).Encode(searchSetOf(lists...))
	}
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
// runtime, so the defence against an aggregate code is that no such constant
// exists to reach for. Two rules, because a denylist is only as good as the names
// someone thinks to forbid: every member of nl-gf-zorgcontext-vs is a FHIR
// resource type, so a code with a digit or a hyphen is foreign by construction
// (that catches 55188-7, the aggregate LOINC code this per-category model
// replaced), and the denylist covers aggregates shaped like a resource type.
func TestCategoryConstants_ContainNoAggregateCode(t *testing.T) {
	aggregates := []string{"MEDAFSPRAAK", "55188-7"}
	for _, category := range []string{CategoryPatient, CategoryCondition, CategoryMedicationRequest, CategoryAllergyIntolerance} {
		if strings.Contains(category, "BGZ") || slices.Contains(aggregates, category) {
			t.Errorf("category %q is an aggregate code, not a member of nl-gf-zorgcontext-vs", category)
		}
		if strings.ContainsFunc(category, func(r rune) bool { return !unicode.IsLetter(r) }) {
			t.Errorf("category %q is not a FHIR resource type, so it cannot be a member of nl-gf-zorgcontext-vs", category)
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

// A duplicate is not an unnamed record. CategoriesOf deduplicates, so counting
// unnamed Lists by subtracting its result from the number of Lists reports a
// second List of the same category as one whose category cannot be named.
func TestUnnamedListCount_CountsListsNotMissingCategories(t *testing.T) {
	reg := Registration{CustodianURA: "00000010", BSN: "999900006", ClientID: "c"}
	foreign := BuildList(reg, CategoryCondition)
	foreign.Code.Coding[0].System = to.Ptr("http://example.test/other")
	uncoded := BuildList(reg, CategoryCondition)
	uncoded.Code = nil

	for name, tc := range map[string]struct {
		lists []fhir.List
		want  int
	}{
		"duplicates of one recognized category": {
			[]fhir.List{BuildList(reg, CategoryCondition), BuildList(reg, CategoryCondition)}, 0,
		},
		"a foreign code system": {[]fhir.List{BuildList(reg, CategoryCondition), foreign}, 1},
		"no code at all":        {[]fhir.List{uncoded}, 1},
		"nothing":               {nil, 0},
	} {
		t.Run(name, func(t *testing.T) {
			if got := UnnamedListCount(tc.lists); got != tc.want {
				t.Errorf("UnnamedListCount = %d, want %d", got, tc.want)
			}
		})
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
// results instead silently deletes nothing: the store rewrites source.identifier
// into a Device reference, so a List read back has nothing to compare.
func TestDeleteForClientSearchesBySourceIdentifier(t *testing.T) {
	var query url.Values
	base := fakeNVI(t, func(w http.ResponseWriter, r *http.Request) {
		// The client's DefaultConfig sets UsePostSearch, so a search is
		// POST List/_search with the parameters form-encoded in the body, not in
		// the URL (go-fhir-client client.go, SearchWithContext).
		if err := r.ParseForm(); err != nil {
			t.Errorf("parse search body: %v", err)
		}
		query = r.PostForm
		answerSearch()(w, r)
	})

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
	base := fakeNVI(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			deleted = append(deleted, strings.TrimPrefix(r.URL.Path, "/List/"))
			w.WriteHeader(http.StatusNoContent)
			return
		}
		answerSearch(listFor("plataan-list", "00000010"), listFor("zonnebloem-list", "00000020"))(w, r)
	})

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
// comparing against one: see TestDeleteForClientRefusesAnEmptyScope.
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

// ListsForCustodian returns one custodian's registrations, not the subject's. The
// search behind it is by subject, so a patient registered at both De Plataan and
// De Zonnebloem comes back twice; returning the raw result would report two
// registrations for each custodian and make a double seed look like a duplicate.
func TestListsForCustodianReturnsOneCustodian(t *testing.T) {
	base := fakeNVI(t, answerSearch(listFor("plataan-list", "00000010"), listFor("zonnebloem-list", "00000020")))

	for _, custodian := range []string{"00000010", "00000020"} {
		got, err := ListsForCustodian(context.Background(), base, custodian, "999900006")
		if err != nil {
			t.Fatalf("ListsForCustodian(%s): %v", custodian, err)
		}
		if len(got) != 1 {
			t.Fatalf("custodian %s: expected 1 registration, got %d", custodian, len(got))
		}
	}
}

// An empty custodian, client or BSN must not act as a wildcard: an unguarded
// empty value widens the search instead of narrowing it. The guard has to refuse
// before anything reaches the NVI, and the test has to tell "refused" from "the
// request failed": with an unreachable address, deleting the guard still yields
// an error (connection refused) and the test still passes. So this server counts
// requests, and any request at all is the failure.
func TestDeleteForClientRefusesAnEmptyScope(t *testing.T) {
	for name, tc := range map[string]struct{ custodian, bsn, clientID string }{
		"empty custodian": {"", "999900006", "gf-sandbox-plataan"},
		"empty client":    {"00000010", "999900006", ""},
		"empty bsn":       {"00000010", "", "gf-sandbox-plataan"},
	} {
		t.Run(name, func(t *testing.T) {
			var requests int
			base := fakeNVI(t, func(w http.ResponseWriter, r *http.Request) {
				requests++
				answerSearch()(w, r)
			})

			err := deleteForClient(context.Background(),
				fhirclient.New(base, http.DefaultClient, nil), tc.custodian, tc.bsn, tc.clientID)

			if err == nil {
				t.Error("want an error, got nil")
			}
			if requests != 0 {
				t.Errorf("the guard let %d request(s) reach the NVI; it must refuse before searching", requests)
			}
		})
	}
}
