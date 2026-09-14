package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/nuts-foundation/nuts-knooppunt/lib/coding"
	"github.com/nuts-foundation/nuts-knooppunt/test/testdata/vectors/nvi"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

// bgzCallTimeout bounds one search at the source. The chain runs several in
// sequence inside one request, so the budget is per call rather than overall.
const bgzCallTimeout = 15 * time.Second

// demoMarkerPrefix is the convention for the marker path: a note beginning with
// it was typed into the source system by the presenter, so seeing it in the
// enriched record proves the data travelled rather than being local all along.
const demoMarkerPrefix = "DEMO-"

// bgzMedicationCategory is the category the BGZ policy requires on a
// MedicationRequest search. It is restated here rather than imported from the
// seed because it belongs to the query the policy authorizes, not to the
// fixture; test/testdata/vectors/pool carries the matching value on the data.
const bgzMedicationCategory = "http://snomed.info/sct|16076005"

// recordItem is one retrieved clinical element as the record screen shows it.
type recordItem struct {
	Section string
	Title   string
	Detail  string
	Source  string
	// Marker reports a DEMO- note, which the screen highlights.
	Marker bool
}

// queryOutcome is one authorized search and what the source answered. It is what
// the authorization screen renders: the status is the real decision, taken at
// the source and enforced by its PEP.
type queryOutcome struct {
	Label   string
	Request string
	Status  int
	Error   string
}

// chainOutcome is what the source's answer established about the request as a
// whole. Three states rather than a boolean, because "it said no" and "it did
// not answer" are different facts with different next actions, and a screen that
// renders the second as the first tells the presenter the source refused them
// when it was merely down.
type chainOutcome int

const (
	// chainFailed: nothing was established. A transport error, a timeout, a 5xx,
	// a 429, or a body that could not be read as a FHIR Bundle. It says nothing
	// about whether access would have been granted.
	chainFailed chainOutcome = iota
	// chainRefused: the source answered, and its answer was no. 401 or 403, the
	// two statuses its policy enforcement point uses to decline.
	chainRefused
	// chainGranted: the source answered and served the request.
	chainGranted
)

// sourceRetrieval is everything one source returned for one patient.
//
// Outcome and PatientID answer different questions and the screens must not
// conflate them: Outcome reports what the source's answer established, PatientID
// whether it actually holds this patient. A source can serve a request and hold
// nothing.
type sourceRetrieval struct {
	Source    sourceAddress
	Outcome   chainOutcome
	PatientID string
	Queries   []queryOutcome
	Items     []recordItem
}

// The three states the templates branch on. Methods rather than exported
// constants, because html/template cannot compare against a typed constant.
func (r sourceRetrieval) Granted() bool { return r.Outcome == chainGranted }
func (r sourceRetrieval) Refused() bool { return r.Outcome == chainRefused }
func (r sourceRetrieval) Failed() bool  { return r.Outcome == chainFailed }

// outcomeOf classifies one answer. Only the statuses a policy enforcement point
// uses to decline count as a refusal; everything else that is not a served
// response is a failure, including a 200 whose body could not be read.
func outcomeOf(outcome queryOutcome) chainOutcome {
	switch {
	case outcome.Status == http.StatusUnauthorized || outcome.Status == http.StatusForbidden:
		return chainRefused
	case outcome.Status == http.StatusOK && outcome.Error == "":
		return chainGranted
	}
	return chainFailed
}

// bgzSearch is the BGZ query that retrieves one localized data category.
//
// The two axes meet here. Which categories exist at a source comes from the NVI,
// in the GF Localization vocabulary; which query retrieves one is the BGZ 2017
// information standard, as enforced by component/pdp/policies/bgz/policy.rego.
// Params carries what that policy demands beyond the patient scope.
type bgzSearch struct {
	Label    string
	Section  string
	Resource string
	Params   url.Values

	// InBody sends the parameters as a form-encoded POST to [type]/_search
	// instead of a query string. Used for the one search that carries the BSN:
	// the PEP's access log records $request, so a BSN in the URL is written to a
	// file outside anything this application can clear or expire. The PDP reads
	// search parameters from the body for a search-type interaction with this
	// content type (component/pdp/fhirreq.go), so the policy decision is
	// unchanged.
	InBody bool
}

