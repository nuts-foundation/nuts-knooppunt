package subscription

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/nuts-foundation/nuts-knooppunt/lib/fhirsubscription"
	"github.com/nuts-foundation/nuts-knooppunt/lib/logging"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/caramel/to"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

// requestError distinguishes a malformed request (400) from an agreement/business-rule
// violation (422), per the spec's in-band creation transaction.
type requestError struct {
	Status  int
	Message string
}

func (e *requestError) Error() string { return e.Message }

func badRequest(format string, args ...any) *requestError {
	return &requestError{Status: http.StatusBadRequest, Message: fmt.Sprintf(format, args...)}
}

func unprocessable(format string, args ...any) *requestError {
	return &requestError{Status: http.StatusUnprocessableEntity, Message: fmt.Sprintf(format, args...)}
}

// selfFHIRBase is the absolute base of this knooppunt's public FHIR surface.
func (c *Component) selfFHIRBase() string {
	return strings.TrimSuffix(c.config.PublicBaseURL, "/") + "/fhir"
}

// selfSubscriptionRef is the absolute reference this server uses for a subscription in
// notifications. The spec's examples use relative references; an absolute one is what
// makes out-of-band bootstrap (receiver fetching the Subscription) possible at all — a
// finding, see FINDINGS.md.
func (c *Component) selfSubscriptionRef(id string) string {
	return c.selfFHIRBase() + "/Subscription/" + id
}

// subscriptionRefID extracts the logical id from a subscription reference, relative or absolute.
func subscriptionRefID(reference string) string {
	if idx := strings.LastIndex(reference, "/Subscription/"); idx >= 0 {
		return reference[idx+len("/Subscription/"):]
	}
	if rest, found := strings.CutPrefix(reference, "Subscription/"); found {
		return rest
	}
	return ""
}

// CreateInBandSubscription implements the public POST /Subscription transaction: validate
// against the pre-agreed topics and allowed payload modes (systems SHALL NOT accept
// arbitrary subscription requests), store as requested, kick off the handshake.
func (c *Component) CreateInBandSubscription(ctx context.Context, incoming fhirsubscription.Subscription) (fhirsubscription.Subscription, *requestError) {
	if !c.config.Server.Enabled {
		return fhirsubscription.Subscription{}, unprocessable("this knooppunt does not run the Subscription Server role")
	}
	if incoming.Status != fhirsubscription.StatusRequested {
		return fhirsubscription.Subscription{}, badRequest("Subscription.status must be requested, got %q", incoming.Status)
	}
	if incoming.Channel.Type != "rest-hook" {
		return fhirsubscription.Subscription{}, badRequest("Subscription.channel.type must be rest-hook, got %q", incoming.Channel.Type)
	}
	if incoming.Channel.Endpoint == nil || *incoming.Channel.Endpoint == "" {
		return fhirsubscription.Subscription{}, badRequest("Subscription.channel.endpoint is required")
	}
	if incoming.Channel.Payload == nil || *incoming.Channel.Payload != fhirsubscription.FHIRJSONContentType {
		return fhirsubscription.Subscription{}, badRequest("Subscription.channel.payload must be %s", fhirsubscription.FHIRJSONContentType)
	}
	topic := incoming.TopicCanonical()
	if topic == "" {
		return fhirsubscription.Subscription{}, badRequest("Subscription._criteria must carry the backport-topic-canonical extension")
	}
	if !slices.Contains(c.config.Server.AgreedTopics, topic) {
		c.store.appendLog(logEntry{Direction: "received", Type: "subscription-create", Outcome: "422 rejected", Detail: "unknown topic " + topic})
		return fhirsubscription.Subscription{}, unprocessable("SubscriptionTopic %s is not in the agreed topic list", topic)
	}
	mode := incoming.PayloadContent()
	if mode == "" {
		return fhirsubscription.Subscription{}, badRequest("Subscription.channel._payload must carry the backport-payload-content extension")
	}
	if !slices.Contains(c.config.Server.AllowedModes, string(mode)) {
		c.store.appendLog(logEntry{Direction: "received", Type: "subscription-create", Outcome: "422 rejected", PayloadMode: string(mode), Detail: "payload mode outside agreement"})
		return fhirsubscription.Subscription{}, unprocessable("payload mode %s is outside the exchange agreement", mode)
	}

	heartbeat := incoming.HeartbeatPeriod()
	if heartbeat != nil && !c.config.Server.HeartbeatSupported {
		// The spec says heartbeats are "agreed at creation" but not what happens when
		// the request asks for one the server won't send. This server strips it — the
		// created resource shows no heartbeat, so an inspecting client could notice.
		// Permutation test 4 shows what happens when the client doesn't. (Finding.)
		slog.WarnContext(ctx, "Requested heartbeat not supported by this server; stripping from subscription")
		heartbeat = nil
	}

	sub := c.createServerSubscription(ctx, serverSubscription{
		Topic:           topic,
		Filters:         incoming.FilterCriteria(),
		Mode:            mode,
		Endpoint:        *incoming.Channel.Endpoint,
		HeartbeatPeriod: heartbeat,
		CreatedVia:      "in-band",
	})
	c.store.appendLog(logEntry{Direction: "received", Type: "subscription-create", Subscription: sub.ID, PayloadMode: string(mode), Outcome: "201", Detail: "in-band, topic " + topic})
	return c.asBackportResource(sub), nil
}

