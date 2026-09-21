package subscription

import (
	"context"
	"net/http"

	"github.com/nuts-foundation/nuts-knooppunt/api"
	"github.com/nuts-foundation/nuts-knooppunt/lib/fhirsubscription"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/caramel/to"
)

// The internal, vendor-facing API (doc 09's proposed contract), implemented against the
// generated api.* types and wired through strictAPIServer in package cmd. Deliberately
// plain JSON: the point of the split is that a vendor integrates without touching the
// backport IG or the wire format.

func (c *Component) IngestSubscriptionEvent(ctx context.Context, request api.IngestSubscriptionEventRequestObject) (api.IngestSubscriptionEventResponseObject, error) {
	if request.Body == nil {
		return api.IngestSubscriptionEvent400JSONResponse{SubscriptionErrorJSONResponse: api.SubscriptionErrorJSONResponse{Error: "request body is required"}}, nil
	}
	if !c.config.Server.Enabled {
		return api.IngestSubscriptionEvent400JSONResponse{SubscriptionErrorJSONResponse: api.SubscriptionErrorJSONResponse{Error: "this knooppunt does not run the Subscription Server role"}}, nil
	}
	body := request.Body
	ev := event{
		ResourceType: body.ResourceType,
		ResourceID:   body.ResourceId,
	}
	if body.Interaction != nil {
		ev.Interaction = string(*body.Interaction)
	}
	if body.OwnerUra != nil {
		ev.OwnerURA = *body.OwnerUra
	}
	if body.UseCaseCode != nil {
		ev.UseCaseCode = *body.UseCaseCode
	}
	suppress := body.SuppressDelivery != nil && *body.SuppressDelivery

	matches := c.IngestEvent(ctx, ev, suppress)
	report := api.SubscriptionEventIngestReport{Matches: []struct {
		EventNumber    int64  `json:"eventNumber"`
		SubscriptionId string `json:"subscriptionId"`
		Suppressed     *bool  `json:"suppressed,omitempty"`
	}{}}
	for _, match := range matches {
		entry := struct {
			EventNumber    int64  `json:"eventNumber"`
			SubscriptionId string `json:"subscriptionId"`
			Suppressed     *bool  `json:"suppressed,omitempty"`
		}{EventNumber: match.EventNumber, SubscriptionId: match.SubscriptionID}
		if match.Suppressed {
			entry.Suppressed = to.Ptr(true)
		}
		report.Matches = append(report.Matches, entry)
	}
	return api.IngestSubscriptionEvent200JSONResponse(report), nil
}

func (c *Component) CreateOutOfBandSubscription(ctx context.Context, request api.CreateOutOfBandSubscriptionRequestObject) (api.CreateOutOfBandSubscriptionResponseObject, error) {
	if request.Body == nil {
		return api.CreateOutOfBandSubscription400JSONResponse{SubscriptionErrorJSONResponse: api.SubscriptionErrorJSONResponse{Error: "request body is required"}}, nil
	}
	body := request.Body
	mode := fhirsubscription.PayloadMode("")
	if body.PayloadMode != nil {
		mode = fhirsubscription.PayloadMode(*body.PayloadMode)
	}
	endpoint := ""
	if body.NotificationEndpoint != nil {
		endpoint = *body.NotificationEndpoint
	}
	sub, reqErr := c.CreateOutOfBand(ctx, body.PartnerUra, body.Topic, mode, body.HeartbeatPeriodSeconds, endpoint)
	if reqErr != nil {
		if reqErr.Status == http.StatusUnprocessableEntity {
			return api.CreateOutOfBandSubscription422JSONResponse(api.SubscriptionAPIError{Error: reqErr.Message}), nil
		}
		return api.CreateOutOfBandSubscription400JSONResponse{SubscriptionErrorJSONResponse: api.SubscriptionErrorJSONResponse{Error: reqErr.Message}}, nil
	}
	return api.CreateOutOfBandSubscription201JSONResponse(serverSubToAPI(sub)), nil
}

