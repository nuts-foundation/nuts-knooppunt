package mitzmock

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/caramel/to"
)

// sequentialIDs yields predictable ids, so a whole Bundle can be compared.
func sequentialIDs() func() string {
	n := 0
	return func() string { n++; return fmt.Sprintf("00000000-0000-4000-8000-%012d", n) }
}

// expectedBundle is written from the package's example notification, element
// for element, with the values the mock is specified to send.
func expectedBundle(provisionType, narrative string) string {
	return `{
  "resourceType": "Bundle",
  "id": "11111111-1111-4111-8111-111111111111",
  "type": "transaction",
  "entry": [
    {
      "fullUrl": "urn:uuid:00000000-0000-4000-8000-000000000001",
      "resource": {
        "resourceType": "Consent",
        "id": "00000000-0000-4000-8000-000000000001",
        "meta": {"profile": ["http://vzvz.nl/fhir/StructureDefinition/nl-vzvz-mitz-Consent-Notify|3.8.0"]},
        "text": {"status": "generated", "div": "<div xmlns=\"http://www.w3.org/1999/xhtml\" xml:lang=\"nl-NL\" lang=\"nl-NL\">` + narrative + `</div>"},
        "extension": [
          {"url": "http://fhir.nl/StructureDefinition/OTV-ProviderCategory", "valueCodeableConcept": {"coding": [{"system": "http://fhir.nl/otv/CodeSystem/raadplegende-zorgaanbiedercategorie", "version": "3810200", "code": "RPZAC001"}]}},
          {"url": "http://fhir.nl/StructureDefinition/OTV-ProviderCategory", "valueCodeableConcept": {"coding": [{"system": "http://fhir.nl/otv/CodeSystem/raadplegende-zorgaanbiedercategorie", "version": "3810200", "code": "RPZAC002"}]}},
          {"url": "http://fhir.nl/StructureDefinition/OTV-ProviderCategory", "valueCodeableConcept": {"coding": [{"system": "http://fhir.nl/otv/CodeSystem/raadplegende-zorgaanbiedercategorie", "version": "3810200", "code": "RPZAC003"}]}},
          {"url": "http://fhir.nl/StructureDefinition/OTV-ProviderCategory", "valueCodeableConcept": {"coding": [{"system": "http://fhir.nl/otv/CodeSystem/raadplegende-zorgaanbiedercategorie", "version": "3810200", "code": "RPZAC004"}]}},
          {"url": "http://fhir.nl/StructureDefinition/OTV-ProviderCategory", "valueCodeableConcept": {"coding": [{"system": "http://fhir.nl/otv/CodeSystem/raadplegende-zorgaanbiedercategorie", "version": "3810200", "code": "RPZAC005"}]}},
          {"url": "http://fhir.nl/StructureDefinition/OTV-ProviderCategory", "valueCodeableConcept": {"coding": [{"system": "http://fhir.nl/otv/CodeSystem/raadplegende-zorgaanbiedercategorie", "version": "3810200", "code": "RPZAC006"}]}},
          {"url": "http://fhir.nl/StructureDefinition/OTV-ProviderCategory", "valueCodeableConcept": {"coding": [{"system": "http://fhir.nl/otv/CodeSystem/raadplegende-zorgaanbiedercategorie", "version": "3810200", "code": "RPZAC101"}]}},
          {"url": "http://fhir.nl/StructureDefinition/OTV-ProviderCategory", "valueCodeableConcept": {"coding": [{"system": "http://fhir.nl/otv/CodeSystem/raadplegende-zorgaanbiedercategorie", "version": "3810200", "code": "RPZAC102"}]}},
          {"url": "http://fhir.nl/StructureDefinition/OTV-ProviderCategory", "valueCodeableConcept": {"coding": [{"system": "http://fhir.nl/otv/CodeSystem/raadplegende-zorgaanbiedercategorie", "version": "3810200", "code": "RPZAC103"}]}},
          {"url": "http://fhir.nl/StructureDefinition/OTV-ProviderCategory", "valueCodeableConcept": {"coding": [{"system": "http://fhir.nl/otv/CodeSystem/raadplegende-zorgaanbiedercategorie", "version": "3810200", "code": "RPZAC104"}]}}
        ],
        "status": "active",
        "scope": {"coding": [{"system": "http://terminology.hl7.org/CodeSystem/consentscope", "version": "3810200", "code": "patient-privacy"}]},
        "category": [{"coding": [{"system": "http://fhir.nl/otv/CodeSystem/gegevenscategorie", "version": "3810200", "code": "GGC002", "display": "Behandelgegevens"}]}],
        "patient": {"reference": "urn:uuid:00000000-0000-4000-8000-000000000002"},
        "dateTime": "2026-10-12T14:04:55.123456789Z",
        "policyRule": {"coding": [{"system": "http://fhir.nl/otv/CodeSystem/toestemmingbron", "code": "MITZ"}]},
        "provision": {
          "type": "` + provisionType + `",
          "actor": [{"role": {"coding": [{"system": "http://terminology.hl7.org/CodeSystem/v3-ParticipationType", "code": "CST"}]}, "reference": {"reference": "urn:uuid:00000000-0000-4000-8000-000000000003"}}],
          "purpose": [{"system": "http://hl7.org/fhir/v3/ActReason", "code": "TREAT"}]
        }
      },
      "request": {"method": "POST", "url": "Consent"}
    },
    {
      "fullUrl": "urn:uuid:00000000-0000-4000-8000-000000000002",
      "resource": {
        "resourceType": "Patient",
        "id": "00000000-0000-4000-8000-000000000002",
        "identifier": [{"system": "http://fhir.nl/fhir/NamingSystem/bsn", "value": "999911120"}]
      },
      "request": {"method": "POST", "url": "Patient"}
    },
    {
      "fullUrl": "urn:uuid:00000000-0000-4000-8000-000000000003",
      "resource": {
        "resourceType": "Organization",
        "id": "00000000-0000-4000-8000-000000000003",
        "identifier": [{"system": "http://fhir.nl/fhir/NamingSystem/ura", "value": "00000010"}],
        "type": [{"coding": [{"system": "http://nictiz.nl/fhir/NamingSystem/organization-type", "version": "3810200", "code": "Z3"}]}]
      },
      "request": {"method": "POST", "url": "Organization"}
    }
  ]
}`
}

