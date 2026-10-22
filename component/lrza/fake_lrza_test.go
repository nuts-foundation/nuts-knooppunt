package lrza

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
)

// fakeLRZA is an in-memory FHIR server standing in for the LRZA: read, search by identifier or
// primary-organization, create, versioned update honoring If-Match, and transactions resolving
// urn:uuid references. It records every request it receives.
type fakeLRZA struct {
	t      *testing.T
	server *httptest.Server

	mux       sync.Mutex
	resources map[string]resource // by "Type/id"
	nextID    int
	requests  []recordedRequest
	// failures holds status codes to answer the next requests with, in order, before handling them.
	failures []int
}

type recordedRequest struct {
	method string
	path   string
	header http.Header
	body   []byte
}

func newFakeLRZA(t *testing.T, resources ...resource) *fakeLRZA {
	t.Helper()
	f := &fakeLRZA{t: t, resources: map[string]resource{}}
	for _, res := range resources {
		f.put(res.clone())
	}
	f.server = httptest.NewServer(http.HandlerFunc(f.handle))
	t.Cleanup(f.server.Close)
	return f
}

// put stores a resource as a new version, returning that version.
func (f *fakeLRZA) put(res resource) resource {
	version := 1
	if existing, ok := f.resources[res.reference()]; ok {
		version, _ = strconv.Atoi(existing.versionID())
		version++
	}
	meta, _ := res["meta"].(map[string]any)
	if meta == nil {
		meta = map[string]any{}
		res["meta"] = meta
	}
	meta["versionId"] = strconv.Itoa(version)
	f.resources[res.reference()] = res
	return res
}

func (f *fakeLRZA) get(reference string) resource {
	f.mux.Lock()
	defer f.mux.Unlock()
	return f.resources[reference].clone()
}

// requestsTo returns the recorded requests with the given method and path.
func (f *fakeLRZA) requestsTo(method, path string) []recordedRequest {
	f.mux.Lock()
	defer f.mux.Unlock()
	var result []recordedRequest
	for _, r := range f.requests {
		if r.method == method && r.path == path {
			result = append(result, r)
		}
	}
	return result
}

// writes returns the recorded POST and PUT requests.
func (f *fakeLRZA) writes() []recordedRequest {
	f.mux.Lock()
	defer f.mux.Unlock()
	var result []recordedRequest
	for _, r := range f.requests {
		if r.method == http.MethodPost || r.method == http.MethodPut {
			result = append(result, r)
		}
	}
	return result
}

func (f *fakeLRZA) failNext(statuses ...int) {
	f.mux.Lock()
	defer f.mux.Unlock()
	f.failures = append(f.failures, statuses...)
}

func (f *fakeLRZA) handle(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	f.mux.Lock()
	defer f.mux.Unlock()
	f.requests = append(f.requests, recordedRequest{method: r.Method, path: r.URL.Path, header: r.Header.Clone(), body: body})

	if len(f.failures) > 0 {
		status := f.failures[0]
		f.failures = f.failures[1:]
		if status == http.StatusTooManyRequests {
			w.Header().Set("Retry-After", "0")
		}
		writeOutcome(w, status, "injected failure")
		return
	}

	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	switch {
	case r.Method == http.MethodGet && len(parts) == 2:
		res, ok := f.resources[parts[0]+"/"+parts[1]]
		if !ok {
			writeOutcome(w, http.StatusNotFound, "not found")
			return
		}
		writeResource(w, http.StatusOK, res)
	case r.Method == http.MethodGet && len(parts) == 1:
		f.search(w, parts[0], r)
	case r.Method == http.MethodPost && len(parts) == 1 && parts[0] == "":
		f.transaction(w, body)
	case r.Method == http.MethodPost && len(parts) == 1:
		var res resource
		if err := json.Unmarshal(body, &res); err != nil {
			writeOutcome(w, http.StatusBadRequest, err.Error())
			return
		}
		res["id"] = f.newID()
		stored := f.put(res)
		w.Header().Set("Location", f.server.URL+"/"+stored.reference()+"/_history/"+stored.versionID())
		writeResource(w, http.StatusCreated, stored)
	case r.Method == http.MethodPut && len(parts) == 2:
		var res resource
		if err := json.Unmarshal(body, &res); err != nil {
			writeOutcome(w, http.StatusBadRequest, err.Error())
			return
		}
		if status, message := f.checkIfMatch(parts[0]+"/"+parts[1], r.Header.Get("If-Match")); status != 0 {
			writeOutcome(w, status, message)
			return
		}
		_, existed := f.resources[parts[0]+"/"+parts[1]]
		status := http.StatusOK
		if !existed {
			status = http.StatusCreated
		}
		writeResource(w, status, f.put(res))
	default:
		writeOutcome(w, http.StatusBadRequest, "unsupported request")
	}
}

