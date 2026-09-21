package subscription

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/nuts-foundation/nuts-knooppunt/lib/fhirsubscription"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/caramel/to"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

// notificationEndpoint is this client's own public rest-hook endpoint.
func (c *Component) notificationEndpoint() string {
	if c.config.Client.NotificationEndpoint != "" {
		return c.config.Client.NotificationEndpoint
	}
	return c.selfFHIRBase() + "/notifications"
}

// CreateIntent implements the client side of in-band creation: one internal call
// replaces the whole transaction. The knooppunt resolves the partner's Subscription
// endpoint, builds the backport Subscription (pre-approved topic and mode only, per the
// spec's agreement enforcement), POSTs it and tracks the result.
func (c *Component) CreateIntent(ctx context.Context, partnerURA, topic string, mode fhirsubscription.PayloadMode, heartbeatSeconds *int, baseOverride string) (clientSubscription, *requestError) {
	if !c.config.Client.Enabled {
		return clientSubscription{}, unprocessable("this knooppunt does not run the Subscription Client role")
	}
	if !slices.Contains(c.config.Client.AgreedTopics, topic) {
		return clientSubscription{}, unprocessable("SubscriptionTopic %s is not in the agreed topic list", topic)
	}
	if mode == "" {
		mode = fhirsubscription.PayloadModeIDOnly
	}
	if !slices.Contains(c.config.Client.AllowedModes, string(mode)) {
		return clientSubscription{}, unprocessable("payload mode %s is outside the exchange agreement", mode)
	}
	base := baseOverride
	if base == "" {
		resolved, err := c.resolveSubscriptionBase(ctx, partnerURA)
		if err != nil {
			return clientSubscription{}, unprocessable("%v", err)
		}
		base = resolved
	}
	base = strings.TrimSuffix(base, "/")

	// The COW profile's interest direction: a broad, long-lived per-partner
	// subscription, filtered on Task.owner = this (receiving) organisation.
	resource := fhirsubscription.Build(fhirsubscription.BuildOptions{
		TopicCanonical:  topic,
		FilterCriteria:  []string{"Task.owner=http://fhir.nl/fhir/NamingSystem/ura|" + c.config.Client.URA},
		Endpoint:        c.notificationEndpoint(),
		Mode:            mode,
		HeartbeatPeriod: heartbeatSeconds,
	})

	created, status, err := c.postSubscription(ctx, base+"/Subscription", resource)
	outcome := strconv.Itoa(status)
	if err != nil {
		outcome = err.Error()
	}
	c.store.appendLog(logEntry{Direction: "sent", Type: "subscription-create", PayloadMode: string(mode), Outcome: outcome, Detail: "in-band at URA " + partnerURA + " (" + base + ")"})
	if err != nil {
		return clientSubscription{}, unprocessable("subscription creation at partner failed: %v", err)
	}
	if created.ID == nil {
		return clientSubscription{}, unprocessable("partner returned a Subscription without an id")
	}

	sub := &clientSubscription{
		Reference:          base + "/Subscription/" + *created.ID,
		ID:                 *created.ID,
		Topic:              topic,
		PartnerURA:         partnerURA,
		Mode:               mode,
		ServerFHIRBase:     base,
		HeartbeatPeriod:    heartbeatSeconds,
		Status:             fhirsubscription.StatusRequested,
		Origin:             "in-band",
		Processed:          map[int64]bool{},
		LastNotificationAt: time.Now(),
	}
	return c.store.upsertIntentClientSub(sub), nil
}

