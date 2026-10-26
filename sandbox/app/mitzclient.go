package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// mitzCallTimeout bounds the sandbox's calls to the Knooppunt's Mitz endpoint and
// to the mock. Without it a blackholed Mitz holds the share request open past any
// useful demo.
const mitzCallTimeout = 10 * time.Second

// mitzSubscribeFunc builds the call that starts a consent subscription for a
// patient at De Plataan. providertype comes from the same SANDBOX_FACILITY_TYPE
// that feeds organization_facility_type in the access token, so the two cannot
// drift. Z3 as the hospital provider type is a convention this repository
// documents (docs/INTEGRATION.md); it has not been checked against the Mitz
// register.
func mitzSubscribeFunc(knooppuntInternalURL *url.URL, providerURA, providerType string) func(context.Context, string) error {
	endpoint := knooppuntInternalURL.JoinPath("mitz", "Subscription").String()
	client := &http.Client{Transport: newCaptureTransport("consent", nil), Timeout: mitzCallTimeout}

	return func(ctx context.Context, bsn string) error {
		criteria := fmt.Sprintf("Consent?_query=otv&patientid=%s&providerid=%s&providertype=%s",
			url.QueryEscape(bsn), url.QueryEscape(providerURA), url.QueryEscape(providerType))
		body, err := json.Marshal(map[string]any{
			"resourceType": "Subscription",
			"status":       "requested",
			"reason":       "OTV",
			"criteria":     criteria,
			"channel":      map[string]any{"type": "rest-hook"},
		})
		if err != nil {
			return fmt.Errorf("build Mitz subscription: %w", err)
		}

		ctx, cancel := context.WithTimeout(ctx, mitzCallTimeout)
		defer cancel()
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
		if err != nil {
			return fmt.Errorf("build Mitz subscription request: %w", err)
		}
		req.Header.Set("Content-Type", "application/fhir+json")

		res, err := client.Do(req)
		if err != nil {
			return fmt.Errorf("call Mitz subscription endpoint: %w", err)
		}
		defer res.Body.Close()
		if res.StatusCode != http.StatusCreated {
			return fmt.Errorf("Mitz subscription returned %s", res.Status)
		}
		return nil
	}
}

// mitzSubscribedFunc builds the reconciliation query: does a subscription for
// this provider/patient pair exist? It reads the mock directly, because neither
// the Knooppunt (POST /mitz/Subscription only) nor the national Mitz offers a
// lookup; it is a demo affordance that disappears against a real Mitz. Returns
// nil when no mock is configured, so the caller renders unknown instead of a
// false negative.
func mitzSubscribedFunc(mitzMockBaseURL *url.URL, providerURA string) func(context.Context, string) (bool, error) {
	if mitzMockBaseURL == nil {
		return nil
	}
	endpoint := mitzMockBaseURL.JoinPath("abonnementen", "fhir", "Subscription")
	client := &http.Client{Transport: newCaptureTransport("consent", nil), Timeout: mitzCallTimeout}

	return func(ctx context.Context, bsn string) (bool, error) {
		query := *endpoint
		query.RawQuery = url.Values{"providerid": {providerURA}, "patientid": {bsn}}.Encode()

		ctx, cancel := context.WithTimeout(ctx, mitzCallTimeout)
		defer cancel()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, query.String(), nil)
		if err != nil {
			return false, fmt.Errorf("build Mitz lookup request: %w", err)
		}
		res, err := client.Do(req)
		if err != nil {
			return false, fmt.Errorf("query Mitz subscriptions: %w", err)
		}
		defer res.Body.Close()
		if res.StatusCode != http.StatusOK {
			return false, fmt.Errorf("Mitz subscription lookup returned %s", res.Status)
		}
		var bundle struct {
			Entry []json.RawMessage `json:"entry"`
		}
		if err := json.NewDecoder(res.Body).Decode(&bundle); err != nil {
			return false, fmt.Errorf("parse Mitz subscription bundle: %w", err)
		}
		return len(bundle.Entry) > 0, nil
	}
}

// mitzDecisionsFunc builds the read of the mock Mitz's decision log by trace id.
// Like the subscription lookup it exists only on the mock, and it is not
// captured: it is how the viewer learns of a closed question, not a call of the
// demo. Returns nil when no mock is configured.
func mitzDecisionsFunc(mitzMockBaseURL *url.URL) func(context.Context, string) ([]mitzDecision, error) {
	if mitzMockBaseURL == nil {
		return nil
	}
	endpoint := mitzMockBaseURL.JoinPath("mock", "decisions")
	client := &http.Client{}
	return func(ctx context.Context, traceID string) ([]mitzDecision, error) {
		query := *endpoint
		query.RawQuery = url.Values{"traceid": {traceID}}.Encode()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, query.String(), nil)
		if err != nil {
			return nil, fmt.Errorf("build Mitz decision read: %w", err)
		}
		res, err := client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("read Mitz decisions: %w", err)
		}
		defer res.Body.Close()
		if res.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("Mitz decision read returned %s", res.Status)
		}
		var body struct {
			Decisions []mitzDecision `json:"decisions"`
		}
		if err := json.NewDecoder(io.LimitReader(res.Body, 4<<20)).Decode(&body); err != nil {
			return nil, fmt.Errorf("parse Mitz decisions: %w", err)
		}
		return body.Decisions, nil
	}
}

