package zonnebloem

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"

	fhirclient "github.com/SanteonNL/go-fhir-client"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

// errNotFound reports that the store holds no such client.
var errNotFound = errors.New("not found")

// store is De Zonnebloem's FHIR store as the screens use it. Handler tests swap
// in a fake; fhirStore is the real one.
type store interface {
	Clients(ctx context.Context) ([]client, error)
	Client(ctx context.Context, id string) (client, error)
	Record(ctx context.Context, clientID string) (record, error)
	AddAllergy(ctx context.Context, allergy fhir.AllergyIntolerance) error
	Allergy(ctx context.Context, id string) (fhir.AllergyIntolerance, error)
	DeleteAllergy(ctx context.Context, id string) error
}

// searchCount bounds every search to one page. A care home's client list and one
// client's record fit well inside it; the demo store holds seven clients.
const searchCount = "200"

// freshResults asks HAPI not to answer from its cache of identical searches,
// which it keeps for a while (60 seconds by default): a record added a moment
// ago has to show on the next page view.
var freshResults = fhirclient.RequestHeaders(http.Header{"Cache-Control": {"no-cache"}})

type fhirStore struct {
	fhir fhirclient.Client
}

func newFHIRStore(baseURL *url.URL, httpClient *http.Client) *fhirStore {
	return &fhirStore{fhir: fhirclient.New(baseURL, httpClient, nil)}
}

func (s *fhirStore) Clients(ctx context.Context) ([]client, error) {
	patients, err := search[fhir.Patient](ctx, s.fhir, "Patient", url.Values{})
	if err != nil {
		return nil, err
	}
	clients := make([]client, 0, len(patients))
	for _, p := range patients {
		clients = append(clients, clientFrom(p))
	}
	sort.SliceStable(clients, func(i, j int) bool {
		return clients[i].sortKey < clients[j].sortKey
	})
	return clients, nil
}

func (s *fhirStore) Client(ctx context.Context, id string) (client, error) {
	var patient fhir.Patient
	var status int
	err := s.fhir.ReadWithContext(ctx, "Patient/"+id, &patient, fhirclient.ResponseStatusCode(&status))
	if status == http.StatusNotFound || status == http.StatusGone {
		return client{}, errNotFound
	}
	if err != nil {
		return client{}, fmt.Errorf("read client %s: %w", id, err)
	}
	c := clientFrom(patient)
	c.ID = id
	return c, nil
}

func (s *fhirStore) Record(ctx context.Context, clientID string) (record, error) {
	ref := "Patient/" + clientID
	allergies, err := search[fhir.AllergyIntolerance](ctx, s.fhir, "AllergyIntolerance", url.Values{"patient": {ref}})
	if err != nil {
		return record{}, err
	}
	medication, err := search[fhir.MedicationRequest](ctx, s.fhir, "MedicationRequest", url.Values{"subject": {ref}})
	if err != nil {
		return record{}, err
	}
	conditions, err := search[fhir.Condition](ctx, s.fhir, "Condition", url.Values{"subject": {ref}})
	if err != nil {
		return record{}, err
	}
	var rec record
	for _, a := range allergies {
		rec.Allergies = append(rec.Allergies, allergyEntry(a))
	}
	// What was added during the demo first: that is what the presenter just typed.
	sort.SliceStable(rec.Allergies, func(i, j int) bool {
		return rec.Allergies[i].AddedInDemo && !rec.Allergies[j].AddedInDemo
	})
	for _, m := range medication {
		rec.Medication = append(rec.Medication, medicationEntry(m))
	}
	for _, c := range conditions {
		rec.Conditions = append(rec.Conditions, conditionEntry(c))
	}
	return rec, nil
}

func (s *fhirStore) AddAllergy(ctx context.Context, allergy fhir.AllergyIntolerance) error {
	var created fhir.AllergyIntolerance
	if err := s.fhir.CreateWithContext(ctx, allergy, &created); err != nil {
		return fmt.Errorf("create allergy: %w", err)
	}
	return nil
}

// Allergy reads one allergy. errNotFound covers both one the store never held
// and one deleted since, which HAPI answers with 410.
func (s *fhirStore) Allergy(ctx context.Context, id string) (fhir.AllergyIntolerance, error) {
	var allergy fhir.AllergyIntolerance
	var status int
	err := s.fhir.ReadWithContext(ctx, "AllergyIntolerance/"+id, &allergy, fhirclient.ResponseStatusCode(&status))
	if status == http.StatusNotFound || status == http.StatusGone {
		return fhir.AllergyIntolerance{}, errNotFound
	}
	if err != nil {
		return fhir.AllergyIntolerance{}, fmt.Errorf("read allergy %s: %w", id, err)
	}
	return allergy, nil
}

func (s *fhirStore) DeleteAllergy(ctx context.Context, id string) error {
	if err := s.fhir.DeleteWithContext(ctx, "AllergyIntolerance/"+id); err != nil {
		return fmt.Errorf("delete allergy %s: %w", id, err)
	}
	return nil
}

// search returns the resources of one type a search matched. An entry of another
// type, such as the OperationOutcome a server may add to a search result, is not
// a record and is skipped.
func search[T any](ctx context.Context, c fhirclient.Client, resourceType string, params url.Values) ([]T, error) {
	params.Set("_count", searchCount)
	var bundle fhir.Bundle
	if err := c.SearchWithContext(ctx, resourceType, params, &bundle, freshResults); err != nil {
		return nil, fmt.Errorf("search %s: %w", resourceType, err)
	}
	resources := make([]T, 0, len(bundle.Entry))
	for _, e := range bundle.Entry {
		var kind struct {
			ResourceType string `json:"resourceType"`
		}
		if err := json.Unmarshal(e.Resource, &kind); err != nil {
			return nil, fmt.Errorf("parse %s search entry: %w", resourceType, err)
		}
		if kind.ResourceType != resourceType {
			continue
		}
		var resource T
		if err := json.Unmarshal(e.Resource, &resource); err != nil {
			return nil, fmt.Errorf("parse %s: %w", resourceType, err)
		}
		resources = append(resources, resource)
	}
	return resources, nil
}