func (f *fakeLRZA) newID() string {
	f.nextID++
	return fmt.Sprintf("generated-%d", f.nextID)
}

func (f *fakeLRZA) checkIfMatch(reference, ifMatch string) (int, string) {
	if ifMatch == "" {
		return 0, ""
	}
	current, ok := f.resources[reference]
	if !ok || ifMatch != `W/"`+current.versionID()+`"` {
		return http.StatusPreconditionFailed, "version conflict: " + ifMatch
	}
	return 0, ""
}

func (f *fakeLRZA) search(w http.ResponseWriter, resourceType string, r *http.Request) {
	var matches []any
	for _, res := range f.resources {
		if res.resourceType() != resourceType {
			continue
		}
		if token := r.URL.Query().Get("identifier"); token != "" {
			found := false
			for _, identifier := range identifiers(res) {
				if fmt.Sprintf("%v|%v", identifier["system"], identifier["value"]) == token {
					found = true
				}
			}
			if !found {
				continue
			}
		}
		if primary := r.URL.Query().Get("primary-organization"); primary != "" {
			organization, _ := res["organization"].(map[string]any)
			if organization["reference"] != primary {
				continue
			}
		}
		matches = append(matches, map[string]any{"resource": res, "search": map[string]any{"mode": "match"}})
	}
	writeJSON(w, http.StatusOK, map[string]any{"resourceType": "Bundle", "type": "searchset", "entry": matches})
}

// transaction applies a transaction all-or-nothing: every entry's ifMatch is checked first.
func (f *fakeLRZA) transaction(w http.ResponseWriter, body []byte) {
	var bundle struct {
		Entry []struct {
			FullUrl  string   `json:"fullUrl"`
			Resource resource `json:"resource"`
			Request  struct {
				Method  string `json:"method"`
				Url     string `json:"url"`
				IfMatch string `json:"ifMatch"`
			} `json:"request"`
		} `json:"entry"`
	}
	if err := json.Unmarshal(body, &bundle); err != nil {
		writeOutcome(w, http.StatusBadRequest, err.Error())
		return
	}
	references := map[string]string{}
	for i, entry := range bundle.Entry {
		switch entry.Request.Method {
		case http.MethodPost:
			bundle.Entry[i].Resource["id"] = f.newID()
			references[entry.FullUrl] = entry.Resource.reference()
		case http.MethodPut:
			if status, message := f.checkIfMatch(entry.Request.Url, entry.Request.IfMatch); status != 0 {
				writeOutcome(w, status, message)
				return
			}
		default:
			writeOutcome(w, http.StatusBadRequest, "unsupported transaction method "+entry.Request.Method)
			return
		}
	}
	var responseEntries []any
	for _, entry := range bundle.Entry {
		replaceReferences(map[string]any(entry.Resource), references)
		status := "201 Created"
		if _, existed := f.resources[entry.Resource.reference()]; existed {
			status = "200 OK"
		}
		stored := f.put(entry.Resource)
		responseEntries = append(responseEntries, map[string]any{
			"resource": stored,
			"response": map[string]any{
				"status":   status,
				"location": stored.reference() + "/_history/" + stored.versionID(),
				"etag":     `W/"` + stored.versionID() + `"`,
			},
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"resourceType": "Bundle", "type": "transaction-response", "entry": responseEntries})
}

func replaceReferences(value any, references map[string]string) {
	switch typed := value.(type) {
	case map[string]any:
		if ref, ok := typed["reference"].(string); ok {
			if replacement, ok := references[ref]; ok {
				typed["reference"] = replacement
			}
		}
		for _, child := range typed {
			replaceReferences(child, references)
		}
	case []any:
		for _, child := range typed {
			replaceReferences(child, references)
		}
	}
}

func writeResource(w http.ResponseWriter, status int, res resource) {
	w.Header().Set("ETag", `W/"`+res.versionID()+`"`)
	writeJSON(w, status, res)
}

func writeOutcome(w http.ResponseWriter, status int, diagnostics string) {
	writeJSON(w, status, map[string]any{
		"resourceType": "OperationOutcome",
		"issue":        []any{map[string]any{"severity": "error", "code": "processing", "diagnostics": diagnostics}},
	})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/fhir+json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
