package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"time"

	"github.com/nuts-foundation/nuts-knooppunt/test/testdata/vectors/pool"
)

const (
	notifyBodyLimit   = 1 << 20
	bsnNamingSystem   = "http://fhir.nl/fhir/NamingSystem/bsn"
	uraNamingSystem   = "http://fhir.nl/fhir/NamingSystem/ura"
	custodianRoleCode = "CST"
	fhirJSONMediaType = "application/fhir+json"
)

// consentBundle is what the intake reads from a notification: which change it
// reports, what the patient decided and when, and whom it is about.
type consentBundle struct {
	ID          string
	Decision    string // "deny" or "permit"
	ConsentTime time.Time
	BSN, URA    string
}

type bundleEntry struct {
	FullURL  string          `json:"fullUrl"`
	Resource json.RawMessage `json:"resource"`
}

type identified struct {
	ResourceType string `json:"resourceType"`
	ID           string `json:"id"`
	Identifier   []struct {
		System string `json:"system"`
		Value  string `json:"value"`
	} `json:"identifier"`
}

// consentTimeSkew is how far ahead of this application's clock a
// notification's consent time may be: the difference between two machines'
// clocks, and no more. An accepted time becomes the patient's watermark, which
// only rises, so a time far ahead would make every later notification look old.
const consentTimeSkew = time.Minute

// readConsentNotification reads a Mitz consent notification, and accepts one
// shape and nothing wider: a transaction Bundle with an id and exactly one
// Consent, active, with an explicit permit or deny and a full timestamp, whose
// patient and custodian references each resolve to exactly one entry carrying
// exactly one BSN or URA. A Bundle that leaves any of that open is refused, so
// the wrong patient's data is never removed on a guess.
//
// That is the shape of the vzvz.fhir.mitz 3.8.0 example, which mock Mitz sends.
// The notification captured from the Mitz test environment
// (component/mitz/example/example_notification_bundle.xml) has another: a
// Consent per set of provider categories, the inactive one without
// provision.type. This intake refuses it: the demo's consent is one answer for
// every category (sandbox/DESIGN.md section 10, deviation 6), and it has no
// rule for a notification that splits them. The two also name different
// profiles, which does not matter here: meta.profile is not read.
func readConsentNotification(body []byte) (consentBundle, error) {
	var bundle struct {
		ResourceType string        `json:"resourceType"`
		ID           string        `json:"id"`
		Type         string        `json:"type"`
		Entry        []bundleEntry `json:"entry"`
	}
	if err := json.Unmarshal(body, &bundle); err != nil {
		return consentBundle{}, fmt.Errorf("not JSON: %w", err)
	}
	if bundle.ResourceType != "Bundle" || bundle.Type != "transaction" {
		return consentBundle{}, errors.New("not a transaction Bundle")
	}
	if bundle.ID == "" {
		return consentBundle{}, errors.New("the Bundle has no id")
	}

	var consents []json.RawMessage
	for _, entry := range bundle.Entry {
		var resource identified
		if json.Unmarshal(entry.Resource, &resource) == nil && resource.ResourceType == "Consent" {
			consents = append(consents, entry.Resource)
		}
	}
	if len(consents) != 1 {
		return consentBundle{}, fmt.Errorf("expected exactly one Consent, found %d", len(consents))
	}
	var consent struct {
		Status   string `json:"status"`
		DateTime string `json:"dateTime"`
		Patient  struct {
			Reference string `json:"reference"`
		} `json:"patient"`
		Provision struct {
			Type  string `json:"type"`
			Actor []struct {
				Role struct {
					Coding []struct {
						Code string `json:"code"`
					} `json:"coding"`
				} `json:"role"`
				Reference struct {
					Reference string `json:"reference"`
				} `json:"reference"`
			} `json:"actor"`
		} `json:"provision"`
	}
	if err := json.Unmarshal(consents[0], &consent); err != nil {
		return consentBundle{}, fmt.Errorf("unreadable Consent: %w", err)
	}
	if consent.Status != "active" {
		return consentBundle{}, fmt.Errorf("Consent.status is %q, not active", consent.Status)
	}
	if consent.Provision.Type != "permit" && consent.Provision.Type != "deny" {
		return consentBundle{}, fmt.Errorf("Consent.provision.type is %q, neither permit nor deny", consent.Provision.Type)
	}
	consentTime, err := time.Parse(time.RFC3339Nano, consent.DateTime)
	if err != nil {
		return consentBundle{}, fmt.Errorf("Consent.dateTime is not a full timestamp: %w", err)
	}

	var custodians []string
	for _, actor := range consent.Provision.Actor {
		for _, coding := range actor.Role.Coding {
			if coding.Code == custodianRoleCode {
				custodians = append(custodians, actor.Reference.Reference)
			}
		}
	}
	if len(custodians) != 1 {
		return consentBundle{}, fmt.Errorf("expected exactly one custodian actor, found %d", len(custodians))
	}
	bsn, err := identifierOf(bundle.Entry, consent.Patient.Reference, "Patient", bsnNamingSystem)
	if err != nil {
		return consentBundle{}, err
	}
	ura, err := identifierOf(bundle.Entry, custodians[0], "Organization", uraNamingSystem)
	if err != nil {
		return consentBundle{}, err
	}
	return consentBundle{ID: bundle.ID, Decision: consent.Provision.Type, ConsentTime: consentTime, BSN: bsn, URA: ura}, nil
}

