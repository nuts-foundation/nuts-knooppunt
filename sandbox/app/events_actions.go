package main

import (
	"context"
	"net/http"
	"strings"

	"github.com/nuts-foundation/nuts-knooppunt/test/testdata/vectors/nvi"
)

type eventActionKey struct{}
type eventPurposeKey struct{}
type eventResourceTypeKey struct{}
type eventAction struct{ id, kind string }

func withEventAction(ctx context.Context, kind string) context.Context {
	if !eventEnum(kind, "record", "share-preview", "share", "subscribe", "discover", "retrieve", "authorize") {
		return ctx
	}
	return context.WithValue(ctx, eventActionKey{}, eventAction{id: randomURLSafe(), kind: kind})
}

func withEventResourceType(ctx context.Context, resourceType string) context.Context {
	if !eventEnum(resourceType, "Patient", "Condition", "MedicationRequest", "AllergyIntolerance") {
		return ctx
	}
	return context.WithValue(ctx, eventResourceTypeKey{}, resourceType)
}

func withEventPurpose(ctx context.Context, purpose string) context.Context {
	if !eventEnum(purpose, "status", "registration-cleanup", "registration", "subscription-check", "subscription", "localization", "addressing", "authorization", "exchange", "pseudonymization") {
		return ctx
	}
	return context.WithValue(ctx, eventPurposeKey{}, purpose)
}

func actionForRequest(r *http.Request) string {
	switch {
	case strings.HasSuffix(r.URL.Path, "/share"):
		if r.Method == http.MethodGet {
			return "share-preview"
		}
		return "share"
	case strings.HasSuffix(r.URL.Path, "/subscribe"):
		return "subscribe"
	case strings.HasSuffix(r.URL.Path, "/retrieve"):
		if r.Method == http.MethodGet {
			return "discover"
		}
		return "retrieve"
	default:
		return "record"
	}
}

func applyEventAction(ctx context.Context, event *stepEvent) {
	if action, ok := ctx.Value(eventActionKey{}).(eventAction); ok {
		event.ActionID, event.Action = action.id, action.kind
	}
	event.CallID = randomURLSafe()
	if event.GF == "exchange" {
		event.ResourceType, _ = ctx.Value(eventResourceTypeKey{}).(string)
	}
	event.Purpose, _ = ctx.Value(eventPurposeKey{}).(string)
	if event.Purpose == "" {
		switch event.GF {
		case "consent":
			event.Purpose = "subscription"
			if event.Request.Method == http.MethodGet {
				event.Purpose = "subscription-check"
			}
		case "authentication", "authorization":
			event.Purpose = "authorization"
		default:
			event.Purpose = event.GF
		}
	}
	if event.Purpose == "registration" && (event.Request.Method == http.MethodDelete || strings.HasSuffix(event.Request.Path, "/_search")) {
		event.Purpose = "registration-cleanup"
	}
	category := nvi.RegistrationCategory(ctx)
	if event.Purpose == "registration" && eventEnum(category, "Patient", "Condition", "MedicationRequest", "AllergyIntolerance") {
		event.ResourceType = category
	}
}
