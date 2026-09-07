package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

// mitzCallTimeout bounds the sandbox's calls to the Knooppunt's Mitz endpoint and
// to the mock. Without it a blackholed Mitz holds the share request open past any
// useful demo.
const mitzCallTimeout = 10 * time.Second

// mitzSubscribeFunc builds the call that starts a consent subscription for a
// patient at De Plataan.
//
// providertype comes from the same SANDBOX_FACILITY_TYPE that feeds
// organization_facility_type in the access token, rather than a second constant
// that could drift from it. Z3 as the hospital provider type is a convention
// this repository documents (docs/INTEGRATION.md); it has not been checked
// against the Mitz register.
func mitzSubscribeFunc(knooppuntInternalURL *url.URL, providerURA, providerType string) func(context.Context, string) error {
	endpoint := knooppuntInternalURL.JoinPath("mitz", "Subscription").String()
	client := &http.Client{Timeout: mitzCallTimeout}

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
		req.Header.Set("X-Tenant-ID", "http://fhir.nl/fhir/NamingSystem/ura|"+providerURA)

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
// this provider/patient pair exist?
//
// It reads the mock directly rather than going through the Knooppunt, because
// the Knooppunt exposes only POST /mitz/Subscription and the national Mitz offers
// no such lookup either. This is a demo affordance: it is what lets the sandbox
// answer "did that subscription actually get created?" after a response it never
// received, and it disappears against a real Mitz. Returns nil when no mock is
// configured, so the caller renders unknown instead of a false negative.
func mitzSubscribedFunc(mitzMockBaseURL *url.URL, providerURA string) func(context.Context, string) (bool, error) {
	if mitzMockBaseURL == nil {
		return nil
	}
	endpoint := mitzMockBaseURL.JoinPath("abonnementen", "fhir", "Subscription")
	client := &http.Client{Timeout: mitzCallTimeout}

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
