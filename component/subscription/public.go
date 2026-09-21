package subscription

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/nuts-foundation/nuts-knooppunt/api/subscriptionpublic"
	"github.com/nuts-foundation/nuts-knooppunt/lib/fhirsubscription"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/caramel/to"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

// Component implements the public (cross-organisation) FHIR surface generated from
// subscription-public.openapi.yaml, mirroring how mitz.Component owns mitzpublic.
//
// The generated schemas are plain JSON maps, converted here from/to the real FHIR
// types: the strict server's per-status response types are *defined types* over the
// schema type, which would silently drop the MarshalJSON methods that write
// resourceType — with maps, the wire form is exactly what the FHIR types marshal.
var _ subscriptionpublic.StrictServerInterface = (*Component)(nil)

// fhirMap round-trips a FHIR resource through JSON into a generic map, preserving the
// resource's own MarshalJSON output (including resourceType).
func fhirMap(v any) map[string]interface{} {
	data, err := json.Marshal(v)
	if err != nil {
		return map[string]interface{}{}
	}
	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		return map[string]interface{}{}
	}
	return result
}

// outcome builds a FHIR OperationOutcome map for an error response (implementation
// obligation 4: all transactions return an OperationOutcome on error).
func outcome(issueType fhir.IssueType, message string) subscriptionpublic.OperationOutcomeErrorApplicationFhirPlusJSONResponse {
	return fhirMap(fhir.OperationOutcome{
		Issue: []fhir.OperationOutcomeIssue{{
			Severity:    fhir.IssueSeverityError,
			Code:        issueType,
			Diagnostics: to.Ptr(message),
		}},
	})
}

func (c *Component) CreateSubscription(ctx context.Context, request subscriptionpublic.CreateSubscriptionRequestObject) (subscriptionpublic.CreateSubscriptionResponseObject, error) {
	if request.Body == nil {
		return subscriptionpublic.CreateSubscription400ApplicationFhirPlusJSONResponse{
			OperationOutcomeErrorApplicationFhirPlusJSONResponse: outcome(fhir.IssueTypeInvalid, "request body is required"),
		}, nil
	}
	raw, err := json.Marshal(request.Body)
	if err != nil {
		return subscriptionpublic.CreateSubscription400ApplicationFhirPlusJSONResponse{
			OperationOutcomeErrorApplicationFhirPlusJSONResponse: outcome(fhir.IssueTypeInvalid, err.Error()),
		}, nil
	}
	incoming, err := fhirsubscription.UnmarshalSubscription(raw)
	if err != nil {
		return subscriptionpublic.CreateSubscription400ApplicationFhirPlusJSONResponse{
			OperationOutcomeErrorApplicationFhirPlusJSONResponse: outcome(fhir.IssueTypeInvalid, err.Error()),
		}, nil
	}
	created, reqErr := c.CreateInBandSubscription(ctx, incoming)
	if reqErr != nil {
		if reqErr.Status == http.StatusUnprocessableEntity {
			return subscriptionpublic.CreateSubscription422ApplicationFhirPlusJSONResponse(outcome(fhir.IssueTypeBusinessRule, reqErr.Message)), nil
		}
		return subscriptionpublic.CreateSubscription400ApplicationFhirPlusJSONResponse{
			OperationOutcomeErrorApplicationFhirPlusJSONResponse: outcome(fhir.IssueTypeInvalid, reqErr.Message),
		}, nil
	}
	return subscriptionpublic.CreateSubscription201ApplicationFhirPlusJSONResponse(fhirMap(created)), nil
}

func (c *Component) GetSubscription(ctx context.Context, request subscriptionpublic.GetSubscriptionRequestObject) (subscriptionpublic.GetSubscriptionResponseObject, error) {
	sub, ok := c.store.getServerSub(request.Id)
	if !ok {
		return subscriptionpublic.GetSubscription404ApplicationFhirPlusJSONResponse{
			OperationOutcomeErrorApplicationFhirPlusJSONResponse: outcome(fhir.IssueTypeNotFound, "unknown Subscription "+request.Id),
		}, nil
	}
	return subscriptionpublic.GetSubscription200ApplicationFhirPlusJSONResponse(fhirMap(c.asBackportResource(sub))), nil
}

func (c *Component) GetSubscriptionStatus(ctx context.Context, request subscriptionpublic.GetSubscriptionStatusRequestObject) (subscriptionpublic.GetSubscriptionStatusResponseObject, error) {
	bundle, ok := c.ServeStatus(request.Id)
	if !ok {
		return subscriptionpublic.GetSubscriptionStatus404ApplicationFhirPlusJSONResponse{
			OperationOutcomeErrorApplicationFhirPlusJSONResponse: outcome(fhir.IssueTypeNotFound, "unknown Subscription "+request.Id),
		}, nil
	}
	return subscriptionpublic.GetSubscriptionStatus200ApplicationFhirPlusJSONResponse(fhirMap(bundle)), nil
}

