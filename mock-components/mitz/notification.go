package mitzmock

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// deliveryBudget is what delivering one change to all of its subscribers may
// take. A variable so a test can shorten it; nothing else reassigns it.
var deliveryBudget = 8 * time.Second

// The catalog version the package's example notification carries on its codings.
const catalogVersion = "3810200"

// activeProviderCategories are the codes of
// http://fhir.nl/otv/CodeSystem/raadplegende-zorgaanbiedercategorie whose
// status is active in vzvz.fhir.mitz 3.8.0. RPZAC100 is deprecated there and is
// left out. The mock's consent is all or nothing, so a notification lists them
// all.
var activeProviderCategories = []string{
	"RPZAC001", "RPZAC002", "RPZAC003", "RPZAC004", "RPZAC005",
	"RPZAC006", "RPZAC101", "RPZAC102", "RPZAC103", "RPZAC104",
}

// consentNotificationBundle builds the notification of one change for one
// subscriber, following the package's example (nl-vzvz-mitz-bundle-notify.json)
// element for element. newID supplies the ids of its three resources.
func consentNotificationBundle(change consentChange, providerURA, providerType string, newID func() string) map[string]any {
	consentID, patientID, organizationID := newID(), newID(), newID()
	provision, verb := "permit", "verleent toestemming"
	if change.Decision == "Deny" {
		provision, verb = "deny", "verleent geen toestemming"
	}
	categories := make([]any, 0, len(activeProviderCategories))
	for _, code := range activeProviderCategories {
		categories = append(categories, map[string]any{
			"url": "http://fhir.nl/StructureDefinition/OTV-ProviderCategory",
			"valueCodeableConcept": map[string]any{"coding": []any{map[string]any{
				"system": "http://fhir.nl/otv/CodeSystem/raadplegende-zorgaanbiedercategorie", "version": catalogVersion, "code": code,
			}}},
		})
	}
	entry := func(id, resourceType string, resource map[string]any) map[string]any {
		resource["resourceType"], resource["id"] = resourceType, id
		return map[string]any{
			"fullUrl": "urn:uuid:" + id, "resource": resource,
			"request": map[string]any{"method": "POST", "url": resourceType},
		}
	}
	return map[string]any{
		"resourceType": "Bundle", "id": change.ChangeID, "type": "transaction",
		"entry": []any{
			entry(consentID, "Consent", map[string]any{
				"meta": map[string]any{"profile": []any{"http://vzvz.nl/fhir/StructureDefinition/nl-vzvz-mitz-Consent-Notify|3.8.0"}},
				"text": map[string]any{"status": "generated", "div": `<div xmlns="http://www.w3.org/1999/xhtml" xml:lang="nl-NL" lang="nl-NL">De patiënt ` + verb +
					` om Behandelgegevens beschikbaar te stellen aan behandelaren in alle zorgaanbiedercategorieën.</div>`},
				"extension": categories,
				"status":    "active",
				"scope": map[string]any{"coding": []any{map[string]any{
					"system": "http://terminology.hl7.org/CodeSystem/consentscope", "version": catalogVersion, "code": "patient-privacy"}}},
				"category": []any{map[string]any{"coding": []any{map[string]any{
					"system": "http://fhir.nl/otv/CodeSystem/gegevenscategorie", "version": catalogVersion, "code": "GGC002", "display": "Behandelgegevens"}}}},
				"patient":    map[string]any{"reference": "urn:uuid:" + patientID},
				"dateTime":   change.ChangedAt.Format(time.RFC3339Nano),
				"policyRule": map[string]any{"coding": []any{map[string]any{"system": "http://fhir.nl/otv/CodeSystem/toestemmingbron", "code": "MITZ"}}},
				"provision": map[string]any{
					"type": provision,
					"actor": []any{map[string]any{
						"role":      map[string]any{"coding": []any{map[string]any{"system": "http://terminology.hl7.org/CodeSystem/v3-ParticipationType", "code": "CST"}}},
						"reference": map[string]any{"reference": "urn:uuid:" + organizationID},
					}},
					"purpose": []any{map[string]any{"system": "http://hl7.org/fhir/v3/ActReason", "code": "TREAT"}},
				},
			}),
			entry(patientID, "Patient", map[string]any{
				"identifier": []any{map[string]any{"system": "http://fhir.nl/fhir/NamingSystem/bsn", "value": change.BSN}},
			}),
			entry(organizationID, "Organization", map[string]any{
				"identifier": []any{map[string]any{"system": "http://fhir.nl/fhir/NamingSystem/ura", "value": providerURA}},
				"type": []any{map[string]any{"coding": []any{map[string]any{
					"system": "http://nictiz.nl/fhir/NamingSystem/organization-type", "version": catalogVersion, "code": providerType}}}},
			}),
		},
	}
}

// deliverChange posts the notification of one change to every subscription of
// that patient, one after another, inside one budget for them all. The caller
// holds the patient's change mutex, so this is where the order of delivery is
// the order of the changes. No store mutex is held while it runs.
func (m *ClosedQuestionService) deliverChange(change consentChange) []notificationResult {
	ctx, cancel := context.WithTimeout(context.Background(), deliveryBudget)
	defer cancel()
	results := []notificationResult{}
	for _, sub := range m.subscriptions.forPatient(change.BSN) {
		if sub.Channel.Endpoint == nil || *sub.Channel.Endpoint == "" {
			continue
		}
		result := notificationResult{SubscriptionID: *sub.Id, Endpoint: *sub.Channel.Endpoint}
		params, _ := url.ParseQuery(strings.TrimPrefix(sub.Criteria, "Consent?"))
		switch {
		case sub.Channel.Payload != nil && *sub.Channel.Payload != "" && *sub.Channel.Payload != "application/fhir+json":
			result.Error = fmt.Sprintf("not attempted: this mock sends application/fhir+json only, the subscription asks for %s", *sub.Channel.Payload)
		case ctx.Err() != nil:
			result.Error = "not attempted: the delivery budget for this change was used up"
		default:
			bundle := consentNotificationBundle(change, params.Get("providerid"), params.Get("providertype"), newUUID)
			result.Status, result.Error = postNotification(ctx, m.notifier, result.Endpoint, bundle)
		}
		results = append(results, result)
	}
	return results
}

func postNotification(ctx context.Context, client *http.Client, endpoint string, bundle map[string]any) (int, string) {
	body, err := json.Marshal(bundle)
	if err != nil {
		return 0, err.Error()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return 0, err.Error()
	}
	req.Header.Set("Content-Type", "application/fhir+json")
	res, err := client.Do(req)
	if err != nil {
		return 0, err.Error()
	}
	defer res.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(res.Body, 64<<10))
	return res.StatusCode, ""
}
