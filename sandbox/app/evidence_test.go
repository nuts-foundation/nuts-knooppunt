package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nuts-foundation/nuts-knooppunt/test/testdata/vectors/nvi"
	"github.com/stretchr/testify/require"
)

var traceparentHeader = regexp.MustCompile(`^00-([0-9a-f]{32})-[0-9a-f]{16}-01$`)

// evidenceHarness is a source that records the traceparent of each request it
// serves, and a decision reader that answers from a table keyed by trace id.
type evidenceHarness struct {
	mu        sync.Mutex
	source    *httptest.Server
	seen      []string // trace ids, in request order
	decisions map[string][]mitzDecision
	readErr   error
	readDelay time.Duration
	reads     int
	events    []stepEvent
}

func newEvidenceHarness(t *testing.T, status int) *evidenceHarness {
	t.Helper()
	h := &evidenceHarness{decisions: map[string][]mitzDecision{}}
	h.source = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		match := traceparentHeader.FindStringSubmatch(r.Header.Get("Traceparent"))
		h.mu.Lock()
		if match != nil {
			h.seen = append(h.seen, match[1])
		} else {
			h.seen = append(h.seen, "")
		}
		h.mu.Unlock()
		w.Header().Set("Content-Type", "application/fhir+json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(`{"resourceType":"Bundle","type":"searchset"}`))
	}))
	t.Cleanup(h.source.Close)
	return h
}

func (h *evidenceHarness) read(ctx context.Context, traceID string) ([]mitzDecision, error) {
	h.mu.Lock()
	h.reads++
	delay, err, found := h.readDelay, h.readErr, h.decisions[traceID]
	h.mu.Unlock()
	select {
	case <-time.After(delay):
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	return found, err
}

// call sends one source request through the capture transport, as a retrieval
// does, and waits until its events are published.
func (h *evidenceHarness) call(t *testing.T, ctx context.Context) {
	t.Helper()
	emit := func(event stepEvent) { h.mu.Lock(); h.events = append(h.events, event); h.mu.Unlock() }
	ctx = withEventCapture(ctx, emit, true)
	ctx = withEventAction(ctx, "retrieve")
	ctx = withEventResourceType(ctx, "Patient")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, h.source.URL+"/fhir/Patient", nil)
	require.NoError(t, err)
	res, err := (&http.Client{Transport: newCaptureTransport("exchange", nil)}).Do(req)
	require.NoError(t, err)
	require.NoError(t, res.Body.Close())
}

func permit(traceID string) mitzDecision {
	return mitzDecision{Time: time.Date(2026, 10, 12, 14, 2, 11, 0, time.UTC), DurationMs: 3, TraceID: traceID,
		PatientID: "999911120", DataHolderURA: "00000020", RequesterURA: "00000010", Decision: "Permit", Status: 200,
		Request: `<Request>999911120</Request>`, Response: `<Decision>Permit</Decision>`}
}

func TestEvidence_EachSourceRequestCarriesItsOwnTraceID(t *testing.T) {
	h := newEvidenceHarness(t, http.StatusOK)
	ctx := withConsentEvidence(context.Background(), h.read)
	h.call(t, ctx)
	h.call(t, ctx)

	require.Len(t, h.seen, 2)
	require.NotEmpty(t, h.seen[0])
	require.NotEmpty(t, h.seen[1])
	require.NotEqual(t, h.seen[0], h.seen[1], "one trace id per request, also for the pages of one search")
}