func TestNotificationBundle_MatchesThePackageExampleForBothDecisions(t *testing.T) {
	at := time.Date(2026, 10, 12, 14, 4, 55, 123456789, time.UTC)
	for _, tc := range []struct{ decision, provision, narrative string }{
		{"Deny", "deny", "De patiënt verleent geen toestemming om Behandelgegevens beschikbaar te stellen aan behandelaren in alle zorgaanbiedercategorieën."},
		{"Permit", "permit", "De patiënt verleent toestemming om Behandelgegevens beschikbaar te stellen aan behandelaren in alle zorgaanbiedercategorieën."},
	} {
		t.Run(tc.decision, func(t *testing.T) {
			change := consentChange{BSN: "999911120", Decision: tc.decision, ChangeID: "11111111-1111-4111-8111-111111111111", ChangedAt: at}
			got, err := json.Marshal(consentNotificationBundle(change, "00000010", "Z3", sequentialIDs()))
			require.NoError(t, err)
			require.JSONEq(t, expectedBundle(tc.provision, tc.narrative), string(got))
		})
	}
}

// notifyTarget is an endpoint that records what it is sent.
type notifyTarget struct {
	mu      sync.Mutex
	server  *httptest.Server
	bodies  []map[string]any
	types   []string
	status  int
	delay   time.Duration // how long the endpoint takes to answer
	arrived chan struct{} // one value per request, sent once it is recorded
	held    chan struct{} // non-nil: every request waits here until release
	once    sync.Once
}