// HandleNotification processes an incoming notification Bundle at the client endpoint:
// classify, always log, then handshake/heartbeat/event handling. Returns the HTTP status
// (200/400/422) and, for errors, an OperationOutcome message.
func (c *Component) HandleNotification(ctx context.Context, bundle fhir.Bundle) (int, string) {
	notification, err := fhirsubscription.ParseBundle(bundle)
	if err != nil {
		c.store.appendLog(logEntry{Direction: "received", Type: "notification", Outcome: "400 rejected", Detail: err.Error()})
		return http.StatusBadRequest, err.Error()
	}
	if !c.config.Client.Enabled {
		c.store.appendLog(logEntry{Direction: "received", Type: shortType(notification.Type), Subscription: notification.SubscriptionReference, Outcome: "422 rejected", Detail: "client role disabled"})
		return http.StatusUnprocessableEntity, "this knooppunt does not run the Subscription Client role"
	}

	sub, known := c.store.getClientSub(notification.SubscriptionReference)
	if !known {
		bootstrapped, reqErr := c.bootstrapUnknownSubscription(ctx, notification)
		if reqErr != nil {
			c.store.appendLog(logEntry{Direction: "received", Type: shortType(notification.Type), Subscription: notification.SubscriptionReference, Outcome: fmt.Sprintf("%d rejected", reqErr.Status), Detail: reqErr.Message})
			return reqErr.Status, reqErr.Message
		}
		sub = bootstrapped
	}

	switch notification.Type {
	case fhirsubscription.NotificationTypeHandshake:
		c.store.withLock(func() {
			sub.Status = fhirsubscription.StatusActive
			sub.LastNotificationAt = time.Now()
		})
		c.store.appendLog(logEntry{Direction: "received", Type: "handshake", Subscription: sub.Reference, Outcome: "200"})
		slog.InfoContext(ctx, "Handshake accepted", slog.String("subscription", sub.Reference))
		return http.StatusOK, ""

	case fhirsubscription.NotificationTypeHeartbeat:
		c.store.withLock(func() { sub.LastNotificationAt = time.Now() })
		// Logged and timer reset even when miss detection is disabled: an unexpected
		// heartbeat is harmless (permutation test 3).
		c.store.appendLog(logEntry{Direction: "received", Type: "heartbeat", Subscription: sub.Reference, Outcome: "200"})
		return http.StatusOK, ""

	case fhirsubscription.NotificationTypeEvent:
		for _, ev := range notification.Events {
			c.processEvent(ctx, sub, ev, false)
		}
		return http.StatusOK, ""

	default:
		c.store.appendLog(logEntry{Direction: "received", Type: shortType(notification.Type), Subscription: sub.Reference, Outcome: "400 rejected", Detail: "unsupported notification type"})
		return http.StatusBadRequest, fmt.Sprintf("unsupported notification type %q", notification.Type)
	}
}

// shortType maps a wire notification type to the transport log's short type name.
func shortType(notificationType fhirsubscription.NotificationType) string {
	switch notificationType {
	case fhirsubscription.NotificationTypeHandshake:
		return "handshake"
	case fhirsubscription.NotificationTypeHeartbeat:
		return "heartbeat"
	case fhirsubscription.NotificationTypeEvent:
		return "event"
	}
	return string(notificationType)
}

// bootstrapUnknownSubscription handles a notification for a subscription this client
// never created: the out-of-band case. Per spec, the receiver MAY inspect the payload
// mode by fetching the Subscription resource; if the mode is outside the agreement the
// receiver SHALL reject the handshake. That fetch is only possible because this
// implementation puts an *absolute* subscription reference in its notifications — with
// the spec's relative example form there is nothing to dereference (finding).
func (c *Component) bootstrapUnknownSubscription(ctx context.Context, notification *fhirsubscription.Notification) (*clientSubscription, *requestError) {
	reference := notification.SubscriptionReference
	if !strings.HasPrefix(reference, "http://") && !strings.HasPrefix(reference, "https://") {
		return nil, unprocessable("unknown subscription %s: reference is not resolvable (out-of-band bootstrap requires an absolute reference)", reference)
	}
	resource, err := c.fetchSubscription(ctx, reference)
	c.store.appendLog(logEntry{Direction: "sent", Type: "subscription-fetch", Subscription: reference, Outcome: outcomeForErr(err)})
	if err != nil {
		return nil, unprocessable("unknown subscription %s: fetching it failed: %v", reference, err)
	}

	topic := resource.TopicCanonical()
	mode := resource.PayloadContent()
	if !slices.Contains(c.config.Client.AgreedTopics, topic) {
		return nil, unprocessable("subscription %s topic %s is not in the agreed topic list — handshake refused", reference, topic)
	}
	if !slices.Contains(c.config.Client.AllowedModes, string(mode)) {
		// Permutation test 6: mode outside the agreement -> refuse the handshake.
		return nil, unprocessable("subscription %s payload mode %s is outside the exchange agreement — handshake refused", reference, mode)
	}

	base := reference
	if idx := strings.LastIndex(reference, "/Subscription/"); idx >= 0 {
		base = reference[:idx]
	}
	sub := c.store.getOrPutClientSub(&clientSubscription{
		Reference:          reference,
		ID:                 subscriptionRefID(reference),
		Topic:              topic,
		Mode:               mode,
		ServerFHIRBase:     base,
		HeartbeatPeriod:    resource.HeartbeatPeriod(),
		Status:             fhirsubscription.StatusRequested,
		Origin:             "out-of-band",
		Processed:          map[int64]bool{},
		LastNotificationAt: time.Now(),
	})
	slog.InfoContext(ctx, "Bootstrapped out-of-band subscription",
		slog.String("subscription", reference),
		slog.String("topic", topic),
		slog.String("mode", string(mode)))
	return sub, nil
}

