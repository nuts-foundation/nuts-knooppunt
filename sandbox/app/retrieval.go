package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
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

	// Truncated says the source served this search but more of it exists than
	// was read. Kept apart from Error on purpose: a partial answer is still an
	// answer, and folding the two together made a truncation discard every page
	// that had already been read.
	Truncated string

	// Note explains a served result the reader would otherwise misread, such as
	// a search that matched nothing. Also kept apart from Error: putting an
	// explanation there made a complete, empty answer count as a retrieval that
	// fell short.
	Note string
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

// Incomplete reports that something the source authorized was not retrieved: a
// search that failed, or one that was served only in part.
//
// Separate from Outcome because they answer different questions. Outcome is the
// authorization verdict, which the Patient search establishes; this is about
// what came back afterwards. Without it a chain whose every clinical search
// returned 503 still rendered "access granted", four passed checks and a link to
// data that was not there.
func (r sourceRetrieval) Incomplete() bool {
	for _, outcome := range r.Queries {
		if outcome.Truncated != "" || outcomeOf(outcome) != chainGranted {
			return true
		}
	}
	return false
}

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
	// Redirects are refused rather than followed. A FHIR search has no reason to
	// redirect, and following one would put the access token on a hop the
	// directory never resolved. The standard library strips the Authorization
	// header across hosts, but relying on that leaves the decision implicit.
	client := &http.Client{
		Transport: newCaptureTransport("exchange", nil),
		Timeout:   bgzCallTimeout,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	result := sourceRetrieval{Source: source}

	// The patient scope travels as the BSN here and as a local reference in every
	// later search. The source is the only party that knows its own id for this
	// patient, so the chain has to ask before it can scope anything else.
	// Paged like the clinical searches. The match can sit behind a continuation
	// link, and the answer to this one search decides whether the source knows
	// this patient at all: reading a single page would turn "not on page one"
	// into "not here" (https://hl7.org/fhir/R4/http.html#paging).
	patientBundles, outcome := runPagedBGZSearch(ctx, client, base, token, bsn, bgzSearch{
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

	patientIDs, unreadable := resourceIDs(patientBundles, "Patient")
	switch {
	case len(patientIDs) > 1:
		// Which of them is the patient on screen is a question the source has to
		// answer. Taking the first would let the order the server happened to
		// serve its entries in decide whose clinical record is shown as this
		// patient's.
		addTruncated(&result.Queries[0], fmt.Sprintf(
			"the source holds %d patients with this BSN, so which record is this patient's "+
				"was not established and nothing further was asked", len(patientIDs)))
		return result, nil
	case unreadable > 0:
		// The source meant these entries as matches. One of them could be a second
		// patient, so a readable entry beside them settles nothing: picking it
		// would show one person's record as this patient's on the strength of
		// which entry happened to decode.
		reason := fmt.Sprintf("%d patient record(s) in this answer could not be read, so whether "+
			"the source holds this patient was not established", unreadable)
		if len(patientIDs) > 0 {
			reason = fmt.Sprintf("%d patient record(s) in this answer could not be read alongside %d "+
				"that could, so how many patients match this BSN was not established, and nothing "+
				"further was asked", unreadable, len(patientIDs))
		}
		addTruncated(&result.Queries[0], reason)
		return result, nil
	case outcome.Truncated != "":
		// The search did not finish. With no id that makes its silence not an
		// answer; with one it makes "one match" not an answer either, because a
		// second record for this BSN can sit on the page that was never read.
		// Either way the reason is already on the outcome and says the result is
		// incomplete.
		if len(patientIDs) > 0 {
			addTruncated(&result.Queries[0], "so whether this BSN matches more than one record at "+
				"this source was not established, and nothing further was asked")
		}
		return result, nil
	case len(patientIDs) == 0:
		// A Note, not an Error. The source answered in full; it simply holds
		// nobody with this BSN, and there is nothing further to ask, so the row
		// reads as a complete answer rather than as a retrieval that fell short.
		result.Queries[0].Note = "the source served the request but holds no patient with this BSN"
		return result, nil
	}
	patientID := patientIDs[0]
	// The BSN travels in the body of the search above precisely so it stays out
	// of request lines. The id that search returns goes into the query string of
	// every search after it, and FHIR puts no constraint on what a server uses
	// as an id (https://hl7.org/fhir/R4/datatypes.html#id). A source that
	// identifies its patients by BSN would undo the whole arrangement, at the
	// source's PEP access log and on the screen that renders these requests, so
	// the chain stops instead of complying.
	if strings.Contains(patientID, bsn) {
		addTruncated(&result.Queries[0], "the source identifies this patient by a value containing the BSN, "+
			"which every following request would carry in its URL, so nothing further was asked")
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

		bundles, outcome := runPagedBGZSearch(ctx, client, base, token, bsn, search)
		result.Queries = append(result.Queries, outcome)
		if outcomeOf(outcome) != chainGranted {
			continue
		}
		unrendered := 0
		for _, bundle := range bundles {
			items, skipped := itemsFrom(bundle, search, source.Name)
			result.Items = append(result.Items, items...)
			unrendered += skipped
		}
		if unrendered > 0 {
			// Served, complete, and not all of it made the screen: exactly what
			// Truncated is for. Silently rendering fewer lines than the source
			// sent is the failure this reports, because the overview then looks
			// whole while the reader has no way to know it is not.
			addTruncated(&result.Queries[len(result.Queries)-1], fmt.Sprintf(
				"%d resource(s) in this answer could not be rendered, so this result is incomplete", unrendered))
		}
	}
	return result, nil
}

// addTruncated records one more reason a result is incomplete. Reasons
// accumulate rather than replace: a page that was never fetched and a resource
// that arrived unreadable are different losses, and writing one over the other
// tells the reader about the smaller one while the larger goes unmentioned.
func addTruncated(outcome *queryOutcome, reason string) {
	if outcome.Truncated == "" {
		outcome.Truncated = reason
		return
	}
	outcome.Truncated += "; " + reason
}

// maxSearchPages bounds how far a paged result is followed. A source that keeps
// offering a next link cannot hold the demo open indefinitely, and stopping is
// reported rather than hidden.
const maxSearchPages = 20

// runPagedBGZSearch runs one search and follows the server-supplied next links.
// FHIR R4 pages search results (https://hl7.org/fhir/R4/http.html#paging), so
// reading only the first page would render a partial clinical record as a whole
// one. The outcome reported is the first page's: a later page that fails leaves
// the result incomplete, which is recorded in Truncated. Error is what outcomeOf
// reads as a failed search, and a failed search discards the pages already in
// hand.
func runPagedBGZSearch(ctx context.Context, client *http.Client, base *url.URL, token, redact string, search bgzSearch) ([]fhir.Bundle, queryOutcome) {
	ctx = withEventResourceType(ctx, search.Resource)
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
			addTruncated(&outcome, fmt.Sprintf(
				"the source offered more than %d pages, so this result is incomplete", maxSearchPages))
			return bundles, outcome
		}
		if problem := continuationProblem(base, next, redact); problem != "" {
			addTruncated(&outcome, problem)
			return bundles, outcome
		}
		page, pageOutcome := runPageAt(ctx, client, token, next)
		if outcomeOf(pageOutcome) != chainGranted {
			addTruncated(&outcome, "a later page could not be read, so this result is incomplete: "+
				joinNonEmpty(" ", pageOutcome.Error, http.StatusText(pageOutcome.Status)))
			return bundles, outcome
		}
		bundles = append(bundles, page)
	}
}