func newNotifyTarget(t *testing.T) *notifyTarget {
	t.Helper()
	target := &notifyTarget{status: http.StatusNoContent, arrived: make(chan struct{}, 16)}
	target.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var body map[string]any
		_ = json.Unmarshal(raw, &body)
		target.mu.Lock()
		target.bodies, target.types = append(target.bodies, body), append(target.types, r.Header.Get("Content-Type"))
		status, delay, held := target.status, target.delay, target.held
		target.mu.Unlock()
		target.arrived <- struct{}{}
		if held != nil {
			<-held
		}
		time.Sleep(delay)
		w.WriteHeader(status)
	}))
	t.Cleanup(target.server.Close)
	// Registered last, so it runs first: a request still held when a test fails
	// must not keep server.Close waiting for its handler.
	t.Cleanup(target.release)
	return target
}

// hold makes every request wait in the handler until release.
func (n *notifyTarget) hold() {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.held = make(chan struct{})
}

func (n *notifyTarget) release() {
	n.mu.Lock()
	held := n.held
	n.mu.Unlock()
	if held != nil {
		n.once.Do(func() { close(held) })
	}
}

// awaitArrival waits until one more request has reached the target.
func (n *notifyTarget) awaitArrival(t *testing.T) {
	t.Helper()
	select {
	case <-n.arrived:
	case <-time.After(5 * time.Second):
		t.Fatal("no notification reached the target")
	}
}

func (n *notifyTarget) decisions() []string {
	n.mu.Lock()
	defer n.mu.Unlock()
	var seen []string
	for _, body := range n.bodies {
		entry := body["entry"].([]any)[0].(map[string]any)["resource"].(map[string]any)
		seen = append(seen, entry["provision"].(map[string]any)["type"].(string))
	}
	return seen
}

// dateTimes returns Consent.dateTime of every notification received, as text
// and as the instant it denotes.
func (n *notifyTarget) dateTimes(t *testing.T) (text []string, instants []time.Time) {
	t.Helper()
	n.mu.Lock()
	defer n.mu.Unlock()
	for _, body := range n.bodies {
		raw := body["entry"].([]any)[0].(map[string]any)["resource"].(map[string]any)["dateTime"].(string)
		instant, err := time.Parse(time.RFC3339Nano, raw)
		require.NoError(t, err)
		text, instants = append(text, raw), append(instants, instant)
	}
	return text, instants
}

// consentResult is what a consent call made on a worker goroutine came to. A
// worker does not assert; the test goroutine does, on this.
type consentResult struct {
	body consentBody
	err  error
}

// goConsentCall makes the call on a goroutine of its own.
func goConsentCall(mitz *ClosedQuestionService, method, path, body string) <-chan consentResult {
	done := make(chan consentResult, 1)
	go func() {
		var result consentResult
		defer func() { done <- result }()
		req, err := http.NewRequest(method, mitz.GetURL()+path, strings.NewReader(body))
		if err != nil {
			result.err = err
			return
		}
		req.Header.Set("Content-Type", "application/json")
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			result.err = err
			return
		}
		defer res.Body.Close()
		raw, err := io.ReadAll(res.Body)
		switch {
		case err != nil:
			result.err = err
		case res.StatusCode != http.StatusOK:
			result.err = fmt.Errorf("%s %s answered %d: %s", method, path, res.StatusCode, raw)
		default:
			result.err = json.Unmarshal(raw, &result.body)
		}
	}()
	return done
}

func await(t *testing.T, done <-chan consentResult) consentBody {
	t.Helper()
	select {
	case result := <-done:
		require.NoError(t, result.err)
		return result.body
	case <-time.After(5 * time.Second):
		t.Fatal("the call did not return")
		return consentBody{}
	}
}

