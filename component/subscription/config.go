package subscription

import (
	"time"

	"github.com/nuts-foundation/nuts-knooppunt/lib/fhirsubscription"
)

// Config configures the TTA Notifications v0.6 POC component. One knooppunt can run the
// Subscription Server role, the Subscription Client role, or both — exactly what the
// permutation tests need, and how the spec assigns roles per organisation.
type Config struct {
	Server ServerConfig `koanf:"server"`
	Client ClientConfig `koanf:"client"`
	// PublicBaseURL is the externally reachable base of this knooppunt's public
	// interface (e.g. http://knooppunt:8080). Defaults to the HTTP component's public
	// interface URL (filled in by cmd.Start); the public FHIR surface lives under
	// {PublicBaseURL}/fhir.
	PublicBaseURL string `koanf:"publicbaseurl"`
}

func (c Config) Enabled() bool {
	return c.Server.Enabled || c.Client.Enabled
}

// PartnerConfig is per-partner endpoint configuration, the fallback when the mCSD Query
// Directory has no endpoint of the right payload type for the partner (research question 9).
type PartnerConfig struct {
	// NotificationEndpoint is where notifications for this partner are delivered
	// (Subscription Server role).
	NotificationEndpoint string `koanf:"notificationendpoint"`
	// FHIRBaseURL is the partner's public FHIR base — where POST /Subscription,
	// $status and $events live (Subscription Client role).
	FHIRBaseURL string `koanf:"fhirbaseurl"`
}

type ServerConfig struct {
	Enabled bool `koanf:"enabled"`
	// URA identifies the organisation this server sends on behalf of.
	URA string `koanf:"ura"`
	// FHIRBaseURL is the organisation's FHIR store (HAPI tenant). Used by the Task
	// _history poller and by the de-aliased Task read for the follow-up pull. Optional:
	// without it, events only enter via POST /subscription/events.
	FHIRBaseURL string `koanf:"fhirbaseurl"`
	// PollIntervalSeconds enables the Task _history poller when > 0.
	PollIntervalSeconds int `koanf:"pollintervalseconds"`
	// AliasKey is the HMAC key for the id-only alias layer (research question 8). A
	// random key is generated when empty — fine for a POC, but aliases then don't
	// survive restarts (finding: alias stability requires durable key management).
	AliasKey string `koanf:"aliaskey"`
	// AgreedTopics is the pre-agreed SubscriptionTopic canonical list (spec
	// precondition 1). Defaults to the built-in eOverdracht Task topic.
	AgreedTopics []string `koanf:"agreedtopics"`
	// AllowedModes are the payload modes the exchange agreement permits. Defaults to
	// empty and id-only; full-resource is excluded from the POC by assignment.
	AllowedModes []string `koanf:"allowedmodes"`
	// HeartbeatSupported controls whether this server honours heartbeat agreements.
	// When false, a requested backport-heartbeat-period is stripped at creation
	// (permutation test 4).
	HeartbeatSupported bool `koanf:"heartbeatsupported"`
	// EventsOperationEnabled controls the optional $events replay operation
	// (permutation tests 2 and 5: disabled servers return 404).
	EventsOperationEnabled bool `koanf:"eventsoperation"`
	// Partners maps partner URA to endpoint fallbacks.
	Partners map[string]PartnerConfig `koanf:"partners"`
	// QueryDirectoryFHIRBaseURL is the mCSD Query Directory used to resolve partner
	// notification endpoints by payload type (GF Adressing, research question 9).
	QueryDirectoryFHIRBaseURL string `koanf:"querydirectoryfhirbaseurl"`
	// RetryBaseDelayMillis is the first exponential-backoff delay for notification
	// delivery retries.
	RetryBaseDelayMillis int `koanf:"retrybasedelaymillis"`
	// RetryMaxAttempts bounds delivery attempts; exhausting them on a handshake sets
	// the subscription to error.
	RetryMaxAttempts int `koanf:"retrymaxattempts"`
}

type ClientConfig struct {
	Enabled bool `koanf:"enabled"`
	// URA identifies the receiving organisation.
	URA string `koanf:"ura"`
	// NotificationEndpoint is this client's own public rest-hook endpoint, put in
	// Subscription.channel.endpoint on in-band creation. Defaults to
	// {PublicBaseURL}/fhir/notifications.
	NotificationEndpoint string `koanf:"notificationendpoint"`
	// AgreedTopics is the pre-agreed topic list this client accepts subscriptions for.
	AgreedTopics []string `koanf:"agreedtopics"`
	// AllowedModes are the payload modes within the exchange agreement. An out-of-band
	// subscription with a mode outside this list gets its handshake rejected
	// (permutation test 6).
	AllowedModes []string `koanf:"allowedmodes"`
	// HeartbeatMissCheck enables the missed-heartbeat timer (~1.5x the agreed period;
	// on miss the client queries $status). Disabled = the client ignores heartbeats
	// (permutation test 3).
	HeartbeatMissCheck bool `koanf:"heartbeatmisscheck"`
	// Partners maps partner URA to endpoint fallbacks.
	Partners map[string]PartnerConfig `koanf:"partners"`
	// QueryDirectoryFHIRBaseURL is the mCSD Query Directory used to resolve partner
	// Subscription endpoints (research question 9).
	QueryDirectoryFHIRBaseURL string `koanf:"querydirectoryfhirbaseurl"`
}

// TickIntervalMillis paces the background scheduler that sends due heartbeats (server)
// and detects missed heartbeats (client). Not exposed as configuration; tests shorten it.
var defaultTickInterval = time.Second

func DefaultConfig() Config {
	return Config{
		Server: ServerConfig{
			AgreedTopics:           []string{TaskTopicCanonical},
			AllowedModes:           []string{string(fhirsubscription.PayloadModeEmpty), string(fhirsubscription.PayloadModeIDOnly)},
			HeartbeatSupported:     true,
			EventsOperationEnabled: true,
			RetryBaseDelayMillis:   500,
			RetryMaxAttempts:       5,
		},
		Client: ClientConfig{
			AgreedTopics:       []string{TaskTopicCanonical},
			AllowedModes:       []string{string(fhirsubscription.PayloadModeEmpty), string(fhirsubscription.PayloadModeIDOnly)},
			HeartbeatMissCheck: true,
		},
	}
}