// identifierOf resolves reference, a fullUrl or "Type/id", to exactly one entry
// of resourceType and returns its one identifier in system.
func identifierOf(entries []bundleEntry, reference, resourceType, system string) (string, error) {
	var resolved []identified
	for _, entry := range entries {
		var resource identified
		if json.Unmarshal(entry.Resource, &resource) != nil || resource.ResourceType != resourceType {
			continue
		}
		byID := resource.ID != "" && resourceType+"/"+resource.ID == reference
		if reference != "" && (entry.FullURL == reference || byID) {
			resolved = append(resolved, resource)
		}
	}
	if len(resolved) != 1 {
		return "", fmt.Errorf("the reference %q must resolve to exactly one %s, found %d", reference, resourceType, len(resolved))
	}
	// Counted before their values are looked at. An identifier in the system
	// without a value is still a second identifier, and leaves open which one is
	// meant.
	var inSystem []string
	for _, identifier := range resolved[0].Identifier {
		if identifier.System == system {
			inSystem = append(inSystem, identifier.Value)
		}
	}
	if len(inSystem) != 1 || inSystem[0] == "" {
		return "", fmt.Errorf("the %s must carry exactly one identifier in %s, with a value; found %d", resourceType, system, len(inSystem))
	}
	return inSystem[0], nil
}

func patientByBSN(bsn string) (pool.PoolPatient, bool) {
	for _, patient := range pool.Patients() {
		if patient.BSN == bsn {
			return patient, true
		}
	}
	return pool.PoolPatient{}, false
}

// handleMitzNotification is the EHR's end of the consent subscription: the
// Knooppunt forwards here what Mitz sent it. It is served on a listener of its
// own that no ingress routes to, and has no authentication.
//
// The notification decides nothing about access: the source asks Mitz itself on
// every request. It changes what this EHR shows. A withdrawal removes what was
// retrieved about the patient, which is demo behaviour: a Mitz subscription
// reports consent about the subscriber's own data.
func (c Config) handleMitzNotification(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost || r.URL.Path != "/mitz/notify" {
		http.NotFound(w, r)
		return
	}
	contentType := r.Header.Get("Content-Type")
	if mediaType, _, _ := mime.ParseMediaType(contentType); mediaType != fhirJSONMediaType {
		http.Error(w, "only application/fhir+json notifications are accepted", http.StatusUnsupportedMediaType)
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, notifyBodyLimit))
	var tooLarge *http.MaxBytesError
	switch {
	case errors.As(err, &tooLarge):
		http.Error(w, "notification exceeds 1 MiB", http.StatusRequestEntityTooLarge)
		return
	case err != nil:
		http.Error(w, "notification body could not be read", http.StatusBadRequest)
		return
	}
	notification, err := readConsentNotification(body)
	if err != nil {
		http.Error(w, "not a Mitz consent notification this application accepts: "+err.Error(), http.StatusBadRequest)
		return
	}
	patient, known := patientByBSN(notification.BSN)
	if notification.URA != plataanURA || !known {
		// Valid, and not for anything this application shows. Logged without the
		// BSN.
		slog.Info("Mitz notification ignored: not for De Plataan or not for a pool patient", "custodian", notification.URA)
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if ahead := notification.ConsentTime.Sub(c.runtime.now()); ahead > consentTimeSkew {
		slog.Warn("Mitz notification refused: its consent time is ahead of this application's clock", "ahead", ahead.String())
		http.Error(w, "not a Mitz consent notification this application accepts: Consent.dateTime is ahead of this application's clock", http.StatusBadRequest)
		return
	}

	c.runtime.at(probeIntakeArrived, patient.Key)
	removed := false
	accepted, resets := c.runtime.accept(patient.Key, consentNotification{
		Decision: notification.Decision, ConsentTime: notification.ConsentTime, ReceivedAt: time.Now().UTC(),
	}, func() {
		if notification.Decision == "deny" {
			removed = c.Retrievals.withdraw(patient.Key)
		} else {
			c.Retrievals.clearMarks(patient.Key)
		}
	})
	if accepted {
		// No state lock is held from here on.
		c.runtime.at(probeIntakeAccepted, patient.Key)
		redact := eventRedactor{reveal: c.revealEventIdentifiers}
		event := stepEvent{
			GF: "consent", Actor: "mitz", Purpose: "consent-notification", Action: "consent", CallID: randomURLSafe(),
			Request:  eventRequest{Method: http.MethodPost, Path: "/mitz/notify", Headers: redact.headers(r.Header), Body: redact.body(body, contentType)},
			Response: eventResponse{Status: http.StatusNoContent, Headers: map[string]string{}, CopyRemoved: &removed},
			Outcome:  "ok", TS: time.Now().UTC(),
		}
		if !c.runtime.handOff(patient.Key, notification.ID, event) {
			event.ActionID = randomURLSafe()
			c.publishToHolder(patient.Key, event, resets)
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

// publishToHolder appends an event that did not come from a session's own
// request to the run of whoever holds the patient, and reports whether it did.
// The registry lookup is finished before the run and session stores are
// touched, which keeps their order (session, run, registry) intact; the append
// runs under the session-liveness guard the capture path uses. An event of a
// notification accepted before a reset is dropped: the run it would land in is
// not the one it belongs to. The check of the reset counter and the append are
// fenced together, so a reset is either over before the check or starts after
// the append.
func (c Config) publishToHolder(patientKey string, event stepEvent, resets uint64) bool {
	owner, ok := c.Locks.Owner(patientKey)
	if !ok || c.Runs == nil || c.sessions == nil {
		return false
	}
	runID, sessionID, ok := c.Runs.runOf(owner, patientKey)
	if !ok {
		return false
	}
	published := false
	c.runtime.publishUnlessReset(patientKey, resets, func() {
		c.sessions.storeIfLive(sessionID, func() { published = c.Runs.append(runID, event) })
	})
	return published
}
