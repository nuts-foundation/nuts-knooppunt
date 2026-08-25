package cmd

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/nuts-foundation/nuts-knooppunt/api"
	"github.com/nuts-foundation/nuts-knooppunt/component"
	"github.com/nuts-foundation/nuts-knooppunt/component/lrza"
	"github.com/nuts-foundation/nuts-knooppunt/component/mitz"
	"github.com/nuts-foundation/nuts-knooppunt/component/nvi"
	"github.com/nuts-foundation/nuts-knooppunt/component/pdp"
	"github.com/nuts-foundation/nuts-knooppunt/component/status"
	"github.com/nuts-foundation/nuts-knooppunt/lib/fhirapi"
	"github.com/nuts-foundation/nuts-knooppunt/lib/logging"
)

// errComponentDisabled is returned by a forwarding method when the component that owns the
// operation was disabled (its field on strictAPIServer is nil). RegisterHttpHandlers' response
// error handler turns this into a 404, matching the previous behavior of simply not registering
// the route at all.
var errComponentDisabled = errors.New("component disabled")

// strictAPIServer implements api.StrictServerInterface, generated from openapi.yaml, by
// forwarding each operation to the component that owns it. The component-specific logic
// (translating between generated api.* types and the component's own domain types) lives on
// the component itself, not here — this is wiring only.
//
// The forwarding methods below are hand-written, one per operation, rather than generated:
// they're a single line each, and a generator would need its own way to enumerate operations and
// map each to the component that owns it — information that doesn't live in openapi.yaml, so it
// would still need to be maintained by hand somewhere, just in a generator's input instead of
// here directly.
//
// This does not fail silently when openapi.yaml changes. The var _ api.StrictServerInterface
// assertion below requires strictAPIServer to implement every operation in the generated
// interface: add an operation to the spec, run `go generate ./api/...`, and the next `go build`
// fails at that assertion, naming the exact missing method, until you add its forwarding method
// here. RegisterHttpHandlers registers every operation's route unconditionally (see its own
// comment), so once the build passes, wiring is complete — there's no separate route list to
// remember to update.
//
// Any field may be left nil when that component is disabled. A forwarding method for an operation
// owned by such a component must check for nil itself and return errComponentDisabled — see e.g.
// TriggerLrzaSync below — since its route is still registered either way.
//
// strictAPIServer is itself a component.Lifecycle, added to the components slice in start.go
// like any other component, so it goes through the same RegisterHttpHandlers/Start/Stop loop
// instead of a separate call — Start and Stop are no-ops since it owns no resources of its own.
type strictAPIServer struct {
	status *status.Component
	lrza   *lrza.Component
	pdp    *pdp.Component
	nvi    *nvi.Component
	mitz   *mitz.Component
}

func (s *strictAPIServer) GetStatus(ctx context.Context, r api.GetStatusRequestObject) (api.GetStatusResponseObject, error) {
	return s.status.GetStatus(ctx, r)
}

func (s *strictAPIServer) GetVersion(ctx context.Context, r api.GetVersionRequestObject) (api.GetVersionResponseObject, error) {
	return s.status.GetVersion(ctx, r)
}

func (s *strictAPIServer) TriggerLrzaSync(ctx context.Context, r api.TriggerLrzaSyncRequestObject) (api.TriggerLrzaSyncResponseObject, error) {
	if s.lrza == nil {
		return nil, errComponentDisabled
	}
	return s.lrza.TriggerLrzaSync(ctx, r)
}

func (s *strictAPIServer) ListAuthorizationPolicyBundles(ctx context.Context, r api.ListAuthorizationPolicyBundlesRequestObject) (api.ListAuthorizationPolicyBundlesResponseObject, error) {
	if s.pdp == nil {
		return nil, errComponentDisabled
	}
	return s.pdp.ListAuthorizationPolicyBundles(ctx, r)
}

func (s *strictAPIServer) GetAuthorizationPolicyBundle(ctx context.Context, r api.GetAuthorizationPolicyBundleRequestObject) (api.GetAuthorizationPolicyBundleResponseObject, error) {
	if s.pdp == nil {
		return nil, errComponentDisabled
	}
	return s.pdp.GetAuthorizationPolicyBundle(ctx, r)
}

func (s *strictAPIServer) EvaluateAuthorization(ctx context.Context, r api.EvaluateAuthorizationRequestObject) (api.EvaluateAuthorizationResponseObject, error) {
	if s.pdp == nil {
		return nil, errComponentDisabled
	}
	return s.pdp.EvaluateAuthorization(ctx, r)
}

func (s *strictAPIServer) EvaluateDefaultAuthorization(ctx context.Context, r api.EvaluateDefaultAuthorizationRequestObject) (api.EvaluateDefaultAuthorizationResponseObject, error) {
	if s.pdp == nil {
		return nil, errComponentDisabled
	}
	return s.pdp.EvaluateDefaultAuthorization(ctx, r)
}