// sameOrigin reports whether candidate has the scheme and host the directory
// resolved. A scheme downgrade counts as a different origin: it would put the
// token on the wire in the clear.
func sameOrigin(base *url.URL, candidate string) bool {
	parsed, err := url.Parse(candidate)
	if err != nil {
		return false
	}
	return parsed.Scheme == base.Scheme &&
		strings.EqualFold(parsed.Hostname(), base.Hostname()) &&
		effectivePort(parsed) == effectivePort(base)
}

// continuationProblem reports why a continuation link must not be requested, or
// "" when it may be. One place for the question, because the answers are three
// different reasons to stop and the screen has to name the right one: the reader
// draws a different conclusion from "the source pointed somewhere else" than
// from "the link was unusable".
//
// The shape is bounded rather than sanitized. A paging link is a cursor
// (https://hl7.org/fhir/R4/http.html#paging); credentials in one have no use
// here, and Go keeps the username when it formats a transport error, so anything
// carried there ends up in text this application stores and renders.
func continuationProblem(base *url.URL, next, identifier string) string {
	const notFollowed = ", which was not followed, so this result is incomplete"

	parsed, err := url.Parse(next)
	if err != nil {
		return "the source offered a continuation whose URL could not be read" + notFollowed
	}
	if parsed.User != nil {
		return "the source offered a continuation carrying credentials in its URL" + notFollowed
	}
	if !sameOrigin(base, next) {
		return "the source offered a continuation on another origin" + notFollowed
	}
	carries, readable := urlCarries(next, parsed, identifier)
	if !readable {
		return "the source offered a continuation whose URL could not be read" + notFollowed
	}
	if carries {
		return "the source offered a continuation carrying the patient identifier in its URL" + notFollowed
	}
	return ""
}

