package fhirsubscription

import (
	"encoding/json"
	"fmt"
)

// SubscriptionTopic is a minimal R4B/R5-format SubscriptionTopic. Per the backport IG a
// topic is a design-time contract, not a runtime object: on the wire only its canonical
// URL appears (in Subscription._criteria). This type exists so the agreed topic can be
// loaded from a JSON artifact and served read-only for discoverability; trigger logic is
// implemented manually from the published definition, not evaluated generically.
type SubscriptionTopic struct {
	ID              *string                `json:"id,omitempty"`
	URL             string                 `json:"url"`
	Version         *string                `json:"version,omitempty"`
	Title           *string                `json:"title,omitempty"`
	Status          string                 `json:"status"`
	Description     *string                `json:"description,omitempty"`
	ResourceTrigger []TopicResourceTrigger `json:"resourceTrigger,omitempty"`
	CanFilterBy     []TopicCanFilterBy     `json:"canFilterBy,omitempty"`
}

type TopicResourceTrigger struct {
	Description          *string             `json:"description,omitempty"`
	Resource             string              `json:"resource"`
	SupportedInteraction []string            `json:"supportedInteraction,omitempty"`
	QueryCriteria        *TopicQueryCriteria `json:"queryCriteria,omitempty"`
}

type TopicQueryCriteria struct {
	Current         *string `json:"current,omitempty"`
	ResultForCreate *string `json:"resultForCreate,omitempty"`
}

type TopicCanFilterBy struct {
	Description     *string  `json:"description,omitempty"`
	Resource        *string  `json:"resource,omitempty"`
	FilterParameter string   `json:"filterParameter"`
	Modifier        []string `json:"modifier,omitempty"`
}

type otherSubscriptionTopic SubscriptionTopic

// MarshalJSON adds the resourceType discriminator.
func (t SubscriptionTopic) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		otherSubscriptionTopic
		ResourceType string `json:"resourceType"`
	}{otherSubscriptionTopic: otherSubscriptionTopic(t), ResourceType: "SubscriptionTopic"})
}

// UnmarshalSubscriptionTopic parses raw JSON into a SubscriptionTopic, verifying resourceType.
func UnmarshalSubscriptionTopic(data []byte) (SubscriptionTopic, error) {
	var probe struct {
		ResourceType string `json:"resourceType"`
	}
	if err := json.Unmarshal(data, &probe); err != nil {
		return SubscriptionTopic{}, fmt.Errorf("invalid JSON: %w", err)
	}
	if probe.ResourceType != "SubscriptionTopic" {
		return SubscriptionTopic{}, fmt.Errorf("expected resourceType SubscriptionTopic, got %q", probe.ResourceType)
	}
	var t otherSubscriptionTopic
	if err := json.Unmarshal(data, &t); err != nil {
		return SubscriptionTopic{}, fmt.Errorf("invalid SubscriptionTopic: %w", err)
	}
	return SubscriptionTopic(t), nil
}