func subscribe(t *testing.T, mitz *ClosedQuestionService, providerID, patientID, endpoint, payload string) {
	t.Helper()
	sub := subscription(providerID, patientID)
	if endpoint != "" {
		sub.Channel.Endpoint = to.Ptr(endpoint)
	}
	if payload != "" {
		sub.Channel.Payload = to.Ptr(payload)
	}
	raw, err := json.Marshal(sub)
	require.NoError(t, err)
	res, err := http.Post(mitz.GetURL()+"/abonnementen/fhir/Subscription", "application/fhir+json", bytes.NewReader(raw))
	require.NoError(t, err)
	require.NoError(t, res.Body.Close())
	require.Equal(t, http.StatusCreated, res.StatusCode)
}

func TestDelivery_NotifiesEverySubscriptionOfThatPatientAndNoOther(t *testing.T) {
	mitz := NewClosedQuestionService(t)
	plataan, other, bramsProvider := newNotifyTarget(t), newNotifyTarget(t), newNotifyTarget(t)
	subscribe(t, mitz, "00000010", anna, plataan.server.URL, "application/fhir+json")
	subscribe(t, mitz, "00000030", anna, other.server.URL, "")
	subscribe(t, mitz, "00000010", bram, bramsProvider.server.URL, "application/fhir+json")

	status, put := consentCall(t, mitz, http.MethodPut, "/mock/consent/"+anna,
		`{"decision":"Deny","changeId":"4bf92f35-77b3-4da6-a3ce-929d0e0e4736"}`)

	require.Equal(t, http.StatusOK, status)
	require.Len(t, put.Notifications, 2)
	for _, result := range put.Notifications {
		require.Equal(t, http.StatusNoContent, result.Status)
		require.Empty(t, result.Error)
		require.NotEmpty(t, result.SubscriptionID)
	}
	require.Equal(t, []string{"deny"}, plataan.decisions())
	require.Equal(t, []string{"deny"}, other.decisions())
	require.Empty(t, bramsProvider.decisions(), "another patient's subscriber hears nothing")
	require.Equal(t, []string{"application/fhir+json"}, plataan.types)
	require.Equal(t, "4bf92f35-77b3-4da6-a3ce-929d0e0e4736", plataan.bodies[0]["id"], "the change id is the Bundle's id")
	organization := plataan.bodies[0]["entry"].([]any)[2].(map[string]any)["resource"].(map[string]any)
	require.Equal(t, "00000010", organization["identifier"].([]any)[0].(map[string]any)["value"], "each subscriber is named as the custodian of its own notification")
}

func TestDelivery_AFailingEndpointDoesNotFailTheChange(t *testing.T) {
	mitz := NewClosedQuestionService(t)
	broken, refused := newNotifyTarget(t), newNotifyTarget(t)
	broken.status = http.StatusBadGateway
	refusedURL := refused.server.URL
	refused.server.Close()
	subscribe(t, mitz, "00000010", anna, broken.server.URL, "")
	subscribe(t, mitz, "00000030", anna, refusedURL, "")

	status, put := consentCall(t, mitz, http.MethodPut, "/mock/consent/"+anna, `{"decision":"Deny"}`)

	require.Equal(t, http.StatusOK, status)
	require.Len(t, put.Notifications, 2)
	require.Equal(t, http.StatusBadGateway, put.Notifications[0].Status)
	require.Zero(t, put.Notifications[1].Status)
	require.NotEmpty(t, put.Notifications[1].Error, "a transport failure is reported as an error, not as a status")
	_, decision := decisionFor(t, mitz, closedQuestion(anna, "00000020", "00000010"))
	require.Equal(t, "Deny", decision, "the answer changed whatever delivery did")
}

func TestDelivery_SkipsAndReportsWhatItCannotSend(t *testing.T) {
	mitz := NewClosedQuestionService(t)
	xml := newNotifyTarget(t)
	subscribe(t, mitz, "00000010", anna, xml.server.URL, "application/fhir+xml")
	subscribe(t, mitz, "00000030", anna, "", "") // the E3 shape: no endpoint

	_, put := consentCall(t, mitz, http.MethodPut, "/mock/consent/"+anna, `{"decision":"Deny"}`)

	require.Len(t, put.Notifications, 1, "a subscription without an endpoint has nowhere to be reported to")
	require.Zero(t, put.Notifications[0].Status)
	require.Contains(t, put.Notifications[0].Error, "application/fhir+xml")
	require.Empty(t, xml.decisions())
}