// The pages of a search are requests of their own. A continuation is sent from
// another place in the code than the search it continues (runPageAt), so this
// runs a real retrieval over a source that pages, and not the helper above.
func TestEvidence_EveryPageOfASearchCarriesItsOwnTraceIDAndGetsItsOwnCheck(t *testing.T) {
	var mu sync.Mutex
	var sent, asked []string // trace ids: as the source received them, as the log was read
	conditionRequests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := ""
		if match := traceparentHeader.FindStringSubmatch(r.Header.Get("Traceparent")); match != nil {
			id = match[1]
		}
		mu.Lock()
		sent = append(sent, id)
		body := bundleOf(zonnebloemPatient)
		if !strings.HasPrefix(r.URL.Path, "/fhir/Patient") {
			conditionRequests++
			next := ""
			if conditionRequests == 1 {
				next = `,"link":[{"relation":"next","url":"http://` + r.Host + `/fhir/Condition?page=2"}]`
			}
			body = `{"resourceType":"Bundle","type":"searchset","entry":[{"resource":` + diabetes + `}]` + next + `}`
		}
		mu.Unlock()
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(server.Close)

	var events []stepEvent
	ctx := withEventCapture(t.Context(), func(event stepEvent) { mu.Lock(); events = append(events, event); mu.Unlock() }, true)
	ctx = withEventAction(ctx, "retrieve")
	ctx = withConsentEvidence(ctx, func(_ context.Context, traceID string) ([]mitzDecision, error) {
		mu.Lock()
		asked = append(asked, traceID)
		mu.Unlock()
		return []mitzDecision{permit(traceID)}, nil
	})
	source := sourceAddress{URA: "00000020", Name: "Zorgcentrum De Zonnebloem", Address: server.URL + "/fhir"}

	result, err := retrieveBGZ(ctx, source, "the-token", "999900006", []string{nvi.CategoryPatient, nvi.CategoryCondition})

	require.NoError(t, err)
	require.Equal(t, 2, conditionRequests, "the search and its second page")
	require.Len(t, result.Items, 2, "both pages reached the record")
	distinct := map[string]bool{}
	for _, id := range sent {
		require.NotEmpty(t, id, "every source request carries a trace id")
		distinct[id] = true
	}
	require.Len(t, distinct, len(sent), "one trace id per request, also for the second page of one search")
	require.Equal(t, sent, asked, "the log is read once per request, under that request's own id")

	require.Len(t, events, 2*len(sent), "every request is followed by its own consent check")
	for i := 0; i < len(events); i += 2 {
		parent, child := events[i], events[i+1]
		require.Equal(t, "exchange", parent.GF)
		require.Equal(t, "consent-check", child.Purpose)
		require.Equal(t, parent.CallID, child.ParentCallID)
		require.Equal(t, parent.ActionID, child.ActionID)
		require.Equal(t, parent.Action, child.Action)
		require.Equal(t, parent.ResourceType, child.ResourceType)
	}
	continuation := events[len(events)-2]
	require.Contains(t, continuation.Request.Path, "page=2", "the last request is the continuation")
	require.Equal(t, "Condition", continuation.ResourceType, "and it keeps its search's resource type")
}

func TestEvidence_WithoutAReaderNoTraceIDIsSent(t *testing.T) {
	h := newEvidenceHarness(t, http.StatusOK)
	h.call(t, context.Background())
	require.Equal(t, []string{""}, h.seen)
	require.Len(t, h.events, 1)
}

func TestEvidence_AttachesTheAnswerRecordedUnderTheRequestsTraceID(t *testing.T) {
	h := newEvidenceHarness(t, http.StatusOK)
	ctx := withConsentEvidence(context.Background(), func(ctx context.Context, traceID string) ([]mitzDecision, error) {
		return []mitzDecision{permit(traceID)}, nil
	})
	h.call(t, ctx)

	require.Len(t, h.events, 2, "the source request and its consent check")
	parent, child := h.events[0], h.events[1]
	require.Equal(t, "exchange", parent.GF)
	require.Equal(t, "consent", child.GF)
	require.Equal(t, "knooppunt", child.Actor)
	require.Equal(t, "consent-check", child.Purpose)
	require.Equal(t, parent.CallID, child.ParentCallID)
	require.NotEmpty(t, child.CallID)
	require.NotEqual(t, parent.CallID, child.CallID)
	require.Equal(t, parent.ActionID, child.ActionID)
	require.Equal(t, parent.Action, child.Action)
	require.Equal(t, "Patient", child.ResourceType)
	require.Equal(t, "ok", child.Outcome)
	require.Equal(t, "Permit", child.Response.ConsentDecision)
	require.Equal(t, http.StatusOK, child.Response.Status)
	require.Equal(t, http.MethodPost, child.Request.Method)
	require.Equal(t, "/geslotenautorisatievraag/xacml3", child.Request.Path)
	require.Equal(t, int64(3), child.DurationMs)
	require.Equal(t, time.Date(2026, 10, 12, 14, 2, 11, 0, time.UTC), child.TS)
	require.Contains(t, child.Response.Body, "<Decision>Permit</Decision>")
}

