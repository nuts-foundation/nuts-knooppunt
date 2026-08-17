package api

import (
	"bytes"
	"encoding/json"
	"net/http"
)

// OperationOutcomeResponse implements the XxxResponseObject Visit method of every operation
// whose error responses are just an HTTP status code plus a FHIR OperationOutcome body — the
// shape every operation in openapi.yaml uses for its error responses (400/422/500/503, all
// $ref: OperationOutcomeError). Each operation still gets its own generated Go response type per
// status code — that's inherent to the strict-server pattern, since each operation's
// ResponseObject is its own interface — but this collapses the actual response-writing logic
// and construction into one shared implementation instead of one per operation.
//
// Hand-written, not generated: this file is never touched by `go generate`, which only writes
// to server.gen.go (see oapi-codegen.yaml).
type OperationOutcomeResponse struct {
	StatusCode int
	Outcome    FHIROperationOutcome
}

func (r OperationOutcomeResponse) Write(w http.ResponseWriter) error {
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(r.Outcome); err != nil {
		return err
	}
	w.Header().Set("Content-Type", "application/fhir+json")
	w.WriteHeader(r.StatusCode)
	_, err := buf.WriteTo(w)
	return err
}

// The methods below exist only because Go interfaces require exact method names: each
// operation's XxxResponseObject is its own interface (VisitRegisterListBundleResponse,
// VisitGetListResponse, ...), so satisfying several of them means implementing several
// identically-shaped methods — there's no way to express "any operation whose error response is
// an OperationOutcome" as a single method. This is intentionally NOT generated (see the
// package-level comment on why): the boilerplate is a couple of lines per operation, whereas a
// generator would mean either re-implementing $ref resolution against openapi.yaml or depending
// on oapi-codegen's internal template format, which is far more maintenance surface for less
// benefit.
//
// This is safe to maintain by hand: it does NOT fail silently if you add a new operation to
// openapi.yaml that uses the OperationOutcomeError response and forget to add its Visit method
// here. The first call site that tries to return an OperationOutcomeResponse as that operation's
// ResponseObject (typically a `nviError(ctx, err)`-style helper in the owning component) will
// fail to compile, naming the exact missing method. If you add a new operation's error handling
// and the compiler doesn't complain, you don't need a new method here.
func (r OperationOutcomeResponse) VisitRegisterListBundleResponse(w http.ResponseWriter) error {
	return r.Write(w)
}

func (r OperationOutcomeResponse) VisitRegisterListResponse(w http.ResponseWriter) error {
	return r.Write(w)
}

func (r OperationOutcomeResponse) VisitGetListResponse(w http.ResponseWriter) error {
	return r.Write(w)
}

func (r OperationOutcomeResponse) VisitDeleteListResponse(w http.ResponseWriter) error {
	return r.Write(w)
}

func (r OperationOutcomeResponse) VisitDeleteListsByParamsResponse(w http.ResponseWriter) error {
	return r.Write(w)
}

func (r OperationOutcomeResponse) VisitSearchListsResponse(w http.ResponseWriter) error {
	return r.Write(w)
}

func (r OperationOutcomeResponse) VisitSearchListsFormResponse(w http.ResponseWriter) error {
	return r.Write(w)
}

func (r OperationOutcomeResponse) VisitCreateMitzSubscriptionResponse(w http.ResponseWriter) error {
	return r.Write(w)
}