func (c *Component) GetSubscriptionEvents(ctx context.Context, request subscriptionpublic.GetSubscriptionEventsRequestObject) (subscriptionpublic.GetSubscriptionEventsResponseObject, error) {
	bundle, status, message := c.ServeEvents(request.Id, request.Params.EventsSinceNumber, request.Params.EventsUntilNumber)
	switch status {
	case http.StatusOK:
		return subscriptionpublic.GetSubscriptionEvents200ApplicationFhirPlusJSONResponse(fhirMap(bundle)), nil
	case http.StatusNotFound:
		return subscriptionpublic.GetSubscriptionEvents404ApplicationFhirPlusJSONResponse{
			OperationOutcomeErrorApplicationFhirPlusJSONResponse: outcome(fhir.IssueTypeNotFound, message),
		}, nil
	default:
		return subscriptionpublic.GetSubscriptionEvents422ApplicationFhirPlusJSONResponse(outcome(fhir.IssueTypeBusinessRule, message)), nil
	}
}

func (c *Component) GetSubscriptionTopic(ctx context.Context, request subscriptionpublic.GetSubscriptionTopicRequestObject) (subscriptionpublic.GetSubscriptionTopicResponseObject, error) {
	if taskTopic.ID == nil || request.Id != *taskTopic.ID {
		return subscriptionpublic.GetSubscriptionTopic404ApplicationFhirPlusJSONResponse{
			OperationOutcomeErrorApplicationFhirPlusJSONResponse: outcome(fhir.IssueTypeNotFound, "unknown SubscriptionTopic "+request.Id),
		}, nil
	}
	return subscriptionpublic.GetSubscriptionTopic200ApplicationFhirPlusJSONResponse(fhirMap(taskTopic)), nil
}

// ReadAliasedTask serves the follow-up pull for id-only notifications: de-alias the
// opaque id and read the Task from the organisation's FHIR store. This is the knooppunt
// standing in the data path — the architectural cost of aliasing non-compliant ids
// (research question 8 / doc 09 honest limit 3).
func (c *Component) ReadAliasedTask(ctx context.Context, request subscriptionpublic.ReadAliasedTaskRequestObject) (subscriptionpublic.ReadAliasedTaskResponseObject, error) {
	notFound := func(message string) subscriptionpublic.ReadAliasedTaskResponseObject {
		return subscriptionpublic.ReadAliasedTask404ApplicationFhirPlusJSONResponse{
			OperationOutcomeErrorApplicationFhirPlusJSONResponse: outcome(fhir.IssueTypeNotFound, message),
		}
	}
	internalRef, ok := c.store.resolveAlias(request.Id)
	if !ok {
		// Not disclosing whether an alias could exist: unknown alias and unknown
		// resource are indistinguishable (404, not 410).
		return notFound("unknown Task " + request.Id), nil
	}
	if c.config.Server.FHIRBaseURL == "" {
		return notFound("no FHIR store configured to serve the follow-up pull"), nil
	}
	body, status, err := c.getJSON(ctx, c.config.Server.FHIRBaseURL+"/"+internalRef)
	if err != nil || status != http.StatusOK {
		return notFound(fmt.Sprintf("reading %s failed", request.Id)), nil
	}
	var resource map[string]interface{}
	if err := json.Unmarshal(body, &resource); err != nil {
		return notFound(fmt.Sprintf("reading %s failed", request.Id)), nil
	}
	// The internal id must not leak through the pull response: it would defeat the
	// alias (pseudo id-only). Rewrite the resource id to the alias.
	resource["id"] = request.Id
	return subscriptionpublic.ReadAliasedTask200ApplicationFhirPlusJSONResponse(resource), nil
}

func (c *Component) ReceiveNotification(ctx context.Context, request subscriptionpublic.ReceiveNotificationRequestObject) (subscriptionpublic.ReceiveNotificationResponseObject, error) {
	if request.Body == nil {
		return subscriptionpublic.ReceiveNotification400ApplicationFhirPlusJSONResponse{
			OperationOutcomeErrorApplicationFhirPlusJSONResponse: outcome(fhir.IssueTypeInvalid, "request body is required"),
		}, nil
	}
	raw, err := json.Marshal(request.Body)
	if err != nil {
		return subscriptionpublic.ReceiveNotification400ApplicationFhirPlusJSONResponse{
			OperationOutcomeErrorApplicationFhirPlusJSONResponse: outcome(fhir.IssueTypeInvalid, err.Error()),
		}, nil
	}
	var bundle fhir.Bundle
	if err := json.Unmarshal(raw, &bundle); err != nil {
		return subscriptionpublic.ReceiveNotification400ApplicationFhirPlusJSONResponse{
			OperationOutcomeErrorApplicationFhirPlusJSONResponse: outcome(fhir.IssueTypeInvalid, "invalid notification Bundle: "+err.Error()),
		}, nil
	}
	status, message := c.HandleNotification(ctx, bundle)
	switch status {
	case http.StatusOK:
		return subscriptionpublic.ReceiveNotification200Response{}, nil
	case http.StatusUnprocessableEntity:
		return subscriptionpublic.ReceiveNotification422ApplicationFhirPlusJSONResponse(outcome(fhir.IssueTypeBusinessRule, message)), nil
	default:
		return subscriptionpublic.ReceiveNotification400ApplicationFhirPlusJSONResponse{
			OperationOutcomeErrorApplicationFhirPlusJSONResponse: outcome(fhir.IssueTypeInvalid, message),
		}, nil
	}
}