func TestEvidence_DenyAndErrorAnswersHaveTheirOutcome(t *testing.T) {
	for _, tc := range []struct {
		name     string
		decision string
		status   int
		outcome  string
	}{
		{"deny", "Deny", 200, "deny"},
		{"another decision", "Indeterminate", 200, "deny"},
		{"error answer", "", 500, "error"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newEvidenceHarness(t, http.StatusForbidden)
			ctx := withConsentEvidence(context.Background(), func(ctx context.Context, traceID string) ([]mitzDecision, error) {
				d := permit(traceID)
				d.Decision, d.Status = tc.decision, tc.status
				return []mitzDecision{d}, nil
			})
			h.call(t, ctx)
			require.Len(t, h.events, 2)
			require.Equal(t, tc.outcome, h.events[1].Outcome)
			require.Equal(t, tc.decision, h.events[1].Response.ConsentDecision)
		})
	}
}

// An answer recorded for an earlier request that already timed out carries that
// request's trace id, so no later request finds it.
func TestEvidence_ALateAnswerForAnEarlierRequestIsAttachedToNoLaterOne(t *testing.T) {
	h := newEvidenceHarness(t, http.StatusOK)
	ctx := withConsentEvidence(context.Background(), h.read)
	h.call(t, ctx) // A: nothing recorded yet
	h.mu.Lock()
	h.decisions[h.seen[0]] = []mitzDecision{permit(h.seen[0])} // A's answer lands late
	h.mu.Unlock()
	h.call(t, ctx) // B: never reached Mitz

	require.Len(t, h.events, 2, "two source requests and no consent check")
	for _, event := range h.events {
		require.Equal(t, "exchange", event.GF)
	}
}

func TestEvidence_NoEntryAndAFailedReadAttachNothingAndDoNotFailTheCall(t *testing.T) {
	h := newEvidenceHarness(t, http.StatusOK)
	h.call(t, withConsentEvidence(context.Background(), h.read))
	require.Len(t, h.events, 1)

	h.readErr = errors.New("connection refused")
	h.call(t, withConsentEvidence(context.Background(), h.read))
	require.Len(t, h.events, 2)
	require.Equal(t, "ok", h.events[1].Outcome, "the clinical call is unaffected")
}

func TestEvidence_OneChainSpendsAtMostItsBudgetOnReads(t *testing.T) {
	h := newEvidenceHarness(t, http.StatusOK)
	h.readDelay = time.Minute // every read runs into its deadline
	restore := setEvidenceTimeouts(40*time.Millisecond, 100*time.Millisecond)
	defer restore()

	ctx := withConsentEvidence(context.Background(), h.read)
	started := time.Now()
	for i := 0; i < 8; i++ {
		h.call(t, ctx)
	}
	// The number of reads is the assertion: a 100 ms budget pays for three reads
	// of 40 ms at most. The elapsed time is only a loose bound against a read
	// that ignores its deadline altogether.
	require.LessOrEqual(t, h.reads, 3, "reads stop once the budget is used up")
	require.Less(t, time.Since(started), 5*time.Second, "a stalled read ends at its deadline")
	require.Len(t, h.events, 8)
}

func TestEvidence_TheReadDeadlineDoesNotDeriveFromTheClinicalRequests(t *testing.T) {
	h := newEvidenceHarness(t, http.StatusOK)
	ctx, cancel := context.WithCancel(context.Background())
	evidenceCtx := withConsentEvidence(ctx, func(readCtx context.Context, traceID string) ([]mitzDecision, error) {
		cancel() // the clinical request's context ends while the read runs
		if err := readCtx.Err(); err != nil {
			return nil, err
		}
		return []mitzDecision{permit(traceID)}, nil
	})
	h.call(t, evidenceCtx)
	require.Len(t, h.events, 2, "the evidence read survived the clinical context")
}