func (c *Component) ListSubscriptions(ctx context.Context, _ api.ListSubscriptionsRequestObject) (api.ListSubscriptionsResponseObject, error) {
	listing := api.SubscriptionListing{
		Server: []api.ServerSubscriptionInfo{},
		Client: []api.ClientSubscriptionInfo{},
	}
	for _, sub := range c.store.serverSubscriptionsSnapshot() {
		listing.Server = append(listing.Server, serverSubToAPI(sub))
	}
	for _, sub := range c.store.clientSubscriptionsSnapshot() {
		listing.Client = append(listing.Client, clientSubToAPI(sub))
	}
	return api.ListSubscriptions200JSONResponse(listing), nil
}

func (c *Component) RegisterSubscriptionAuthorization(ctx context.Context, request api.RegisterSubscriptionAuthorizationRequestObject) (api.RegisterSubscriptionAuthorizationResponseObject, error) {
	if request.Body == nil {
		return api.RegisterSubscriptionAuthorization400JSONResponse{SubscriptionErrorJSONResponse: api.SubscriptionErrorJSONResponse{Error: "request body is required"}}, nil
	}
	body := request.Body
	basis := authorizationBasis{
		ReceiverURA: body.ReceiverUra,
		Topic:       body.Topic,
		ValidUntil:  body.ValidUntil,
	}
	if body.PatientBsn != nil {
		basis.PatientBSN = *body.PatientBsn
	}
	c.store.addAuthorizationBasis(basis)
	c.store.appendLog(logEntry{Direction: "received", Type: "authorization-basis", Outcome: "registered", Detail: "receiver URA " + body.ReceiverUra + ", topic " + body.Topic})
	return api.RegisterSubscriptionAuthorization201JSONResponse(*body), nil
}

func (c *Component) CreateSubscriptionIntent(ctx context.Context, request api.CreateSubscriptionIntentRequestObject) (api.CreateSubscriptionIntentResponseObject, error) {
	if request.Body == nil {
		return api.CreateSubscriptionIntent400JSONResponse{SubscriptionErrorJSONResponse: api.SubscriptionErrorJSONResponse{Error: "request body is required"}}, nil
	}
	body := request.Body
	mode := fhirsubscription.PayloadMode("")
	if body.PayloadMode != nil {
		mode = fhirsubscription.PayloadMode(*body.PayloadMode)
	}
	base := ""
	if body.PartnerFhirBaseUrl != nil {
		base = *body.PartnerFhirBaseUrl
	}
	sub, reqErr := c.CreateIntent(ctx, body.PartnerUra, body.Topic, mode, body.HeartbeatPeriodSeconds, base)
	if reqErr != nil {
		if reqErr.Status == http.StatusUnprocessableEntity {
			return api.CreateSubscriptionIntent422JSONResponse(api.SubscriptionAPIError{Error: reqErr.Message}), nil
		}
		return api.CreateSubscriptionIntent400JSONResponse{SubscriptionErrorJSONResponse: api.SubscriptionErrorJSONResponse{Error: reqErr.Message}}, nil
	}
	return api.CreateSubscriptionIntent201JSONResponse(clientSubToAPI(sub)), nil
}

func (c *Component) ListSubscriptionInbox(ctx context.Context, request api.ListSubscriptionInboxRequestObject) (api.ListSubscriptionInboxResponseObject, error) {
	since := int64(0)
	if request.Params.Since != nil {
		since = *request.Params.Since
	}
	includeAcked := request.Params.All != nil && *request.Params.All
	entries := c.store.inboxSnapshot(since, includeAcked)
	response := api.ListSubscriptionInbox200JSONResponse{}
	for _, entry := range entries {
		response = append(response, api.SubscriptionInboxEvent{
			Id:                    entry.ID,
			Type:                  api.SubscriptionInboxEventType(entry.Type),
			SubscriptionReference: entry.SubscriptionReference,
			Topic:                 to.Ptr(entry.Topic),
			EventNumber:           entry.EventNumber,
			Focus:                 entry.Focus,
			GapFrom:               entry.GapFrom,
			GapTo:                 entry.GapTo,
			ReceivedAt:            entry.ReceivedAt,
			Acknowledged:          entry.Acknowledged,
		})
	}
	return response, nil
}