// CreateOutOfBand implements source-initiated creation: this organisation's system asked
// its knooppunt (via the internal API) to set up a subscription that notifies a partner.
// The partner's notification endpoint comes from GF Adressing or configuration. The TTA
// v0.6 spec names out-of-band creation but defines no transaction for it — a finding.
func (c *Component) CreateOutOfBand(ctx context.Context, partnerURA, topic string, mode fhirsubscription.PayloadMode, heartbeatSeconds *int, endpointOverride string) (serverSubscription, *requestError) {
	if !c.config.Server.Enabled {
		return serverSubscription{}, unprocessable("this knooppunt does not run the Subscription Server role")
	}
	if !slices.Contains(c.config.Server.AgreedTopics, topic) {
		return serverSubscription{}, unprocessable("SubscriptionTopic %s is not in the agreed topic list", topic)
	}
	if mode == "" {
		mode = fhirsubscription.PayloadModeIDOnly
	}
	if !slices.Contains(c.config.Server.AllowedModes, string(mode)) {
		return serverSubscription{}, unprocessable("payload mode %s is outside the exchange agreement", mode)
	}
	if heartbeatSeconds != nil && !c.config.Server.HeartbeatSupported {
		return serverSubscription{}, unprocessable("this server does not support heartbeats")
	}
	endpoint := endpointOverride
	if endpoint == "" {
		resolved, err := c.resolveNotificationEndpoint(ctx, partnerURA)
		if err != nil {
			return serverSubscription{}, unprocessable("%v", err)
		}
		endpoint = resolved
	}

	sub := c.createServerSubscription(ctx, serverSubscription{
		Topic:           topic,
		Filters:         []string{"Task.owner=http://fhir.nl/fhir/NamingSystem/ura|" + partnerURA},
		PartnerURA:      partnerURA,
		Mode:            mode,
		Endpoint:        endpoint,
		HeartbeatPeriod: heartbeatSeconds,
		CreatedVia:      "out-of-band",
	})
	c.store.appendLog(logEntry{Direction: "sent", Type: "subscription-create", Subscription: sub.ID, PayloadMode: string(mode), Outcome: "created", Detail: "out-of-band towards URA " + partnerURA + ", endpoint " + endpoint})
	return sub, nil
}

// createServerSubscription stores the subscription as requested and starts the handshake
// goroutine.
func (c *Component) createServerSubscription(_ context.Context, sub serverSubscription) serverSubscription {
	sub.ID = uuid.NewString()
	sub.Status = fhirsubscription.StatusRequested
	sub.CreatedAt = time.Now()
	stored := sub
	c.store.mu.Lock()
	c.store.serverSubs[sub.ID] = &stored
	c.store.mu.Unlock()

	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		c.handshake(c.backgroundCtx, sub.ID)
	}()
	return sub
}

// handshake sends the handshake notification and transitions requested -> active on 200,
// or -> error after exhausted/definitive failure. The authorization hook runs before
// activation, per implementation obligation 2.
func (c *Component) handshake(ctx context.Context, subID string) {
	sub, ok := c.store.getServerSub(subID)
	if !ok {
		return
	}
	c.verifyAuthorization(ctx, sub, "activation")

	bundle, err := fhirsubscription.Notification{
		Type:                  fhirsubscription.NotificationTypeHandshake,
		SubscriptionReference: c.selfSubscriptionRef(sub.ID),
		Status:                fhirsubscription.StatusRequested,
	}.BuildBundle(sub.Mode)
	if err != nil {
		slog.ErrorContext(ctx, "Failed to build handshake bundle", logging.Error(err))
		return
	}

	result := c.deliver(ctx, sub.Endpoint, bundle)
	c.store.appendLog(logEntry{Direction: "sent", Type: "handshake", Subscription: sub.ID, PayloadMode: string(sub.Mode), Outcome: result.Outcome()})
	newStatus := fhirsubscription.StatusActive
	if result.Err != nil {
		newStatus = fhirsubscription.StatusError
	}
	c.store.updateServerSub(subID, func(s *serverSubscription) {
		s.Status = newStatus
		s.LastSentAt = time.Now()
	})
	slog.InfoContext(ctx, "Handshake completed",
		slog.String("subscription", sub.ID),
		slog.String("status", string(newStatus)),
		slog.String("outcome", result.Outcome()))
}