// While a change is being delivered its patient's mutex is held: that is what
// keeps a second change for the same patient behind it. The test reads the mutex
// itself. Waiting a while to see that a second request does not get through
// proves nothing on a loaded machine, and passes just as well when the second
// request simply has not started yet.
func TestDelivery_TwoChangesForOnePatientArriveInTheOrderTheyWereStored(t *testing.T) {
	mitz := NewClosedQuestionService(t)
	// Two readings a tenth and eleven hundredths past the second: RFC 3339 drops
	// trailing zeros, so as text the earlier one sorts after the later one.
	readings := []time.Time{
		time.Date(2026, 10, 12, 14, 4, 55, 100_000_000, time.UTC),
		time.Date(2026, 10, 12, 14, 4, 55, 110_000_000, time.UTC),
	}
	mitz.clock.mu.Lock()
	mitz.clock.now = func() time.Time {
		reading := readings[0]
		if len(readings) > 1 {
			readings = readings[1:]
		}
		return reading
	}
	mitz.clock.mu.Unlock()
	target := newNotifyTarget(t)
	target.hold()
	subscribe(t, mitz, "00000010", anna, target.server.URL, "")

	first := goConsentCall(mitz, http.MethodPut, "/mock/consent/"+anna, `{"decision":"Deny"}`)
	target.awaitArrival(t)
	if patient := mitz.consent.lockFor(anna); patient.TryLock() {
		patient.Unlock()
		t.Fatal("the patient's mutex is free while the first change is still being delivered")
	}
	second := goConsentCall(mitz, http.MethodPut, "/mock/consent/"+anna, `{"decision":"Permit"}`)

	target.release()
	deny, permit := await(t, first), await(t, second)

	require.Equal(t, []string{"deny", "permit"}, target.decisions())
	text, instants := target.dateTimes(t)
	require.Equal(t, []string{"2026-10-12T14:04:55.1Z", "2026-10-12T14:04:55.11Z"}, text)
	require.Greater(t, text[0], text[1], "as text the earlier timestamp sorts last, which is why nothing may compare them as text")
	require.True(t, instants[1].After(instants[0]), "as instants they are in the order of the changes")
	require.True(t, instants[0].Equal(deny.ChangedAt) && instants[1].Equal(permit.ChangedAt), "each notification carries its own change's timestamp")
}

// holdingWriter stops a handler at its answer: the first Write reports that the
// handler got there and then waits, which keeps the handler inside whatever it
// holds while it answers.
type holdingWriter struct {
	*httptest.ResponseRecorder
	writing chan struct{}
	letGo   chan struct{}
	stop    sync.Once
	free    sync.Once
}

func newHoldingWriter(t *testing.T) *holdingWriter {
	t.Helper()
	w := &holdingWriter{ResponseRecorder: httptest.NewRecorder(), writing: make(chan struct{}), letGo: make(chan struct{})}
	// A handler still held when a test fails must not outlive it.
	t.Cleanup(w.release)
	return w
}

func (w *holdingWriter) Write(p []byte) (int, error) {
	w.stop.Do(func() {
		close(w.writing)
		<-w.letGo
	})
	return w.ResponseRecorder.Write(p)
}

func (w *holdingWriter) release() { w.free.Do(func() { close(w.letGo) }) }

