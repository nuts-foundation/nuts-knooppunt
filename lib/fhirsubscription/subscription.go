package fhirsubscription

import (
	"encoding/json"
	"fmt"

	"github.com/zorgbijjou/golang-fhir-models/fhir-models/caramel/to"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

// Element is a FHIR primitive-element companion (the "_criteria"/"_payload" JSON shape):
// extensions attached to a primitive field.
type Element struct {
	Extension []fhir.Extension `json:"extension,omitempty"`
}

// Channel is Subscription.channel with the backport additions: the payload-content
// primitive extension on payload, and the heartbeat-period extension on the channel itself.
type Channel struct {
	Extension      []fhir.Extension `json:"extension,omitempty"`
	Type           string           `json:"type"`
	Endpoint       *string          `json:"endpoint,omitempty"`
	Payload        *string          `json:"payload,omitempty"`
	PayloadElement *Element         `json:"_payload,omitempty"`
	Header         []string         `json:"header,omitempty"`
}

// Subscription is an R4 Subscription profiled per the Subscriptions R5 Backport IG:
// criteria carries the backport-topic-canonical and backport-filter-criteria extensions
// in _criteria, channel.payload carries backport-payload-content in _payload.
type Subscription struct {
	ID              *string          `json:"id,omitempty"`
	Meta            *fhir.Meta       `json:"meta,omitempty"`
	Extension       []fhir.Extension `json:"extension,omitempty"`
	Status          Status           `json:"status"`
	End             *string          `json:"end,omitempty"`
	Reason          string           `json:"reason"`
	Criteria        string           `json:"criteria"`
	CriteriaElement *Element         `json:"_criteria,omitempty"`
	Error           *string          `json:"error,omitempty"`
	Channel         Channel          `json:"channel"`
}

type otherSubscription Subscription

// MarshalJSON adds the resourceType discriminator, matching the generated fhir-models style.
func (s Subscription) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		otherSubscription
		ResourceType string `json:"resourceType"`
	}{otherSubscription: otherSubscription(s), ResourceType: "Subscription"})
}

// UnmarshalSubscription parses raw JSON into a Subscription, verifying resourceType.
func UnmarshalSubscription(data []byte) (Subscription, error) {
	var probe struct {
		ResourceType string `json:"resourceType"`
	}
	if err := json.Unmarshal(data, &probe); err != nil {
		return Subscription{}, fmt.Errorf("invalid JSON: %w", err)
	}
	if probe.ResourceType != "Subscription" {
		return Subscription{}, fmt.Errorf("expected resourceType Subscription, got %q", probe.ResourceType)
	}
	var s otherSubscription
	if err := json.Unmarshal(data, &s); err != nil {
		return Subscription{}, fmt.Errorf("invalid Subscription: %w", err)
	}
	return Subscription(s), nil
}

// TopicCanonical returns the backport-topic-canonical extension value, or "".
func (s Subscription) TopicCanonical() string {
	if s.CriteriaElement == nil {
		return ""
	}
	for _, ext := range s.CriteriaElement.Extension {
		if ext.Url == ExtTopicCanonical && ext.ValueCanonical != nil {
			return *ext.ValueCanonical
		}
	}
	return ""
}

// FilterCriteria returns all backport-filter-criteria extension values.
func (s Subscription) FilterCriteria() []string {
	if s.CriteriaElement == nil {
		return nil
	}
	var filters []string
	for _, ext := range s.CriteriaElement.Extension {
		if ext.Url == ExtFilterCriteria && ext.ValueString != nil {
			filters = append(filters, *ext.ValueString)
		}
	}
	return filters
}

// PayloadContent returns the backport-payload-content mode, or "" when absent.
func (s Subscription) PayloadContent() PayloadMode {
	if s.Channel.PayloadElement == nil {
		return ""
	}
	for _, ext := range s.Channel.PayloadElement.Extension {
		if ext.Url == ExtPayloadContent && ext.ValueCode != nil {
			return PayloadMode(*ext.ValueCode)
		}
	}
	return ""
}

// HeartbeatPeriod returns the agreed heartbeat interval in seconds, or nil when no
// heartbeat was agreed.
func (s Subscription) HeartbeatPeriod() *int {
	for _, ext := range s.Channel.Extension {
		if ext.Url == ExtHeartbeatPeriod {
			if ext.ValueUnsignedInt != nil {
				return ext.ValueUnsignedInt
			}
			if ext.ValuePositiveInt != nil {
				return ext.ValuePositiveInt
			}
		}
	}
	return nil
}

// SetHeartbeatPeriod adds (or removes, when nil) the backport-heartbeat-period channel extension.
func (s *Subscription) SetHeartbeatPeriod(seconds *int) {
	kept := s.Channel.Extension[:0]
	for _, ext := range s.Channel.Extension {
		if ext.Url != ExtHeartbeatPeriod {
			kept = append(kept, ext)
		}
	}
	s.Channel.Extension = kept
	if seconds != nil {
		s.Channel.Extension = append(s.Channel.Extension, fhir.Extension{
			Url:              ExtHeartbeatPeriod,
			ValueUnsignedInt: seconds,
		})
	}
}

// BuildOptions carries everything needed to construct a spec-compliant backport Subscription.
type BuildOptions struct {
	TopicCanonical  string
	FilterCriteria  []string
	Endpoint        string
	Mode            PayloadMode
	HeartbeatPeriod *int // seconds; nil = no heartbeat agreed
	Reason          string
}

// Build constructs a Subscription with status requested, per the TTA Notifications v0.6
// in-band creation shape (rest-hook channel, application/fhir+json payload, backport
// extensions on _criteria and _payload).
func Build(opts BuildOptions) Subscription {
	reason := opts.Reason
	if reason == "" {
		reason = "TTA Notifications v0.6"
	}
	sub := Subscription{
		Status:   StatusRequested,
		Reason:   reason,
		Criteria: opts.TopicCanonical,
		CriteriaElement: &Element{
			Extension: []fhir.Extension{{
				Url:            ExtTopicCanonical,
				ValueCanonical: to.Ptr(opts.TopicCanonical),
			}},
		},
		Channel: Channel{
			Type:     "rest-hook",
			Endpoint: to.Ptr(opts.Endpoint),
			Payload:  to.Ptr(FHIRJSONContentType),
			PayloadElement: &Element{
				Extension: []fhir.Extension{{
					Url:       ExtPayloadContent,
					ValueCode: to.Ptr(string(opts.Mode)),
				}},
			},
		},
	}
	for _, filter := range opts.FilterCriteria {
		sub.CriteriaElement.Extension = append(sub.CriteriaElement.Extension, fhir.Extension{
			Url:         ExtFilterCriteria,
			ValueString: to.Ptr(filter),
		})
	}
	sub.SetHeartbeatPeriod(opts.HeartbeatPeriod)
	return sub
}
