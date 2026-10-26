package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"sync"
	"time"
)

// What one evidence read may take, and what all the reads of one retrieval chain
// may take together. Variables so a test can shorten them; nothing else
// reassigns them.
var (
	evidenceReadTimeout = 2 * time.Second
	evidenceChainBudget = 4 * time.Second
)

func setEvidenceTimeouts(read, chain time.Duration) (restore func()) {
	previousRead, previousChain := evidenceReadTimeout, evidenceChainBudget
	evidenceReadTimeout, evidenceChainBudget = read, chain
	return func() { evidenceReadTimeout, evidenceChainBudget = previousRead, previousChain }
}

// mitzDecision is one entry of the mock Mitz's decision log.
type mitzDecision struct {
	Time          time.Time `json:"time"`
	DurationMs    int64     `json:"durationMs"`
	TraceID       string    `json:"traceId"`
	PatientID     string    `json:"patientId"`
	DataHolderURA string    `json:"dataHolderUra"`
	RequesterURA  string    `json:"requesterUra"`
	Decision      string    `json:"decision"`
	Status        int       `json:"status"`
	Request       string    `json:"request"`
	Response      string    `json:"response"`
}

// consentEvidence establishes, per source request, what Mitz answered the
// source about it. The request carries a trace id of its own; the source's PEP
// passes its headers to the PDP, a Knooppunt that propagates trace context puts
// the same id on its closed question, and the mock Mitz records it. Reading the
// log by that id is the only attribution: never timing, never counting.
type consentEvidence struct {
	read func(context.Context, string) ([]mitzDecision, error)

	mu   sync.Mutex
	left time.Duration
}

type consentEvidenceKey struct{}

// withConsentEvidence makes the source requests sent under ctx carry a trace id
// and have their consent answers looked up. One call is one retrieval chain: the
// budget for reads is shared by every request sent under the returned context.
func withConsentEvidence(ctx context.Context, read func(context.Context, string) ([]mitzDecision, error)) context.Context {
	if read == nil {
		return ctx
	}
	return context.WithValue(ctx, consentEvidenceKey{}, &consentEvidence{read: read, left: evidenceChainBudget})
}

func consentEvidenceFrom(ctx context.Context) *consentEvidence {
	evidence, _ := ctx.Value(consentEvidenceKey{}).(*consentEvidence)
	return evidence
}

// newTraceparent returns a W3C traceparent header value with a fresh trace id,
// and that id. ok is false when the system's random source failed.
func newTraceparent() (header, traceID string, ok bool) {
	var ids [24]byte
	if _, err := rand.Read(ids[:]); err != nil {
		return "", "", false
	}
	traceID, spanID := hex.EncodeToString(ids[:16]), hex.EncodeToString(ids[16:])
	return "00-" + traceID + "-" + spanID + "-01", traceID, true
}

// withTraceparent returns a copy of req carrying header as its traceparent.
func withTraceparent(req *http.Request, header string) *http.Request {
	clone := req.Clone(req.Context())
	clone.Header.Del("Tracestate")
	clone.Header.Set("Traceparent", header)
	return clone
}

// decisions returns the entries recorded under traceID, or nothing: when the
// chain's budget is used up, when the read fails, when it finds none. The
// deadline is this read's own. It does not derive from the clinical request's
// context, which may already be over when the response has been read.
func (e *consentEvidence) decisions(traceID string) []mitzDecision {
	e.mu.Lock()
	timeout := min(evidenceReadTimeout, e.left)
	e.mu.Unlock()
	if timeout <= 0 {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	started := time.Now()
	found, err := e.read(ctx, traceID)
	e.mu.Lock()
	e.left -= time.Since(started)
	e.mu.Unlock()
	if err != nil {
		return nil
	}
	return found
}

// consentCheckEvent turns one log entry into the event of the closed question
// the source's Knooppunt asked about parent's request.
func consentCheckEvent(parent stepEvent, decision mitzDecision, redact eventRedactor) stepEvent {
	event := stepEvent{
		GF: "consent", Actor: "knooppunt", Purpose: "consent-check",
		ActionID: parent.ActionID, Action: parent.Action, ResourceType: parent.ResourceType,
		CallID: randomURLSafe(), ParentCallID: parent.CallID,
		Request: eventRequest{Method: http.MethodPost, Path: "/geslotenautorisatievraag/xacml3",
			Headers: map[string]string{}, Body: redact.body([]byte(decision.Request), "text/xml")},
		Response: eventResponse{Status: decision.Status, Headers: map[string]string{},
			Body: redact.body([]byte(decision.Response), "application/xml"), ConsentDecision: decision.Decision},
		Outcome: "deny", DurationMs: decision.DurationMs, TS: decision.Time.UTC(),
	}
	switch {
	case decision.Status < 200 || decision.Status > 299:
		event.Outcome = "error"
	case decision.Decision == "Permit":
		event.Outcome = "ok"
	}
	return event
}