// urlCarries reports whether a URL says the given value anywhere, in any of the
// spellings a URL can say it in, and whether that question could be answered at
// all. Percent-encoding a digit changes the bytes and not the meaning (RFC 3986
// section 2.3), so a source can put an identifier in a request line in a form
// that no substring search of the raw link finds. Path and fragment arrive
// decoded from url.Parse; the query is decoded here.
//
// A query that cannot be decoded is reported as unreadable rather than as clean,
// because this decides whether to send a request and an unreadable link is not
// one this code can vouch for.
func urlCarries(raw string, parsed *url.URL, value string) (carries, readable bool) {
	if value == "" {
		return false, true
	}
	if strings.Contains(raw, value) ||
		strings.Contains(parsed.Path, value) ||
		strings.Contains(parsed.Opaque, value) ||
		strings.Contains(parsed.Fragment, value) {
		return true, true
	}
	unescaped, err := url.QueryUnescape(parsed.RawQuery)
	if err != nil {
		return false, false
	}
	return strings.Contains(unescaped, value), true
}

// effectivePort resolves the port the way RFC 6454 section 4 compares origins:
// an absent port is the scheme's default, so https://host and https://host:443
// are the same origin. A raw host comparison would call those different and drop
// the page for no benefit.
func effectivePort(u *url.URL) string {
	if port := u.Port(); port != "" {
		return port
	}
	switch u.Scheme {
	case "https":
		return "443"
	case "http":
		return "80"
	}
	return ""
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
	bundle, err := decodeSearchset(res.Body)
	if err != nil {
		outcome.Error = err.Error()
	}
	return bundle, outcome
}

// runBGZSearch issues one search and reports what came back. A transport failure
// lands in Error with a zero Status, which the screens render as "could not be
// established" rather than as a refusal.
func runBGZSearch(ctx context.Context, client *http.Client, base *url.URL, token string, search bgzSearch) (fhir.Bundle, queryOutcome) {
	ctx = withEventResourceType(ctx, search.Resource)
	method, body := http.MethodGet, io.Reader(nil)
	target := base.JoinPath(search.Resource)
	rendered := search.Params.Encode()

	if search.InBody {
		method = http.MethodPost
		target = target.JoinPath("_search")
		body = strings.NewReader(search.Params.Encode())
		// What the screen shows. The parameters travelled in the body precisely
		// so they would not be logged, and this string is read off a projector,
		// so it names them without their values. Assembled rather than encoded:
		// the placeholder is prose, and percent-encoding it renders the one part
		// of the row a reader is meant to understand as %5Bsent+in+...%5D.
		names := make([]string, 0, len(search.Params))
		for name := range search.Params {
			names = append(names, name)
		}
		sort.Strings(names)
		for i, name := range names {
			if i > 0 {
				rendered += "&"
			} else {
				rendered = ""
			}
			rendered += name + "=[sent in the request body]"
		}
	} else {
		target.RawQuery = rendered
	}
	outcome := queryOutcome{Label: search.Label, Request: method + " " + target.Path + "?" + rendered}

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

	bundle, err := decodeSearchset(res.Body)
	if err != nil {
		outcome.Error = err.Error()
	}
	return bundle, outcome
}