// A delete may not return while a change it should have waited for is still
// being delivered. The change holds a lock for as long as its delivery runs; the
// delete is required to hold that same lock while it answers. Both at once is
// impossible, so the delete cannot have answered before the delivery ended.
func TestDelivery_ADeleteWaitsForARunningDelivery(t *testing.T) {
	for name, tc := range map[string]struct {
		handler func(*ClosedQuestionService) http.HandlerFunc
		// heldByChange is read while the delivery runs, heldByDelete while the
		// delete answers. Each reports whether the other party is shut out.
		heldByChange, heldByDelete func(*ClosedQuestionService) bool
	}{
		"of one patient": {
			handler:      func(m *ClosedQuestionService) http.HandlerFunc { return m.handleDeleteConsent },
			heldByChange: func(m *ClosedQuestionService) bool { return !tryLock(m.consent.lockFor(anna)) },
			heldByDelete: func(m *ClosedQuestionService) bool { return !tryLock(m.consent.lockFor(anna)) },
		},
		"of everything": {
			handler: func(m *ClosedQuestionService) http.HandlerFunc { return m.handleDeleteAllConsent },
			// A change holds the gate shared, which shuts out a delete of
			// everything; that delete holds it alone, which shuts out a change.
			heldByChange: func(m *ClosedQuestionService) bool {
				if m.consent.changes.TryLock() {
					m.consent.changes.Unlock()
					return false
				}
				return true
			},
			heldByDelete: func(m *ClosedQuestionService) bool {
				if m.consent.changes.TryRLock() {
					m.consent.changes.RUnlock()
					return false
				}
				return true
			},
		},
	} {
		t.Run(name, func(t *testing.T) {
			mitz := NewClosedQuestionService(t)
			target := newNotifyTarget(t)
			target.hold()
			subscribe(t, mitz, "00000010", anna, target.server.URL, "")

			changed := goConsentCall(mitz, http.MethodPut, "/mock/consent/"+anna, `{"decision":"Deny"}`)
			target.awaitArrival(t)
			require.True(t, tc.heldByChange(mitz), "the delete could run during the delivery")

			writer := newHoldingWriter(t)
			request := httptest.NewRequest(http.MethodDelete, "/mock/consent/"+anna, nil)
			request.SetPathValue("bsn", anna)
			deleted := make(chan struct{})
			go func() {
				defer close(deleted)
				tc.handler(mitz)(writer, request)
			}()

			target.release()
			put := await(t, changed)
			select {
			case <-writer.writing:
			case <-time.After(5 * time.Second):
				t.Fatal("the delete did not answer after the delivery ended")
			}
			// The change is over, so what is held now is held by the delete.
			require.True(t, tc.heldByDelete(mitz), "the delete answers without holding what a running delivery holds")

			writer.release()
			<-deleted
			var answer consentBody
			require.NoError(t, json.Unmarshal(writer.Body.Bytes(), &answer), "body: %s", writer.Body)
			require.True(t, answer.Watermark.After(put.ChangedAt), "the watermark is later than the change the delete waited for")
		})
	}
}

func tryLock(mu *sync.Mutex) bool {
	if !mu.TryLock() {
		return false
	}
	mu.Unlock()
	return true
}

func TestDelivery_AllEndpointsShareOneBudget(t *testing.T) {
	previous := deliveryBudget
	deliveryBudget = 150 * time.Millisecond
	defer func() { deliveryBudget = previous }()

	mitz := NewClosedQuestionService(t)
	slow, never := newNotifyTarget(t), newNotifyTarget(t)
	slow.delay = time.Second
	subscribe(t, mitz, "00000010", anna, slow.server.URL, "")
	subscribe(t, mitz, "00000030", anna, never.server.URL, "")

	started := time.Now()
	_, put := consentCall(t, mitz, http.MethodPut, "/mock/consent/"+anna, `{"decision":"Deny"}`)

	require.Less(t, time.Since(started), 600*time.Millisecond, "one budget for the whole change, not one per endpoint")
	require.Len(t, put.Notifications, 2)
	require.NotEmpty(t, put.Notifications[0].Error)
	require.Contains(t, put.Notifications[1].Error, "not attempted")
	require.Empty(t, never.decisions())
}
