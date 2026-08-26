package api

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/nuts-foundation/nuts-knooppunt/lib/fhirapi"
	"github.com/nuts-foundation/nuts-knooppunt/lib/logging"
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
//
// The VisitXxxResponse methods below exist only because Go interfaces require exact method
// names: each operation's XxxResponseObject is its own interface (VisitRegisterNVIListBundleResponse,
// VisitGetNVIListResponse, ...), so satisfying several of them means implementing several
// identically-shaped methods — there's no way to express "any operation whose error response is
// an OperationOutcome" as a single method. This is intentionally not generated: the boilerplate
// is a couple of lines per operation, whereas a generator would mean either re-implementing $ref
// resolution against openapi.yaml or depending on oapi-codegen's internal template format, which
// is far more maintenance surface for less benefit.
//
// This is safe to maintain by hand: it does NOT fail silently if you add a new operation to
// openapi.yaml that uses the OperationOutcomeError response and forget to add its Visit method
// here. The first call site that tries to return an OperationOutcomeResponse as that operation's
// ResponseObject (typically via NewOperationOutcomeResponse) will fail to compile, naming the
// exact missing method. If you add a new operation's error handling and the compiler doesn't
// complain, you don't need a new method here.
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

// NewOperationOutcomeResponse logs err and builds the OperationOutcomeResponse it should be
// returned as: the HTTP status code and FHIR OperationOutcome body are both derived from err via
// fhirapi.StatusCodeForError/OperationOutcomeForError. Shared by every operation whose declared
// error response is OperationOutcomeError (currently NVI and MITZ) — not specific to either.
func NewOperationOutcomeResponse(ctx context.Context, err error) OperationOutcomeResponse {
	slog.ErrorContext(ctx, "FHIR API error", logging.Error(err))
	return OperationOutcomeResponse{StatusCode: fhirapi.StatusCodeForError(err), Outcome: fhirapi.OperationOutcomeForError(err)}
}

func (r OperationOutcomeResponse) VisitRegisterNVIListBundleResponse(w http.ResponseWriter) error {
	return r.Write(w)
}

func (r OperationOutcomeResponse) VisitRegisterNVIListResponse(w http.ResponseWriter) error {
	return r.Write(w)
}

func (r OperationOutcomeResponse) VisitGetNVIListResponse(w http.ResponseWriter) error {
	return r.Write(w)
}

func (r OperationOutcomeResponse) VisitDeleteNVIListResponse(w http.ResponseWriter) error {
	return r.Write(w)
}

func (r OperationOutcomeResponse) VisitDeleteNVIListsByParamsResponse(w http.ResponseWriter) error {
	return r.Write(w)
}

func (r OperationOutcomeResponse) VisitSearchNVIListsResponse(w http.ResponseWriter) error {
	return r.Write(w)
}

func (r OperationOutcomeResponse) VisitSearchNVIListsFormResponse(w http.ResponseWriter) error {
	return r.Write(w)
}

func (r OperationOutcomeResponse) VisitCreateMitzSubscriptionResponse(w http.ResponseWriter) error {
	return r.Write(w)
}