var bgzSearches = map[string]bgzSearch{
	nvi.CategoryAllergyIntolerance: {
		Label: "Allergies", Section: "Allergies", Resource: "AllergyIntolerance",
	},
	nvi.CategoryCondition: {
		Label: "Conditions", Section: "Conditions", Resource: "Condition",
	},
	nvi.CategoryMedicationRequest: {
		Label: "Medication", Section: "Medication", Resource: "MedicationRequest",
		Params: url.Values{
			"category": {bgzMedicationCategory},
			"_include": {"MedicationRequest:medication"},
		},
	},
}

// retrieveBGZ runs the retrieval leg against one source: the Patient search that
// both opens the BGZ and resolves the source's own id for this patient, then one
// search per localized category.
//
// It returns an error only when the chain could not be run at all. A refusal is
// an outcome the screens render, not a failure: reporting a 403 as an error
// would lose the distinction between "the source said no" and "the source could
// not be reached", which are the two things the demo most needs to tell apart.
func retrieveBGZ(ctx context.Context, source sourceAddress, token, bsn string, categories []string) (sourceRetrieval, error) {
	base, err := url.Parse(source.Address)
	if err != nil {
		return sourceRetrieval{}, fmt.Errorf("the directory gave an unusable address %q: %w", source.Address, err)
	}
	client := &http.Client{Timeout: bgzCallTimeout}
	result := sourceRetrieval{Source: source}

	// The patient scope travels as the BSN here and as a local reference in every
	// later search. The source is the only party that knows its own id for this
	// patient, so the chain has to ask before it can scope anything else.
	patientBundle, outcome := runBGZSearch(ctx, client, base, token, bgzSearch{
		Label: "Patient", Section: "", Resource: "Patient", InBody: true,
		Params: url.Values{
			"identifier": {coding.BSNNamingSystem + "|" + bsn},
			"_include":   {"Patient:general-practitioner"},
		},
	})
	result.Queries = append(result.Queries, outcome)
	result.Outcome = outcomeOf(outcome)
	if result.Outcome != chainGranted {
		return result, nil
	}

	patientID, found := firstResourceID(patientBundle, "Patient")
	if !found {
		// Appended, not assigned over the existing Error: this branch is only
		// reached on a body we could read, so there is nothing to overwrite here
		// today, but assigning would have hidden a parse failure behind a claim
		// about the source's records the moment the guard above loosened.
		result.Queries[0].Error = joinNonEmpty(" · ", result.Queries[0].Error,
			"the source served the request but holds no patient with this BSN")
		return result, nil
	}
	result.PatientID = patientID

	// Driven by the categories the NVI reported, in the order it reported them,
	// so the screen is a consequence of the localization rather than a fixed list.
	for _, category := range categories {
		search, retrievable := bgzSearches[category]
		if !retrievable {
			continue
		}
		params := url.Values{"patient": {"Patient/" + patientID}}
		for name, values := range search.Params {
			params[name] = values
		}
		search.Params = params

		bundles, outcome := runPagedBGZSearch(ctx, client, base, token, search)
		result.Queries = append(result.Queries, outcome)
		if outcomeOf(outcome) != chainGranted {
			continue
		}
		for _, bundle := range bundles {
			result.Items = append(result.Items, itemsFrom(bundle, search, source.Name)...)
		}
	}
	return result, nil
}

// maxSearchPages bounds how far a paged result is followed. A source that keeps
// offering a next link cannot hold the demo open indefinitely, and stopping is
// reported rather than hidden.
const maxSearchPages = 20

// runPagedBGZSearch runs one search and follows the server-supplied next links.
// FHIR R4 pages search results (https://hl7.org/fhir/R4/http.html#paging), so
// reading only the first page would render a partial clinical record as a whole
// one. The outcome reported is the first page's: a later page that fails leaves
// the result incomplete, which is recorded in Error rather than turning the
// whole search into a failure.
func runPagedBGZSearch(ctx context.Context, client *http.Client, base *url.URL, token string, search bgzSearch) ([]fhir.Bundle, queryOutcome) {
	bundle, outcome := runBGZSearch(ctx, client, base, token, search)
	bundles := []fhir.Bundle{bundle}
	if outcomeOf(outcome) != chainGranted {
		return bundles, outcome
	}

	for pages := 1; ; pages++ {
		next := nextPageURL(bundles[len(bundles)-1])
		if next == "" {
			return bundles, outcome
		}
		if pages >= maxSearchPages {
			outcome.Error = joinNonEmpty(" · ", outcome.Error, fmt.Sprintf(
				"the source offered more than %d pages; this result is incomplete", maxSearchPages))
			return bundles, outcome
		}
		page, pageOutcome := runPageAt(ctx, client, token, next)
		if outcomeOf(pageOutcome) != chainGranted {
			outcome.Error = joinNonEmpty(" · ", outcome.Error,
				"a later page could not be read, so this result is incomplete: "+
					joinNonEmpty(" ", pageOutcome.Error, http.StatusText(pageOutcome.Status)))
			return bundles, outcome
		}
		bundles = append(bundles, page)
	}
}