// matchResult reports one subscription an ingested event matched.
type matchResult struct {
	SubscriptionID string
	EventNumber    int64
	Suppressed     bool
}

// IngestEvent runs the server event pipeline for one application event: match active
// subscriptions, allocate event numbers (concurrency-safe, monotonic), append to the
// event log, build the bundle per the registered payload mode, and deliver with backoff —
// asynchronously per subscription. suppress skips delivery (test/demo facility to force
// a gap; the event number is still allocated and replayable via $events).
func (c *Component) IngestEvent(ctx context.Context, ev event, suppress bool) []matchResult {
	if ev.Interaction == "" {
		ev.Interaction = "update"
	}
	var results []matchResult
	for _, sub := range c.store.serverSubscriptionsSnapshot() {
		if sub.Status != fhirsubscription.StatusActive {
			continue
		}
		snapshot := sub
		if !c.matchesTopic(&snapshot, ev) {
			continue
		}

		// Authorization and consent verification immediately before each send
		// (implementation obligations 2 and 3).
		c.verifyAuthorization(ctx, snapshot, "notification-send")
		c.verifyConsent(ctx, snapshot)

		internalRef := ev.ResourceType + "/" + ev.ResourceID
		aliasID := c.aliaser.alias(internalRef)
		c.store.putAlias(aliasID, internalRef)
		aliasedRef := ev.ResourceType + "/" + aliasID

		record, ok := c.store.allocateEvent(sub.ID, func(number int64) eventRecord {
			rec := eventRecord{
				EventNumber: number,
				Timestamp:   time.Now(),
				Suppressed:  suppress,
			}
			if snapshot.Mode == fhirsubscription.PayloadModeIDOnly {
				rec.Focus = to.Ptr(aliasedRef)
				rec.FocusFullURL = to.Ptr(c.selfFHIRBase() + "/" + aliasedRef)
			}
			return rec
		})
		if !ok {
			continue
		}
		results = append(results, matchResult{SubscriptionID: sub.ID, EventNumber: record.EventNumber, Suppressed: suppress})

		if suppress {
			c.store.appendLog(logEntry{Direction: "sent", Type: "event", Subscription: sub.ID, EventNumber: to.Ptr(record.EventNumber), PayloadMode: string(snapshot.Mode), Outcome: "suppressed", Detail: "delivery deliberately skipped (gap simulation)"})
			continue
		}

		c.wg.Add(1)
		go func(sub serverSubscription, record eventRecord) {
			defer c.wg.Done()
			c.deliverEvent(c.backgroundCtx, sub, record)
		}(snapshot, record)
	}
	return results
}

func (c *Component) deliverEvent(ctx context.Context, sub serverSubscription, record eventRecord) {
	bundle, err := fhirsubscription.Notification{
		Type:                  fhirsubscription.NotificationTypeEvent,
		SubscriptionReference: c.selfSubscriptionRef(sub.ID),
		Status:                fhirsubscription.StatusActive,
		Events:                []fhirsubscription.NotificationEvent{eventRecordToNotification(record)},
	}.BuildBundle(sub.Mode)
	if err != nil {
		slog.ErrorContext(ctx, "Failed to build event bundle", logging.Error(err))
		return
	}
	result := c.deliver(ctx, sub.Endpoint, bundle)
	c.store.appendLog(logEntry{Direction: "sent", Type: "event", Subscription: sub.ID, EventNumber: to.Ptr(record.EventNumber), PayloadMode: string(sub.Mode), Outcome: result.Outcome()})
	if result.Err == nil {
		c.store.updateServerSub(sub.ID, func(s *serverSubscription) { s.LastSentAt = time.Now() })
	} else {
		// The event stays in the log for $events replay; the subscription stays
		// active. What status a subscription should take after persistent *event*
		// delivery failure (vs handshake failure) is unspecified — finding.
		slog.WarnContext(ctx, "Event delivery failed",
			slog.String("subscription", sub.ID),
			slog.Int64("eventNumber", record.EventNumber),
			logging.Error(result.Err))
	}
}

func eventRecordToNotification(record eventRecord) fhirsubscription.NotificationEvent {
	ts := record.Timestamp
	return fhirsubscription.NotificationEvent{
		EventNumber:  record.EventNumber,
		Timestamp:    &ts,
		Focus:        record.Focus,
		FocusFullURL: record.FocusFullURL,
	}
}

