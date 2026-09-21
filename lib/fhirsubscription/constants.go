// Package fhirsubscription provides hand-written Go types for the HL7 FHIR R4/R4B
// Subscriptions R5 Backport IG (https://hl7.org/fhir/uv/subscriptions-backport/), which
// TTA Notifications v0.6 conforms to. The R4-only zorgbijjou/golang-fhir-models library
// cannot express the backport's primitive extensions on Subscription.criteria and
// Subscription.channel.payload, and has no SubscriptionTopic or SubscriptionStatus at
// all, so the wire shapes are implemented here (precedent: component/mitz/xacml/types.go).
package fhirsubscription

// Extension and profile URLs from the Subscriptions R5 Backport IG.
const (
	// ExtTopicCanonical is the extension on Subscription._criteria carrying the canonical
	// URL of the SubscriptionTopic the subscription is for.
	ExtTopicCanonical = "http://hl7.org/fhir/uv/subscriptions-backport/StructureDefinition/backport-topic-canonical"
	// ExtFilterCriteria is the extension on Subscription._criteria carrying a scoping
	// filter (search-parameter style) narrowing which events are delivered.
	ExtFilterCriteria = "http://hl7.org/fhir/uv/subscriptions-backport/StructureDefinition/backport-filter-criteria"
	// ExtPayloadContent is the extension on Subscription.channel._payload carrying the
	// payload mode: empty, id-only or full-resource.
	ExtPayloadContent = "http://hl7.org/fhir/uv/subscriptions-backport/StructureDefinition/backport-payload-content"
	// ExtHeartbeatPeriod is the extension on Subscription.channel carrying the agreed
	// heartbeat interval in seconds.
	ExtHeartbeatPeriod = "http://hl7.org/fhir/uv/subscriptions-backport/StructureDefinition/backport-heartbeat-period"

	// ProfileNotificationR4 is the profile the notification Bundle declares.
	ProfileNotificationR4 = "http://hl7.org/fhir/uv/subscriptions-backport/StructureDefinition/backport-subscription-notification-r4"
	// ProfileSubscriptionStatusR4 is the profile the SubscriptionStatus Parameters declares.
	ProfileSubscriptionStatusR4 = "http://hl7.org/fhir/uv/subscriptions-backport/StructureDefinition/backport-subscription-status-r4"
)

// PayloadMode is the backport-payload-content code: what a notification carries.
type PayloadMode string

const (
	PayloadModeEmpty        PayloadMode = "empty"
	PayloadModeIDOnly       PayloadMode = "id-only"
	PayloadModeFullResource PayloadMode = "full-resource"
)

// NotificationType is the SubscriptionStatus "type" parameter value.
type NotificationType string

const (
	NotificationTypeHandshake NotificationType = "handshake-notification"
	NotificationTypeHeartbeat NotificationType = "heartbeat-notification"
	NotificationTypeEvent     NotificationType = "event-notification"
	// NotificationTypeQueryStatus is used in $status responses (backport IG code query-status).
	NotificationTypeQueryStatus NotificationType = "query-status"
	// NotificationTypeQueryEvent is used in $events responses (backport IG code query-event).
	NotificationTypeQueryEvent NotificationType = "query-event"
)

// Status is the R4 Subscription.status code, reused for the backport lifecycle:
// requested -> (handshake) -> active; retirement is off (not delete); persistent
// delivery failure is error.
type Status string

const (
	StatusRequested Status = "requested"
	StatusActive    Status = "active"
	StatusError     Status = "error"
	StatusOff       Status = "off"
)

// FHIRJSONContentType is the channel payload MIME type mandated by TTA Notifications v0.6.
const FHIRJSONContentType = "application/fhir+json"