// consentControlTimeout bounds the call that changes consent at the mock. It is
// longer than mitzCallTimeout because the mock delivers its notifications
// before it answers, inside a budget of 8 seconds, and the Knooppunt's relay of
// each takes at most 5. An outer hop that gave up first would report a failure
// the inner one was about to deny.
const consentControlTimeout = 15 * time.Second

// mitzConsentStatus is the answer a closed question about a patient would get
// now, as the mock reports it.
type mitzConsentStatus struct {
	Decision   string
	Overridden bool
	ChangedAt  time.Time
}

// mitzSetConsentFunc builds the simulated patient act: changing the mock's
// answer for one patient. It talks to the mock directly, because consent is not
// registered through the Knooppunt, and it is captured, so the act is an event
// in the run. The national Mitz has no such endpoint. Returns nil when no mock
// is configured.
func mitzSetConsentFunc(mitzMockBaseURL *url.URL) func(context.Context, string, string, string) (time.Time, error) {
	if mitzMockBaseURL == nil {
		return nil
	}
	client := &http.Client{Transport: newCaptureTransport("consent", nil), Timeout: consentControlTimeout}
	return func(ctx context.Context, bsn, decision, changeID string) (time.Time, error) {
		body, err := json.Marshal(map[string]string{"decision": decision, "changeId": changeID})
		if err != nil {
			return time.Time{}, fmt.Errorf("build consent change: %w", err)
		}
		ctx, cancel := context.WithTimeout(withEventPurpose(ctx, "consent-change"), consentControlTimeout)
		defer cancel()
		req, err := http.NewRequestWithContext(ctx, http.MethodPut,
			mitzMockBaseURL.JoinPath("mock", "consent", bsn).String(), bytes.NewReader(body))
		if err != nil {
			return time.Time{}, fmt.Errorf("build consent change request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")
		res, err := client.Do(req)
		if err != nil {
			return time.Time{}, fmt.Errorf("change consent at Mitz: %w", err)
		}
		defer res.Body.Close()
		raw, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
		if err != nil {
			return time.Time{}, fmt.Errorf("read consent change answer: %w", err)
		}
		if res.StatusCode != http.StatusOK {
			return time.Time{}, fmt.Errorf("consent change returned %s", res.Status)
		}
		var answer struct {
			ChangedAt time.Time `json:"changedAt"`
		}
		if err := json.Unmarshal(raw, &answer); err != nil {
			return time.Time{}, fmt.Errorf("parse consent change answer: %w", err)
		}
		return answer.ChangedAt, nil
	}
}

// mitzConsentStateFunc builds the read of what Mitz answers about a patient
// now. A status read for the demo's card and for reconciling a change whose
// answer was lost; not captured.
func mitzConsentStateFunc(mitzMockBaseURL *url.URL) func(context.Context, string) (mitzConsentStatus, error) {
	if mitzMockBaseURL == nil {
		return nil
	}
	client := &http.Client{Timeout: mitzCallTimeout}
	return func(ctx context.Context, bsn string) (mitzConsentStatus, error) {
		ctx, cancel := context.WithTimeout(ctx, mitzCallTimeout)
		defer cancel()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, mitzMockBaseURL.JoinPath("mock", "consent", bsn).String(), nil)
		if err != nil {
			return mitzConsentStatus{}, fmt.Errorf("build consent state request: %w", err)
		}
		res, err := client.Do(req)
		if err != nil {
			return mitzConsentStatus{}, fmt.Errorf("read consent state: %w", err)
		}
		defer res.Body.Close()
		if res.StatusCode != http.StatusOK {
			return mitzConsentStatus{}, fmt.Errorf("consent state returned %s", res.Status)
		}
		var answer struct {
			Decision   string    `json:"decision"`
			Overridden bool      `json:"overridden"`
			ChangedAt  time.Time `json:"changedAt"`
		}
		if err := json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&answer); err != nil {
			return mitzConsentStatus{}, fmt.Errorf("parse consent state: %w", err)
		}
		if answer.Decision != "Permit" && answer.Decision != "Deny" {
			return mitzConsentStatus{}, fmt.Errorf("consent state carried decision %q", answer.Decision)
		}
		return mitzConsentStatus{Decision: answer.Decision, Overridden: answer.Overridden, ChangedAt: answer.ChangedAt}, nil
	}
}