func (s *strictAPIServer) RegisterNVIListBundle(ctx context.Context, r api.RegisterNVIListBundleRequestObject) (api.RegisterNVIListBundleResponseObject, error) {
	if s.nvi == nil {
		return nil, errComponentDisabled
	}
	return s.nvi.RegisterNVIListBundle(ctx, r)
}

func (s *strictAPIServer) RegisterNVIList(ctx context.Context, r api.RegisterNVIListRequestObject) (api.RegisterNVIListResponseObject, error) {
	if s.nvi == nil {
		return nil, errComponentDisabled
	}
	return s.nvi.RegisterNVIList(ctx, r)
}

func (s *strictAPIServer) SearchNVILists(ctx context.Context, r api.SearchNVIListsRequestObject) (api.SearchNVIListsResponseObject, error) {
	if s.nvi == nil {
		return nil, errComponentDisabled
	}
	return s.nvi.SearchNVILists(ctx, r)
}

func (s *strictAPIServer) DeleteNVIListsByParams(ctx context.Context, r api.DeleteNVIListsByParamsRequestObject) (api.DeleteNVIListsByParamsResponseObject, error) {
	if s.nvi == nil {
		return nil, errComponentDisabled
	}
	return s.nvi.DeleteNVIListsByParams(ctx, r)
}

func (s *strictAPIServer) SearchNVIListsForm(ctx context.Context, r api.SearchNVIListsFormRequestObject) (api.SearchNVIListsFormResponseObject, error) {
	if s.nvi == nil {
		return nil, errComponentDisabled
	}
	return s.nvi.SearchNVIListsForm(ctx, r)
}

func (s *strictAPIServer) GetNVIList(ctx context.Context, r api.GetNVIListRequestObject) (api.GetNVIListResponseObject, error) {
	if s.nvi == nil {
		return nil, errComponentDisabled
	}
	return s.nvi.GetNVIList(ctx, r)
}

func (s *strictAPIServer) DeleteNVIList(ctx context.Context, r api.DeleteNVIListRequestObject) (api.DeleteNVIListResponseObject, error) {
	if s.nvi == nil {
		return nil, errComponentDisabled
	}
	return s.nvi.DeleteNVIList(ctx, r)
}

func (s *strictAPIServer) CreateMitzSubscription(ctx context.Context, r api.CreateMitzSubscriptionRequestObject) (api.CreateMitzSubscriptionResponseObject, error) {
	if s.mitz == nil {
		return nil, errComponentDisabled
	}
	return s.mitz.CreateMitzSubscription(ctx, r)
}

var _ api.StrictServerInterface = (*strictAPIServer)(nil)
var _ component.Lifecycle = (*strictAPIServer)(nil)

func (s *strictAPIServer) Start() error {
	return nil
}

func (s *strictAPIServer) Stop(_ context.Context) error {
	return nil
}

// RegisterHttpHandlers mounts the generated OpenAPI strict server's routes onto internalMux; all
// of them are internal-only, so publicMux is unused.
//
// Every operation's route is registered unconditionally via api.HandlerWithOptions, regardless of
// whether the component that owns it is enabled: unlike the manual per-route registration this
// replaced, there's no way to forget one. A disabled component's forwarding method (see above)
// returns errComponentDisabled instead, which ResponseErrorHandlerFunc below turns into a 404 —
// matching the previous behavior of not registering the route at all.
func (s *strictAPIServer) RegisterHttpHandlers(_ *http.ServeMux, internalMux *http.ServeMux) {
	handler := api.NewStrictHandlerWithOptions(s, nil, api.StrictHTTPServerOptions{
		ResponseErrorHandlerFunc: func(w http.ResponseWriter, r *http.Request, err error) {
			if errors.Is(err, errComponentDisabled) {
				http.NotFound(w, r)
				return
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
		},
	})
	api.HandlerWithOptions(handler, api.StdHTTPServerOptions{
		BaseRouter: internalMux,
		// NVI/MITZ params (the X-Tenant-ID header, search query params) are the only ones
		// declared in openapi.yaml, so this is only ever hit for those two components' routes;
		// respond the same way their own code does for a bad/missing request: a FHIR
		// OperationOutcome, not the framework's plain-text default.
		ErrorHandlerFunc: func(w http.ResponseWriter, r *http.Request, err error) {
			badRequestErr := fhirapi.BadRequestError(err.Error(), err)
			slog.ErrorContext(r.Context(), "FHIR API error", logging.Error(badRequestErr))
			_ = api.OperationOutcomeResponse{
				StatusCode: http.StatusBadRequest,
				Outcome:    fhirapi.OperationOutcomeForError(badRequestErr),
			}.Write(w)
		},
	})
}
