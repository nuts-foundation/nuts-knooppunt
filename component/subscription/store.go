package subscription

import (
	"sort"
	"sync"
	"time"

	"github.com/nuts-foundation/nuts-knooppunt/lib/fhirsubscription"
)

// Storage is in-memory with small store types behind one mutex (plan D3): the POC goal is
// spec feedback, not durability. Non-durability across restarts — event numbers, the
// $events log, the client's processed-set — is recorded as an explicit POC limitation
// (see 09-knooppunt-vendor-split.md "honest limits"), not solved.

// serverSubscription is the Subscription Server's record of one subscription it serves.
type serverSubscription struct {
	ID              string
	Topic           string
	Filters         []string
	PartnerURA      string
	Mode            fhirsubscription.PayloadMode
	Endpoint        string
	HeartbeatPeriod *int // seconds; nil = no heartbeat agreed
	Status          fhirsubscription.Status
	CreatedVia      string // in-band | out-of-band
	EventCounter    int64
	// LastSentAt is when the last notification (of any type) was delivered, used to
	// schedule heartbeats.
	LastSentAt time.Time
	CreatedAt  time.Time
}

// OwnerURAFilter returns the URA from the subscription's Task.owner filter, preferring
// the explicit partner URA when set.
func (s *serverSubscription) OwnerURAFilter() string {
	if s.PartnerURA != "" {
		return s.PartnerURA
	}
	for _, criteria := range s.Filters {
		if ura := filterURA(criteria); ura != "" {
			return ura
		}
	}
	return ""
}

// eventRecord is one allocated event, kept for $events replay, the continuity story and
// the logging obligation in one structure.
type eventRecord struct {
	SubscriptionID string
	EventNumber    int64
	Timestamp      time.Time
	// Focus is the aliased relative reference (Task/<alias>); nil in empty mode.
	Focus *string
	// FocusFullURL is the absolute (aliased) URL at this server's public FHIR surface.
	FocusFullURL *string
	// Suppressed marks events whose delivery was deliberately skipped (test/demo
	// facility for exercising gap recovery).
	Suppressed bool
}

// clientSubscription is the Subscription Client's record of one subscription held at a
// partner server.
type clientSubscription struct {
	// Reference is the absolute reference of the Subscription at the partner, the key
	// notifications are matched on.
	Reference string
	// ID is the Subscription's logical id at the partner.
	ID         string
	Topic      string
	PartnerURA string
	Mode       fhirsubscription.PayloadMode
	// ServerFHIRBase is the partner's public FHIR base, used for $status/$events.
	ServerFHIRBase  string
	HeartbeatPeriod *int
	Status          fhirsubscription.Status
	Origin          string // in-band | out-of-band
	HighestEvent    int64
	Processed       map[int64]bool
	// LastNotificationAt is when any notification (event, heartbeat, handshake) was
	// last received; the missed-heartbeat timer measures against it.
	LastNotificationAt time.Time
}

// inboxEntry is a normalised event as presented to the vendor: one uniform shape whatever
// the remote server's optional-feature choices were (research question 6).
type inboxEntry struct {
	ID                    int64
	Type                  string // event | gap
	SubscriptionReference string
	Topic                 string
	EventNumber           *int64
	Focus                 *string
	GapFrom               *int64
	GapTo                 *int64
	ReceivedAt            time.Time
	Acknowledged          bool
}

// logEntry is one transport-log record: all sent and received subscriptions and
// notifications (spec implementation obligation 1).
type logEntry struct {
	Time         time.Time
	Direction    string // sent | received
	Type         string // subscription-create, handshake, event, heartbeat, status-query, events-query, ...
	Subscription string
	EventNumber  *int64
	PayloadMode  string
	Outcome      string
	Detail       string
}

// authorizationBasis is a vendor-registered "who may be notified of what" record, checked
// by the authorization stub hook.
type authorizationBasis struct {
	ReceiverURA string
	Topic       string
	PatientBSN  string
	ValidUntil  *time.Time
}

// store is the component's entire mutable state, guarded by one mutex. Delivery I/O
// happens outside the lock; event-number allocation happens under it, which makes
// allocation concurrency-safe per the spec's event-notification rule.
type store struct {
	mu sync.Mutex

	serverSubs map[string]*serverSubscription
	events     map[string][]eventRecord // keyed by subscription ID, ascending event numbers

	clientSubs map[string]*clientSubscription // keyed by Reference

	inbox     []inboxEntry
	inboxSeq  int64
	log       []logEntry
	authBases []authorizationBasis
	aliases   map[string]string // alias id -> internal relative reference (Task/4711)
}

func newStore() *store {
	return &store{
		serverSubs: map[string]*serverSubscription{},
		events:     map[string][]eventRecord{},
		clientSubs: map[string]*clientSubscription{},
		aliases:    map[string]string{},
	}
}

// withLock runs fn under the store mutex; used for compound mutations of client
// subscriptions, whose records are shared by pointer.
func (s *store) withLock(fn func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	fn()
}

// maxLogEntries bounds the in-memory transport log.
const maxLogEntries = 10000

func (s *store) appendLog(entry logEntry) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entry.Time = time.Now()
	if len(s.log) >= maxLogEntries {
		s.log = s.log[1:]
	}
	s.log = append(s.log, entry)
}

func (s *store) logEntries() []logEntry {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]logEntry, len(s.log))
	copy(out, s.log)
	return out
}