// nextPageURL returns the server-supplied continuation link, if any.
func nextPageURL(bundle fhir.Bundle) string {
	for _, link := range bundle.Link {
		if link.Relation == "next" && link.Url != "" {
			return link.Url
		}
	}
	return ""
}

// runPageAt follows one continuation link. The URL comes from the source, so it
// is used as given rather than rebuilt; it carries the source's own cursor.
func runPageAt(ctx context.Context, client *http.Client, token, pageURL string) (fhir.Bundle, queryOutcome) {
	outcome := queryOutcome{Label: "page", Request: "GET " + pageURL}
	ctx, cancel := context.WithTimeout(ctx, bgzCallTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, pageURL, nil)
	if err != nil {
		outcome.Error = err.Error()
		return fhir.Bundle{}, outcome
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/fhir+json")

	res, err := client.Do(req)
	if err != nil {
		outcome.Error = err.Error()
		return fhir.Bundle{}, outcome
	}
	defer res.Body.Close()
	outcome.Status = res.StatusCode
	if res.StatusCode != http.StatusOK {
		return fhir.Bundle{}, outcome
	}
	var bundle fhir.Bundle
	if err := json.NewDecoder(res.Body).Decode(&bundle); err != nil {
		outcome.Error = "the source returned something that is not a FHIR Bundle: " + err.Error()
	}
	return bundle, outcome
}

// runBGZSearch issues one search and reports what came back. A transport failure
// lands in Error with a zero Status, which the screens render as "could not be
// established" rather than as a refusal.
func runBGZSearch(ctx context.Context, client *http.Client, base *url.URL, token string, search bgzSearch) (fhir.Bundle, queryOutcome) {
	method, body := http.MethodGet, io.Reader(nil)
	target := base.JoinPath(search.Resource)
	rendered := search.Params

	if search.InBody {
		method = http.MethodPost
		target = target.JoinPath("_search")
		body = strings.NewReader(search.Params.Encode())
		// What the screen shows. The parameters travelled in the body precisely
		// so they would not be logged, and this string is rendered on a
		// projector, so it names them without their values.
		rendered = url.Values{}
		for name := range search.Params {
			rendered[name] = []string{"[sent in the request body]"}
		}
	} else {
		target.RawQuery = search.Params.Encode()
	}
	outcome := queryOutcome{Label: search.Label, Request: method + " " + target.Path + "?" + rendered.Encode()}

	ctx, cancel := context.WithTimeout(ctx, bgzCallTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, method, target.String(), body)
	if err != nil {
		outcome.Error = err.Error()
		return fhir.Bundle{}, outcome
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/fhir+json")
	if search.InBody {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}

	res, err := client.Do(req)
	if err != nil {
		outcome.Error = err.Error()
		return fhir.Bundle{}, outcome
	}
	defer res.Body.Close()
	outcome.Status = res.StatusCode
	if res.StatusCode != http.StatusOK {
		return fhir.Bundle{}, outcome
	}

	var bundle fhir.Bundle
	if err := json.NewDecoder(res.Body).Decode(&bundle); err != nil {
		outcome.Error = "the source returned something that is not a FHIR Bundle: " + err.Error()
	}
	return bundle, outcome
}

// firstResourceID returns the id of the first resource of the given type. The
// type check matters: _include puts other resources in the same bundle, and the
// general-practitioner include means a Practitioner can precede the Patient.
func firstResourceID(bundle fhir.Bundle, resourceType string) (string, bool) {
	for _, entry := range bundle.Entry {
		var resource struct {
			ResourceType string `json:"resourceType"`
			Id           string `json:"id"`
		}
		if err := json.Unmarshal(entry.Resource, &resource); err != nil {
			continue
		}
		if resource.ResourceType == resourceType && resource.Id != "" {
			return resource.Id, true
		}
	}
	return "", false
}

// itemsFrom turns one search result into record items, skipping anything that is
// not the resource the search asked for: _include brings companions along, and a
// Medication carried in for MedicationRequest:medication is not a record line of
// its own.
func itemsFrom(bundle fhir.Bundle, search bgzSearch, sourceName string) []recordItem {
	included := includedNames(bundle)
	var items []recordItem
	for _, entry := range bundle.Entry {
		var peek struct {
			ResourceType string `json:"resourceType"`
		}
		if err := json.Unmarshal(entry.Resource, &peek); err != nil || peek.ResourceType != search.Resource {
			continue
		}
		item, ok := itemFrom(entry.Resource, search, included)
		if !ok {
			continue
		}
		item.Source = sourceName
		items = append(items, item)
	}
	return items
}

// includedNames indexes the display name of every resource in the bundle by
// "Type/id", so a resource that names another by reference can be rendered. The
// searches ask for the companions they need through _include; without this the
// answer arrives and is thrown away.
func includedNames(bundle fhir.Bundle) map[string]string {
	names := map[string]string{}
	for _, entry := range bundle.Entry {
		var resource struct {
			ResourceType string                `json:"resourceType"`
			Id           string                `json:"id"`
			Code         *fhir.CodeableConcept `json:"code"`
			Name         *string               `json:"name"`
		}
		if err := json.Unmarshal(entry.Resource, &resource); err != nil || resource.Id == "" {
			continue
		}
		name := codeableText(resource.Code)
		if name == "" && resource.Name != nil {
			name = *resource.Name
		}
		if name != "" {
			names[resource.ResourceType+"/"+resource.Id] = name
		}
	}
	return names
}

func itemFrom(resource json.RawMessage, search bgzSearch, included map[string]string) (recordItem, bool) {
	item := recordItem{Section: search.Section}
	switch search.Resource {
	case "AllergyIntolerance":
		var allergy fhir.AllergyIntolerance
		if json.Unmarshal(resource, &allergy) != nil {
			return item, false
		}
		item.Title = codeableText(allergy.Code)
		item.Detail = joinNonEmpty(" · ", annotationText(allergy.Note), stringOrEmpty(allergy.RecordedDate))
		item.Marker = hasDemoMarker(allergy.Note)
	case "MedicationRequest":
		var medication fhir.MedicationRequest
		if json.Unmarshal(resource, &medication) != nil {
			return item, false
		}
		item.Title = codeableText(medication.MedicationCodeableConcept)
		// FHIR allows the medication inline or by reference
		// (https://hl7.org/fhir/R4/medicationrequest-definitions.html). The search
		// asks for MedicationRequest:medication precisely so the referenced
		// Medication travels with it; reading only the inline form dropped the
		// whole line from the record.
		if item.Title == "" && medication.MedicationReference != nil && medication.MedicationReference.Reference != nil {
			item.Title = included[*medication.MedicationReference.Reference]
		}
		if len(medication.DosageInstruction) > 0 {
			item.Detail = stringOrEmpty(medication.DosageInstruction[0].Text)
		}
		item.Marker = hasDemoMarker(medication.Note)
	case "Condition":
		var condition fhir.Condition
		if json.Unmarshal(resource, &condition) != nil {
			return item, false
		}
		item.Title = codeableText(condition.Code)
		item.Detail = stringOrEmpty(condition.OnsetDateTime)
		item.Marker = hasDemoMarker(condition.Note)
	default:
		return item, false
	}
	return item, item.Title != ""
}

// codeableText prefers the human text a source wrote over a coding display,
// because that is what the source meant the reader to see.
func codeableText(concept *fhir.CodeableConcept) string {
	if concept == nil {
		return ""
	}
	if concept.Text != nil && *concept.Text != "" {
		return *concept.Text
	}
	for _, coding := range concept.Coding {
		if coding.Display != nil && *coding.Display != "" {
			return *coding.Display
		}
	}
	return ""
}

func hasDemoMarker(notes []fhir.Annotation) bool {
	return strings.Contains(annotationText(notes), demoMarkerPrefix)
}

func annotationText(notes []fhir.Annotation) string {
	var texts []string
	for _, note := range notes {
		if note.Text != "" {
			texts = append(texts, note.Text)
		}
	}
	return strings.Join(texts, " ")
}

func joinNonEmpty(separator string, values ...string) string {
	var present []string
	for _, value := range values {
		if value != "" {
			present = append(present, value)
		}
	}
	return strings.Join(present, separator)
}

func stringOrEmpty(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
