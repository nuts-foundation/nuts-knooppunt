package fhirsubscription

import (
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/caramel/to"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

// NotificationEvent is one notification-event group inside a SubscriptionStatus.
type NotificationEvent struct {
	EventNumber int64
	Timestamp   *time.Time
	// Focus is the reference to the affected resource; present only in id-only mode.
	Focus *string
	// FocusFullURL is the absolute URL of the focus at the server, used as the
	// entry.fullUrl of the accompanying GET entry. Optional; falls back to Focus.
	FocusFullURL *string
}

// Notification is the parsed/buildable content of a notification Bundle: the
// SubscriptionStatus Parameters plus, for event notifications, the event groups.
type Notification struct {
	Type NotificationType
	// SubscriptionReference is the reference to the registered Subscription. The TTA
	// spec's examples use a relative reference (Subscription/{id}); this implementation
	// emits an absolute URL so an out-of-band receiver can locate the resource — see
	// FINDINGS on out-of-band bootstrap.
	SubscriptionReference string
	Status                Status
	Events                []NotificationEvent
}

// BuildBundle renders the notification as a backport-subscription-notification history
// Bundle. mode determines whether focus references appear (id-only) or not (empty);
// clinical content is never included (full-resource is out of POC scope).
func (n Notification) BuildBundle(mode PayloadMode) (fhir.Bundle, error) {
	params := fhir.Parameters{
		Meta: &fhir.Meta{Profile: []string{ProfileSubscriptionStatusR4}},
		Parameter: []fhir.ParametersParameter{
			{Name: "subscription", ValueReference: &fhir.Reference{Reference: to.Ptr(n.SubscriptionReference)}},
			{Name: "status", ValueCode: to.Ptr(string(n.Status))},
			{Name: "type", ValueCode: to.Ptr(string(n.Type))},
		},
	}
	for _, event := range n.Events {
		part := []fhir.ParametersParameter{
			{Name: "event-number", ValueString: to.Ptr(strconv.FormatInt(event.EventNumber, 10))},
		}
		if event.Timestamp != nil {
			part = append(part, fhir.ParametersParameter{
				Name:         "timestamp",
				ValueInstant: to.Ptr(event.Timestamp.UTC().Format(time.RFC3339)),
			})
		}
		if mode == PayloadModeIDOnly && event.Focus != nil {
			part = append(part, fhir.ParametersParameter{
				Name:           "focus",
				ValueReference: &fhir.Reference{Reference: event.Focus},
			})
		}
		params.Parameter = append(params.Parameter, fhir.ParametersParameter{
			Name: "notification-event",
			Part: part,
		})
	}

	paramsJSON, err := json.Marshal(params)
	if err != nil {
		return fhir.Bundle{}, fmt.Errorf("marshal SubscriptionStatus parameters: %w", err)
	}
	bundle := fhir.Bundle{
		Meta:      &fhir.Meta{Profile: []string{ProfileNotificationR4}},
		Type:      fhir.BundleTypeHistory,
		Timestamp: to.Ptr(time.Now().UTC().Format(time.RFC3339)),
		Entry: []fhir.BundleEntry{{
			FullUrl:  to.Ptr("urn:uuid:" + uuid.NewString()),
			Resource: paramsJSON,
		}},
	}
	// In id-only mode each event's focus also appears as a request/response entry
	// (spec example: GET entry with response 200), still without clinical content.
	if mode == PayloadModeIDOnly {
		for _, event := range n.Events {
			if event.Focus == nil {
				continue
			}
			fullURL := event.Focus
			if event.FocusFullURL != nil {
				fullURL = event.FocusFullURL
			}
			bundle.Entry = append(bundle.Entry, fhir.BundleEntry{
				FullUrl: fullURL,
				Request: &fhir.BundleEntryRequest{
					Method: fhir.HTTPVerbGET,
					Url:    *event.Focus,
				},
				Response: &fhir.BundleEntryResponse{Status: "200"},
			})
		}
	}
	return bundle, nil
}

// ParseBundle extracts the Notification from a received notification Bundle: it locates
// the SubscriptionStatus Parameters entry and reads subscription, status, type and the
// notification-event groups. Extra entries (focus GET entries) are ignored.
func ParseBundle(bundle fhir.Bundle) (*Notification, error) {
	if bundle.Type != fhir.BundleTypeHistory {
		return nil, fmt.Errorf("notification Bundle must be of type history, got %s", bundle.Type.Code())
	}
	params, err := firstParameters(bundle)
	if err != nil {
		return nil, err
	}
	return parseStatusParameters(*params)
}

// firstParameters returns the first Parameters entry of a Bundle.
func firstParameters(bundle fhir.Bundle) (*fhir.Parameters, error) {
	for _, entry := range bundle.Entry {
		if entry.Resource == nil {
			continue
		}
		var probe struct {
			ResourceType string `json:"resourceType"`
		}
		if err := json.Unmarshal(entry.Resource, &probe); err != nil || probe.ResourceType != "Parameters" {
			continue
		}
		var p fhir.Parameters
		if err := json.Unmarshal(entry.Resource, &p); err != nil {
			return nil, fmt.Errorf("invalid SubscriptionStatus Parameters entry: %w", err)
		}
		return &p, nil
	}
	return nil, fmt.Errorf("Bundle contains no Parameters (SubscriptionStatus) entry")
}

// ParseStatusParameters parses a $status reply. Per the backport IG the reply is a
// searchset Bundle of SubscriptionStatus Parameters; a bare Parameters resource (the
// shape TTA v0.6's text describes) is accepted too.
func ParseStatusParameters(data []byte) (*Notification, error) {
	var probe struct {
		ResourceType string `json:"resourceType"`
	}
	if err := json.Unmarshal(data, &probe); err != nil {
		return nil, fmt.Errorf("invalid $status reply: %w", err)
	}
	switch probe.ResourceType {
	case "Bundle":
		var bundle fhir.Bundle
		if err := json.Unmarshal(data, &bundle); err != nil {
			return nil, fmt.Errorf("invalid Bundle: %w", err)
		}
		params, err := firstParameters(bundle)
		if err != nil {
			return nil, err
		}
		return parseStatusParameters(*params)
	case "Parameters":
		var params fhir.Parameters
		if err := json.Unmarshal(data, &params); err != nil {
			return nil, fmt.Errorf("invalid Parameters: %w", err)
		}
		return parseStatusParameters(params)
	}
	return nil, fmt.Errorf("unexpected $status reply resourceType %q", probe.ResourceType)
}

// BuildStatusBundle wraps the $status SubscriptionStatus in a searchset Bundle, the
// return shape of the backport IG's $status OperationDefinition.
func BuildStatusBundle(subscriptionRef string, status Status, eventsSinceStart int64) (fhir.Bundle, error) {
	params := BuildStatusParameters(subscriptionRef, status, eventsSinceStart)
	paramsJSON, err := json.Marshal(params)
	if err != nil {
		return fhir.Bundle{}, fmt.Errorf("marshal SubscriptionStatus parameters: %w", err)
	}
	return fhir.Bundle{
		Type:      fhir.BundleTypeSearchset,
		Timestamp: to.Ptr(time.Now().UTC().Format(time.RFC3339)),
		Total:     to.Ptr(1),
		Entry: []fhir.BundleEntry{{
			FullUrl:  to.Ptr("urn:uuid:" + uuid.NewString()),
			Resource: paramsJSON,
		}},
	}, nil
}

func parseStatusParameters(params fhir.Parameters) (*Notification, error) {
	notification := &Notification{}
	for _, param := range params.Parameter {
		switch param.Name {
		case "subscription":
			if param.ValueReference != nil && param.ValueReference.Reference != nil {
				notification.SubscriptionReference = *param.ValueReference.Reference
			}
		case "status":
			if param.ValueCode != nil {
				notification.Status = Status(*param.ValueCode)
			}
		case "type":
			if param.ValueCode != nil {
				notification.Type = NotificationType(*param.ValueCode)
			}
		case "notification-event", "events-since-subscription-start":
			if param.Name == "events-since-subscription-start" {
				// $status responses carry the counter instead of event groups; surface
				// it as a bare event so callers can read the latest number uniformly.
				if number, err := parameterInt(param); err == nil {
					notification.Events = append(notification.Events, NotificationEvent{EventNumber: number})
				}
				continue
			}
			event := NotificationEvent{}
			for _, part := range param.Part {
				switch part.Name {
				case "event-number":
					number, err := parameterInt(part)
					if err != nil {
						return nil, fmt.Errorf("invalid event-number: %w", err)
					}
					event.EventNumber = number
				case "timestamp":
					if part.ValueInstant != nil {
						if ts, err := time.Parse(time.RFC3339, *part.ValueInstant); err == nil {
							event.Timestamp = &ts
						}
					}
				case "focus":
					if part.ValueReference != nil && part.ValueReference.Reference != nil {
						event.Focus = part.ValueReference.Reference
					}
				}
			}
			notification.Events = append(notification.Events, event)
		}
	}
	if notification.SubscriptionReference == "" {
		return nil, fmt.Errorf("SubscriptionStatus is missing the subscription parameter")
	}
	if notification.Type == "" {
		return nil, fmt.Errorf("SubscriptionStatus is missing the type parameter")
	}
	return notification, nil
}

// parameterInt reads an integer-valued parameter that may be encoded as valueString (the
// TTA v0.6 examples), valueInteger, valueUnsignedInt or valuePositiveInt.
func parameterInt(param fhir.ParametersParameter) (int64, error) {
	switch {
	case param.ValueString != nil:
		return strconv.ParseInt(*param.ValueString, 10, 64)
	case param.ValueInteger != nil:
		return int64(*param.ValueInteger), nil
	case param.ValueUnsignedInt != nil:
		return int64(*param.ValueUnsignedInt), nil
	case param.ValuePositiveInt != nil:
		return int64(*param.ValuePositiveInt), nil
	}
	return 0, fmt.Errorf("parameter %s carries no integer value", param.Name)
}

// BuildStatusParameters renders the SubscriptionStatus Parameters returned by $status:
// type query-status plus the events-since-subscription-start counter.
func BuildStatusParameters(subscriptionRef string, status Status, eventsSinceStart int64) fhir.Parameters {
	return fhir.Parameters{
		Meta: &fhir.Meta{Profile: []string{ProfileSubscriptionStatusR4}},
		Parameter: []fhir.ParametersParameter{
			{Name: "subscription", ValueReference: &fhir.Reference{Reference: to.Ptr(subscriptionRef)}},
			{Name: "status", ValueCode: to.Ptr(string(status))},
			{Name: "type", ValueCode: to.Ptr(string(NotificationTypeQueryStatus))},
			{Name: "events-since-subscription-start", ValueString: to.Ptr(strconv.FormatInt(eventsSinceStart, 10))},
		},
	}
}