// processEvent applies continuity checking, idempotent processing and inbox
// normalisation to one notification-event. viaRecovery marks events replayed by
// $events, which must not trigger recursive gap recovery.
func (c *Component) processEvent(ctx context.Context, sub *clientSubscription, ev fhirsubscription.NotificationEvent, viaRecovery bool) {
	var duplicate bool
	var gapFrom, gapTo int64
	var topic string
	var mode fhirsubscription.PayloadMode
	c.store.withLock(func() {
		sub.LastNotificationAt = time.Now()
		topic, mode = sub.Topic, sub.Mode
		if sub.Processed[ev.EventNumber] {
			duplicate = true
			return
		}
		if ev.EventNumber > sub.HighestEvent+1 {
			gapFrom, gapTo = sub.HighestEvent+1, ev.EventNumber-1
		}
	})

	if duplicate {
		// Idempotency: re-delivery is acknowledged but causes no duplicate side
		// effects (implementation obligation 6).
		c.store.appendLog(logEntry{Direction: "received", Type: "event", Subscription: sub.Reference, EventNumber: to.Ptr(ev.EventNumber), Outcome: "200 duplicate", Detail: "already processed, ignored"})
		return
	}

	if gapFrom > 0 && !viaRecovery {
		c.recoverGap(ctx, sub, gapFrom, gapTo)
	}

	c.store.withLock(func() {
		sub.Processed[ev.EventNumber] = true
		if ev.EventNumber > sub.HighestEvent {
			sub.HighestEvent = ev.EventNumber
		}
	})
	entry := inboxEntry{
		Type:                  "event",
		SubscriptionReference: sub.Reference,
		Topic:                 topic,
		EventNumber:           to.Ptr(ev.EventNumber),
		Focus:                 ev.Focus,
	}
	c.store.appendInbox(entry)
	outcome := "200"
	if viaRecovery {
		outcome = "200 recovered"
	}
	c.store.appendLog(logEntry{Direction: "received", Type: "event", Subscription: sub.Reference, EventNumber: to.Ptr(ev.EventNumber), PayloadMode: string(mode), Outcome: outcome})
}

// recoverGap tries to close a detected event-number gap: first via the server's optional
// $events operation, then degrading to $status plus a `gap` inbox entry telling the
// vendor a resync is needed (the empty-mode / no-$events path: "full re-pull").
func (c *Component) recoverGap(ctx context.Context, sub *clientSubscription, from, until int64) {
	slog.WarnContext(ctx, "Event-number gap detected",
		slog.String("subscription", sub.Reference),
		slog.Int64("from", from),
		slog.Int64("until", until))

	if sub.ServerFHIRBase != "" {
		// Parameter names per the backport IG's $events OperationDefinition.
		eventsURL := fmt.Sprintf("%s/Subscription/%s/$events?eventsSinceNumber=%d&eventsUntilNumber=%d",
			sub.ServerFHIRBase, url.PathEscape(sub.ID), from, until)
		body, status, err := c.getJSON(ctx, eventsURL)
		c.store.appendLog(logEntry{Direction: "sent", Type: "events-query", Subscription: sub.Reference, Outcome: outcomeForStatus(status, err), Detail: fmt.Sprintf("gap recovery %d..%d", from, until)})
		if err == nil && status == http.StatusOK {
			var bundle fhir.Bundle
			if err := json.Unmarshal(body, &bundle); err == nil {
				if replayed, err := fhirsubscription.ParseBundle(bundle); err == nil {
					for _, ev := range replayed.Events {
						c.processEvent(ctx, sub, ev, true)
					}
					slog.InfoContext(ctx, "Gap recovered via $events",
						slog.String("subscription", sub.Reference),
						slog.Int("events", len(replayed.Events)))
					return
				}
			}
		}
	}

	// Degraded path: learn the server's latest state via $status, then surface the
	// unrecoverable range to the vendor and move on.
	c.queryStatus(ctx, sub, "gap-recovery")
	var topic string
	c.store.withLock(func() {
		topic = sub.Topic
		for n := from; n <= until; n++ {
			sub.Processed[n] = true
		}
		if until > sub.HighestEvent {
			sub.HighestEvent = until
		}
	})
	c.store.appendInbox(inboxEntry{
		Type:                  "gap",
		SubscriptionReference: sub.Reference,
		Topic:                 topic,
		GapFrom:               to.Ptr(from),
		GapTo:                 to.Ptr(until),
	})
	c.store.appendLog(logEntry{Direction: "received", Type: "event", Subscription: sub.Reference, Outcome: "gap unrecovered", Detail: fmt.Sprintf("events %d..%d lost; vendor notified to resync", from, until)})
}