// heartbeatTick sends a heartbeat to every active subscription with an agreed heartbeat
// period whose last delivery is longer ago than that period. Called by the background
// scheduler.
func (c *Component) heartbeatTick(ctx context.Context) {
	if !c.config.Server.Enabled {
		return
	}
	now := time.Now()
	for _, sub := range c.store.serverSubscriptionsSnapshot() {
		if sub.Status != fhirsubscription.StatusActive || sub.HeartbeatPeriod == nil {
			continue
		}
		if now.Sub(sub.LastSentAt) < time.Duration(*sub.HeartbeatPeriod)*time.Second {
			continue
		}
		// Claim the slot under the lock before the (slow) delivery, so overlapping
		// ticks don't double-send.
		c.store.updateServerSub(sub.ID, func(s *serverSubscription) { s.LastSentAt = now })
		c.wg.Add(1)
		go func(sub serverSubscription) {
			defer c.wg.Done()
			bundle, err := fhirsubscription.Notification{
				Type:                  fhirsubscription.NotificationTypeHeartbeat,
				SubscriptionReference: c.selfSubscriptionRef(sub.ID),
				Status:                fhirsubscription.StatusActive,
			}.BuildBundle(sub.Mode)
			if err != nil {
				return
			}
			result := c.deliver(ctx, sub.Endpoint, bundle)
			c.store.appendLog(logEntry{Direction: "sent", Type: "heartbeat", Subscription: sub.ID, Outcome: result.Outcome()})
		}(sub)
	}
}

// ServeStatus implements the backport IG's $status: a searchset Bundle holding the
// current SubscriptionStatus Parameters (with events-since-subscription-start).
func (c *Component) ServeStatus(subID string) (fhir.Bundle, bool) {
	sub, ok := c.store.getServerSub(subID)
	if !ok {
		return fhir.Bundle{}, false
	}
	bundle, err := fhirsubscription.BuildStatusBundle(c.selfSubscriptionRef(subID), sub.Status, sub.EventCounter)
	if err != nil {
		return fhir.Bundle{}, false
	}
	c.store.appendLog(logEntry{Direction: "received", Type: "status-query", Subscription: subID, Outcome: "200"})
	return bundle, true
}

// ServeEvents implements the optional $events replay: a notification Bundle (type
// query-event) carrying the requested event range. Returns (bundle, status): 200 on
// success, 404 when the operation is disabled or the subscription unknown, 422 when the
// range is invalid or unavailable.
func (c *Component) ServeEvents(subID string, since, until *int64) (fhir.Bundle, int, string) {
	if !c.config.Server.EventsOperationEnabled {
		return fhir.Bundle{}, http.StatusNotFound, "$events is not supported by this server"
	}
	sub, ok := c.store.getServerSub(subID)
	if !ok {
		return fhir.Bundle{}, http.StatusNotFound, "unknown Subscription " + subID
	}
	from := int64(1)
	if since != nil {
		from = *since
	}
	to64 := sub.EventCounter
	if until != nil {
		to64 = *until
	}
	if from < 1 || to64 < from || to64 > sub.EventCounter {
		return fhir.Bundle{}, http.StatusUnprocessableEntity, fmt.Sprintf("invalid event range %d..%d (events 1..%d exist)", from, to64, sub.EventCounter)
	}
	records, complete := c.store.eventRange(subID, from, to64)
	if !complete {
		return fhir.Bundle{}, http.StatusUnprocessableEntity, fmt.Sprintf("event range %d..%d is no longer fully retained", from, to64)
	}
	notification := fhirsubscription.Notification{
		Type:                  fhirsubscription.NotificationTypeQueryEvent,
		SubscriptionReference: c.selfSubscriptionRef(subID),
		Status:                sub.Status,
	}
	for _, record := range records {
		notification.Events = append(notification.Events, eventRecordToNotification(record))
	}
	bundle, err := notification.BuildBundle(sub.Mode)
	if err != nil {
		return fhir.Bundle{}, http.StatusUnprocessableEntity, err.Error()
	}
	c.store.appendLog(logEntry{Direction: "received", Type: "events-query", Subscription: subID, Outcome: "200", Detail: fmt.Sprintf("replayed %d..%d", from, to64)})
	return bundle, http.StatusOK, ""
}

// asBackportResource renders a stored server subscription as the backport FHIR resource.
func (c *Component) asBackportResource(sub serverSubscription) fhirsubscription.Subscription {
	resource := fhirsubscription.Build(fhirsubscription.BuildOptions{
		TopicCanonical:  sub.Topic,
		FilterCriteria:  sub.Filters,
		Endpoint:        sub.Endpoint,
		Mode:            sub.Mode,
		HeartbeatPeriod: sub.HeartbeatPeriod,
	})
	resource.ID = to.Ptr(sub.ID)
	resource.Status = sub.Status
	return resource
}