// decodeSearchset reads a search response and insists it really is one. Decoding
// without error is not the same as being a search result: null, an empty object,
// an OperationOutcome and a Bundle of another type all unmarshal happily into
// fhir.Bundle, and each was being reported as a served request that simply found
// nothing. FHIR requires a successful search to answer with a searchset Bundle
// (https://hl7.org/fhir/R4/http.html#search).
func decodeSearchset(body io.Reader) (fhir.Bundle, error) {
	raw, err := io.ReadAll(body)
	if err != nil {
		return fhir.Bundle{}, fmt.Errorf("the source's response could not be read: %w", err)
	}
	var envelope struct {
		ResourceType string `json:"resourceType"`
		Type         string `json:"type"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return fhir.Bundle{}, fmt.Errorf("the source returned something that is not a FHIR Bundle: %w", err)
	}
	if envelope.ResourceType != "Bundle" {
		return fhir.Bundle{}, fmt.Errorf(
			"the source answered with %s where a searchset Bundle was expected", describeResource(envelope.ResourceType))
	}
	if envelope.Type != "searchset" {
		return fhir.Bundle{}, fmt.Errorf(
			"the source answered with a Bundle of type %q where a searchset was expected", envelope.Type)
	}
	var bundle fhir.Bundle
	if err := json.Unmarshal(raw, &bundle); err != nil {
		return fhir.Bundle{}, fmt.Errorf("the source's Bundle could not be read: %w", err)
	}
	return bundle, nil
}

func describeResource(resourceType string) string {
	if resourceType == "" {
		return "no FHIR resource"
	}
	return "a " + resourceType
}

// resourceIDs returns the distinct ids of the given type across every page read,
// and how many entries of that type it could not read one from. The type check
// matters: _include puts other resources in the same bundle, and the
// general-practitioner include means a Practitioner can precede the Patient, on
// its own page or otherwise.
//
// Both return values are answers. One id is an identification; several is an
// identity question this application cannot settle; none with unreadable entries
// is a search whose result was not understood, which is not the same as a search
// that found nothing.
func resourceIDs(bundles []fhir.Bundle, resourceType string) (ids []string, unreadable int) {
	seen := map[string]bool{}
	for _, bundle := range bundles {
		for _, entry := range bundle.Entry {
			var resource struct {
				ResourceType string `json:"resourceType"`
				Id           string `json:"id"`
			}
			// Typed decoding can fail on an entry that is still recognizably of
			// this type, so the type is read first and separately: an id of the
			// wrong JSON type would otherwise make the whole entry invisible
			// rather than reported.
			if !entryIsOfType(entry.Resource, resourceType) {
				continue
			}
			if err := json.Unmarshal(entry.Resource, &resource); err != nil || resource.Id == "" {
				unreadable++
				continue
			}
			if !seen[resource.Id] {
				seen[resource.Id] = true
				ids = append(ids, resource.Id)
			}
		}
	}
	return ids, unreadable
}

// entryIsOfType reads only the resourceType, so an entry this chain cannot
// otherwise decode is still attributed to the search that asked for it.
func entryIsOfType(resource json.RawMessage, resourceType string) bool {
	var peek struct {
		ResourceType string `json:"resourceType"`
	}
	if err := json.Unmarshal(resource, &peek); err != nil {
		return false
	}
	return peek.ResourceType == resourceType
}

// itemsFrom turns one search result into record items, skipping anything that is
// not the resource the search asked for: _include brings companions along, and a
// Medication carried in for MedicationRequest:medication is not a record line of
// its own.
// It also reports how many matches of the searched-for type it could not turn
// into a line, so the caller can say the overview holds less than the answer
// did. Companions are not counted: they are not record lines to begin with.
func itemsFrom(bundle fhir.Bundle, search bgzSearch, sourceName string) ([]recordItem, int) {
	included := includedNames(bundle)
	var items []recordItem
	skipped := 0
	for _, entry := range bundle.Entry {
		var peek struct {
			ResourceType string `json:"resourceType"`
		}
		if err := json.Unmarshal(entry.Resource, &peek); err != nil || peek.ResourceType != search.Resource {
			continue
		}
		item, ok := itemFrom(entry.Resource, search, included)
		if !ok {
			skipped++
			continue
		}
		item.Source = sourceName
		items = append(items, item)
	}
	return items, skipped
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
		if err := json.Unmarshal(entry.Resource, &resource); err != nil {
			continue
		}
		name := codeableText(resource.Code)
		if name == "" && resource.Name != nil {
			name = *resource.Name
		}
		if name == "" {
			continue
		}
		if resource.Id != "" {
			names[resource.ResourceType+"/"+resource.Id] = name
		}
		// A reference may also be absolute, in which case it matches the entry's
		// fullUrl rather than a relative Type/id
		// (https://hl7.org/fhir/R4/references.html#literal).
		if entry.FullUrl != nil && *entry.FullUrl != "" {
			names[*entry.FullUrl] = name
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
	// Both are optional (https://hl7.org/fhir/R4/datatypes-definitions.html#CodeableConcept.text),
	// so a source may answer with the code alone. Rendering the token form keeps
	// the resource on the record: a line a reader has to look up beats a
	// diagnosis that silently is not there.
	for _, coding := range concept.Coding {
		if coding.Code != nil && *coding.Code != "" {
			return stringOrEmpty(coding.System) + "|" + *coding.Code
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