// queryStatus calls the server's $status and logs the result. reason is what triggered
// it (heartbeat-miss or gap-recovery).
func (c *Component) queryStatus(ctx context.Context, sub *clientSubscription, reason string) {
	if sub.ServerFHIRBase == "" {
		return
	}
	statusURL := sub.ServerFHIRBase + "/Subscription/" + url.PathEscape(sub.ID) + "/$status"
	body, status, err := c.getJSON(ctx, statusURL)
	detail := reason
	if err == nil && status == http.StatusOK {
		if parsed, perr := fhirsubscription.ParseStatusParameters(body); perr == nil {
			latest := int64(-1)
			if len(parsed.Events) > 0 {
				latest = parsed.Events[0].EventNumber
			}
			detail = fmt.Sprintf("%s: server status %s, events-since-subscription-start %d", reason, parsed.Status, latest)
			c.store.withLock(func() { sub.Status = parsed.Status })
		}
	}
	c.store.appendLog(logEntry{Direction: "sent", Type: "status-query", Subscription: sub.Reference, Outcome: outcomeForStatus(status, err), Detail: detail})
}

// heartbeatMissTick detects missed heartbeats: for every subscription with an agreed
// heartbeat period, if no notification of any kind arrived within ~1.5x the period, the
// client SHOULD query $status — which is exactly what it does. Called by the scheduler.
func (c *Component) heartbeatMissTick(ctx context.Context) {
	if !c.config.Client.Enabled || !c.config.Client.HeartbeatMissCheck {
		return
	}
	now := time.Now()
	var due []*clientSubscription
	c.store.withLock(func() {
		for _, sub := range c.store.clientSubs {
			if sub.HeartbeatPeriod == nil || sub.Status != fhirsubscription.StatusActive {
				continue
			}
			deadline := time.Duration(float64(*sub.HeartbeatPeriod)*1.5) * time.Second
			if now.Sub(sub.LastNotificationAt) > deadline {
				sub.LastNotificationAt = now // claim, so one miss logs once per window
				due = append(due, sub)
			}
		}
	})
	for _, sub := range due {
		c.store.appendLog(logEntry{Direction: "received", Type: "heartbeat", Subscription: sub.Reference, Outcome: "missed", Detail: "no notification within 1.5x heartbeat period"})
		slog.WarnContext(ctx, "Heartbeat missed, querying $status", slog.String("subscription", sub.Reference))
		c.wg.Add(1)
		go func(sub *clientSubscription) {
			defer c.wg.Done()
			c.queryStatus(c.backgroundCtx, sub, "heartbeat-miss")
		}(sub)
	}
}

// --- outbound HTTP helpers (client role) ---

func (c *Component) fetchSubscription(ctx context.Context, absoluteRef string) (fhirsubscription.Subscription, error) {
	body, status, err := c.getJSON(ctx, absoluteRef)
	if err != nil {
		return fhirsubscription.Subscription{}, err
	}
	if status != http.StatusOK {
		return fhirsubscription.Subscription{}, fmt.Errorf("HTTP %d", status)
	}
	return fhirsubscription.UnmarshalSubscription(body)
}

func (c *Component) postSubscription(ctx context.Context, endpoint string, resource fhirsubscription.Subscription) (fhirsubscription.Subscription, int, error) {
	body, err := resource.MarshalJSON()
	if err != nil {
		return fhirsubscription.Subscription{}, 0, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(string(body)))
	if err != nil {
		return fhirsubscription.Subscription{}, 0, err
	}
	req.Header.Set("Content-Type", fhirsubscription.FHIRJSONContentType)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fhirsubscription.Subscription{}, 0, err
	}
	defer resp.Body.Close()
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fhirsubscription.Subscription{}, resp.StatusCode, err
	}
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return fhirsubscription.Subscription{}, resp.StatusCode, fmt.Errorf("HTTP %d: %s", resp.StatusCode, truncate(string(responseBody), 300))
	}
	created, err := fhirsubscription.UnmarshalSubscription(responseBody)
	return created, resp.StatusCode, err
}

func (c *Component) getJSON(ctx context.Context, rawURL string) ([]byte, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Accept", fhirsubscription.FHIRJSONContentType)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, err
	}
	return body, resp.StatusCode, nil
}

func outcomeForErr(err error) string {
	if err == nil {
		return "200"
	}
	return err.Error()
}

func outcomeForStatus(status int, err error) string {
	if err != nil {
		return err.Error()
	}
	return strconv.Itoa(status)
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