func (c *Component) AckSubscriptionInboxEvent(ctx context.Context, request api.AckSubscriptionInboxEventRequestObject) (api.AckSubscriptionInboxEventResponseObject, error) {
	if !c.store.ackInbox(request.Id) {
		return api.AckSubscriptionInboxEvent404JSONResponse{SubscriptionErrorJSONResponse: api.SubscriptionErrorJSONResponse{Error: "unknown inbox entry"}}, nil
	}
	return api.AckSubscriptionInboxEvent204Response{}, nil
}

func (c *Component) ListSubscriptionLog(ctx context.Context, _ api.ListSubscriptionLogRequestObject) (api.ListSubscriptionLogResponseObject, error) {
	response := api.ListSubscriptionLog200JSONResponse{}
	for _, entry := range c.store.logEntries() {
		item := api.SubscriptionTransportLogEntry{
			Time:      entry.Time,
			Direction: api.SubscriptionTransportLogEntryDirection(entry.Direction),
			Type:      entry.Type,
			Outcome:   entry.Outcome,
		}
		if entry.Subscription != "" {
			item.Subscription = to.Ptr(entry.Subscription)
		}
		item.EventNumber = entry.EventNumber
		if entry.PayloadMode != "" {
			item.PayloadMode = to.Ptr(entry.PayloadMode)
		}
		if entry.Detail != "" {
			item.Detail = to.Ptr(entry.Detail)
		}
		response = append(response, item)
	}
	return response, nil
}

func serverSubToAPI(sub serverSubscription) api.ServerSubscriptionInfo {
	info := api.ServerSubscriptionInfo{
		Id:                     sub.ID,
		Topic:                  sub.Topic,
		PayloadMode:            string(sub.Mode),
		Status:                 api.ServerSubscriptionInfoStatus(sub.Status),
		CreatedVia:             api.ServerSubscriptionInfoCreatedVia(sub.CreatedVia),
		HeartbeatPeriodSeconds: sub.HeartbeatPeriod,
		EventCounter:           to.Ptr(sub.EventCounter),
	}
	if sub.PartnerURA != "" {
		info.PartnerUra = to.Ptr(sub.PartnerURA)
	} else if ura := sub.OwnerURAFilter(); ura != "" {
		info.PartnerUra = to.Ptr(ura)
	}
	if sub.Endpoint != "" {
		info.Endpoint = to.Ptr(sub.Endpoint)
	}
	return info
}

func clientSubToAPI(sub clientSubscription) api.ClientSubscriptionInfo {
	info := api.ClientSubscriptionInfo{
		Reference:              sub.Reference,
		Topic:                  sub.Topic,
		PayloadMode:            string(sub.Mode),
		Status:                 api.ClientSubscriptionInfoStatus(sub.Status),
		Origin:                 api.ClientSubscriptionInfoOrigin(sub.Origin),
		HeartbeatPeriodSeconds: sub.HeartbeatPeriod,
		HighestEventNumber:     to.Ptr(sub.HighestEvent),
	}
	if sub.ID != "" {
		info.Id = to.Ptr(sub.ID)
	}
	if sub.PartnerURA != "" {
		info.PartnerUra = to.Ptr(sub.PartnerURA)
	}
	if sub.ServerFHIRBase != "" {
		info.ServerFhirBaseUrl = to.Ptr(sub.ServerFHIRBase)
	}
	return info
}
