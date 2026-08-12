package cmd

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/nuts-foundation/nuts-knooppunt/api"
	"github.com/nuts-foundation/nuts-knooppunt/component/lrza"
	"github.com/nuts-foundation/nuts-knooppunt/component/mitz"
	"github.com/nuts-foundation/nuts-knooppunt/component/nvi"
	"github.com/nuts-foundation/nuts-knooppunt/component/pdp"
	"github.com/nuts-foundation/nuts-knooppunt/component/status"
	"github.com/nuts-foundation/nuts-knooppunt/lib/fhirapi"
	"github.com/nuts-foundation/nuts-knooppunt/lib/logging"
)

// strictAPIServer implements api.StrictServerInterface, generated from openapi.yaml, by
// forwarding each operation to the component that owns it. The component-specific logic
// (translating between generated api.* types and the component's own domain types) lives on
// the component itself, not here — this is wiring only.
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
	return s.lrza.TriggerLrzaSync(ctx, r)
}

func (s *strictAPIServer) ListPolicyBundles(ctx context.Context, r api.ListPolicyBundlesRequestObject) (api.ListPolicyBundlesResponseObject, error) {
	return s.pdp.ListPolicyBundles(ctx, r)
}

func (s *strictAPIServer) GetPolicyBundle(ctx context.Context, r api.GetPolicyBundleRequestObject) (api.GetPolicyBundleResponseObject, error) {
	return s.pdp.GetPolicyBundle(ctx, r)
}

func (s *strictAPIServer) EvaluateAuthorization(ctx context.Context, r api.EvaluateAuthorizationRequestObject) (api.EvaluateAuthorizationResponseObject, error) {
	return s.pdp.EvaluateAuthorization(ctx, r)
}

func (s *strictAPIServer) RegisterListBundle(ctx context.Context, r api.RegisterListBundleRequestObject) (api.RegisterListBundleResponseObject, error) {
	return s.nvi.RegisterListBundle(ctx, r)
}

func (s *strictAPIServer) RegisterList(ctx context.Context, r api.RegisterListRequestObject) (api.RegisterListResponseObject, error) {
	return s.nvi.RegisterList(ctx, r)
}

func (s *strictAPIServer) SearchLists(ctx context.Context, r api.SearchListsRequestObject) (api.SearchListsResponseObject, error) {
	return s.nvi.SearchLists(ctx, r)
}

func (s *strictAPIServer) DeleteListsByParams(ctx context.Context, r api.DeleteListsByParamsRequestObject) (api.DeleteListsByParamsResponseObject, error) {
	return s.nvi.DeleteListsByParams(ctx, r)
}

func (s *strictAPIServer) SearchListsForm(ctx context.Context, r api.SearchListsFormRequestObject) (api.SearchListsFormResponseObject, error) {
	return s.nvi.SearchListsForm(ctx, r)
}

func (s *strictAPIServer) GetList(ctx context.Context, r api.GetListRequestObject) (api.GetListResponseObject, error) {
	return s.nvi.GetList(ctx, r)
}

func (s *strictAPIServer) DeleteList(ctx context.Context, r api.DeleteListRequestObject) (api.DeleteListResponseObject, error) {
	return s.nvi.DeleteList(ctx, r)
}

func (s *strictAPIServer) CreateMitzSubscription(ctx context.Context, r api.CreateMitzSubscriptionRequestObject) (api.CreateMitzSubscriptionResponseObject, error) {
	return s.mitz.CreateMitzSubscription(ctx, r)
}

var _ api.StrictServerInterface = (*strictAPIServer)(nil)

// RegisterAPIRoutes mounts the generated OpenAPI strict server's routes onto internalMux.
// Any component argument may be nil when that component is disabled, in which case its
// routes are not registered, matching the previous behavior of each component's own
// RegisterHttpHandlers method.
func RegisterAPIRoutes(internalMux *http.ServeMux, statusComponent *status.Component, lrzaComponent *lrza.Component, pdpComponent *pdp.Component, nviComponent *nvi.Component, mitzComponent *mitz.Component) {
	handler := api.NewStrictHandler(&strictAPIServer{
		status: statusComponent,
		lrza:   lrzaComponent,
		pdp:    pdpComponent,
		nvi:    nviComponent,
		mitz:   mitzComponent,
	}, nil)
	wrapper := api.ServerInterfaceWrapper{
		Handler: handler,
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
	}

	internalMux.HandleFunc("GET /status", wrapper.GetStatus)
	internalMux.HandleFunc("GET /version", wrapper.GetVersion)
	if lrzaComponent != nil {
		internalMux.HandleFunc("POST /lrza/update", wrapper.TriggerLrzaSync)
	}
	if pdpComponent != nil {
		internalMux.HandleFunc("GET /pdp/bundles", wrapper.ListPolicyBundles)
		internalMux.HandleFunc("GET /pdp/bundles/{policyName}", wrapper.GetPolicyBundle)
		internalMux.HandleFunc("POST /pdp/v1/data/knooppunt/authz", wrapper.EvaluateAuthorization)
	}
	if nviComponent != nil {
		internalMux.HandleFunc("POST /nvi", wrapper.RegisterListBundle)
		internalMux.HandleFunc("POST /nvi/List", wrapper.RegisterList)
		internalMux.HandleFunc("GET /nvi/List", wrapper.SearchLists)
		internalMux.HandleFunc("DELETE /nvi/List", wrapper.DeleteListsByParams)
		internalMux.HandleFunc("POST /nvi/List/_search", wrapper.SearchListsForm)
		internalMux.HandleFunc("GET /nvi/List/{id}", wrapper.GetList)
		internalMux.HandleFunc("DELETE /nvi/List/{id}", wrapper.DeleteList)
	}
	if mitzComponent != nil {
		internalMux.HandleFunc("POST /mitz/Subscription", wrapper.CreateMitzSubscription)
	}
}