// serverSubscriptionsSnapshot returns copies of all server-side subscriptions, sorted by
// creation time for stable listings.
func (s *store) serverSubscriptionsSnapshot() []serverSubscription {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]serverSubscription, 0, len(s.serverSubs))
	for _, sub := range s.serverSubs {
		out = append(out, *sub)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out
}

func (s *store) clientSubscriptionsSnapshot() []clientSubscription {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]clientSubscription, 0, len(s.clientSubs))
	for _, sub := range s.clientSubs {
		copied := *sub
		copied.Processed = nil // internal detail, not part of snapshots
		out = append(out, copied)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Reference < out[j].Reference })
	return out
}

// updateServerSub runs fn on the subscription with the given id under the lock.
// Returns false when the subscription doesn't exist.
func (s *store) updateServerSub(id string, fn func(*serverSubscription)) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	sub, ok := s.serverSubs[id]
	if !ok {
		return false
	}
	fn(sub)
	return true
}

// getServerSub returns a copy of the subscription with the given id.
func (s *store) getServerSub(id string) (serverSubscription, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sub, ok := s.serverSubs[id]
	if !ok {
		return serverSubscription{}, false
	}
	return *sub, true
}

// allocateEvent increments the subscription's event counter and appends the record —
// atomically, so event numbers are monotonically increasing and concurrency-safe.
func (s *store) allocateEvent(subID string, build func(number int64) eventRecord) (eventRecord, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sub, ok := s.serverSubs[subID]
	if !ok {
		return eventRecord{}, false
	}
	sub.EventCounter++
	record := build(sub.EventCounter)
	record.SubscriptionID = subID
	s.events[subID] = append(s.events[subID], record)
	return record, true
}

// eventRange returns records with since <= number <= until, and whether the whole range
// is available.
func (s *store) eventRange(subID string, since, until int64) ([]eventRecord, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []eventRecord
	for _, record := range s.events[subID] {
		if record.EventNumber >= since && record.EventNumber <= until {
			out = append(out, record)
		}
	}
	return out, int64(len(out)) == until-since+1
}

// getClientSub finds a client subscription by the reference in a notification: exact
// match first, then a unique suffix match on /Subscription/{id} — servers may emit
// relative references (the spec's examples do) while the client stored an absolute one.
func (s *store) getClientSub(reference string) (*clientSubscription, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if sub, ok := s.clientSubs[reference]; ok {
		return sub, true
	}
	var match *clientSubscription
	for _, sub := range s.clientSubs {
		if sub.ID != "" && subscriptionRefID(reference) == sub.ID {
			if match != nil {
				return nil, false // ambiguous
			}
			match = sub
		}
	}
	return match, match != nil
}

// getOrPutClientSub stores sub unless a record with the same reference already exists,
// returning the record that is now live. Concurrent arrivals (e.g. two notifications
// bootstrapping the same unknown subscription) converge on one record.
func (s *store) getOrPutClientSub(sub *clientSubscription) *clientSubscription {
	s.mu.Lock()
	defer s.mu.Unlock()
	if existing, ok := s.clientSubs[sub.Reference]; ok {
		return existing
	}
	s.clientSubs[sub.Reference] = sub
	return sub
}

// upsertIntentClientSub records an in-band intent. The server handshakes immediately on
// creation, so the handshake may already have bootstrapped (and activated) a record for
// the same reference before the intent call returns — in that case the live record is
// kept and only the intent metadata is folded in. Returns a snapshot for the caller.
func (s *store) upsertIntentClientSub(sub *clientSubscription) clientSubscription {
	s.mu.Lock()
	defer s.mu.Unlock()
	live, ok := s.clientSubs[sub.Reference]
	if ok {
		live.Topic = sub.Topic
		live.PartnerURA = sub.PartnerURA
		live.Mode = sub.Mode
		live.HeartbeatPeriod = sub.HeartbeatPeriod
		live.Origin = sub.Origin
	} else {
		s.clientSubs[sub.Reference] = sub
		live = sub
	}
	snapshot := *live
	snapshot.Processed = nil
	return snapshot
}

// appendInbox adds a normalised entry, assigning the next sequence number.
func (s *store) appendInbox(entry inboxEntry) inboxEntry {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.inboxSeq++
	entry.ID = s.inboxSeq
	entry.ReceivedAt = time.Now()
	s.inbox = append(s.inbox, entry)
	return entry
}

func (s *store) inboxSnapshot(since int64, includeAcked bool) []inboxEntry {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []inboxEntry
	for _, entry := range s.inbox {
		if entry.ID <= since {
			continue
		}
		if entry.Acknowledged && !includeAcked {
			continue
		}
		out = append(out, entry)
	}
	return out
}

func (s *store) ackInbox(id int64) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.inbox {
		if s.inbox[i].ID == id {
			s.inbox[i].Acknowledged = true
			return true
		}
	}
	return false
}

func (s *store) addAuthorizationBasis(basis authorizationBasis) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.authBases = append(s.authBases, basis)
}

// findAuthorizationBasis reports whether a registered basis covers the receiver/topic.
func (s *store) findAuthorizationBasis(receiverURA, topic string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	for _, basis := range s.authBases {
		if basis.ReceiverURA == receiverURA && basis.Topic == topic &&
			(basis.ValidUntil == nil || basis.ValidUntil.After(now)) {
			return true
		}
	}
	return false
}

func (s *store) putAlias(alias, internalRef string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.aliases[alias] = internalRef
}

func (s *store) resolveAlias(alias string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ref, ok := s.aliases[alias]
	return ref, ok
}
