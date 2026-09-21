package main

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"go.opentelemetry.io/otel/trace"
	collector "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	common "go.opentelemetry.io/proto/otlp/common/v1"
	resource "go.opentelemetry.io/proto/otlp/resource/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
	"google.golang.org/protobuf/proto"
)

func traceString(key, value string) *common.KeyValue {
	return &common.KeyValue{Key: key, Value: &common.AnyValue{Value: &common.AnyValue_StringValue{StringValue: value}}}
}
func traceInt(key string, value int64) *common.KeyValue {
	return &common.KeyValue{Key: key, Value: &common.AnyValue{Value: &common.AnyValue_IntValue{IntValue: value}}}
}
func traceFixture(t *testing.T, b *eventTraceBridge, emit func(stepEvent), live func() bool) (*http.Request, *collector.ExportTraceServiceRequest) {
	t.Helper()
	req := httptest.NewRequest("POST", "http://knooppunt/nvi/fhir/List", strings.NewReader("original body"))
	req.Header.Set("Authorization", "Bearer private")
	req.Header.Set("Traceparent", "00-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa-bbbbbbbbbbbbbbbb-01")
	req.Header.Set("Tracestate", "untrusted=value")
	prepared := b.prepare(req, stepEvent{GF: "localization", ActionID: "action-1", Action: "share", Purpose: "registration", ResourceType: "Patient", CallID: "parent-1"}, emit, live)
	parts := strings.Split(prepared.Header.Get("Traceparent"), "-")
	if len(parts) != 4 || parts[1] == "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" {
		t.Fatal("prepare must generate fresh trace authority")
	}
	traceID, err := hex.DecodeString(parts[1])
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	span := &tracepb.Span{TraceId: traceID, SpanId: []byte{1, 2, 3, 4, 5, 6, 7, 8}, Kind: tracepb.Span_SPAN_KIND_CLIENT, StartTimeUnixNano: uint64(now.Add(-20 * time.Millisecond).UnixNano()), EndTimeUnixNano: uint64(now.UnixNano()), Attributes: []*common.KeyValue{traceString("http.request.method", "POST"), traceString("url.full", "http://mock-prs:8080/oprf/eval?secret=do-not-retain"), traceInt("http.response.status_code", 200), traceString("private.body", "do-not-retain")}, Status: &tracepb.Status{Message: "do-not-retain"}}
	batch := &collector.ExportTraceServiceRequest{ResourceSpans: []*tracepb.ResourceSpans{{Resource: &resource.Resource{Attributes: []*common.KeyValue{traceString("service.name", "nuts-knooppunt")}}, ScopeSpans: []*tracepb.ScopeSpans{{Spans: []*tracepb.Span{span}}}}}}
	return prepared, batch
}
func tracePost(t *testing.T, b *eventTraceBridge, batch *collector.ExportTraceServiceRequest) *httptest.ResponseRecorder {
	t.Helper()
	data, err := proto.Marshal(batch)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("POST", "/v1/traces", bytes.NewReader(data))
	req.Header.Set("Authorization", "Bearer test-ingest-token")
	req.Header.Set("Content-Type", "application/x-protobuf")
	rec := httptest.NewRecorder()
	b.handler().ServeHTTP(rec, req)
	return rec
}
func traceBridge(t *testing.T) *eventTraceBridge {
	t.Helper()
	b, err := newEventTraceBridge("http://mock-prs:8080", "nuts-knooppunt", "test-ingest-token")
	if err != nil {
		t.Fatal(err)
	}
	return b
}
func traceSpan(batch *collector.ExportTraceServiceRequest) *tracepb.Span {
	return batch.ResourceSpans[0].ScopeSpans[0].Spans[0]
}

func TestTraceBridgeProjectsCorrelatedSpanOnce(t *testing.T) {
	b := traceBridge(t)
	var events []stepEvent
	req, batch := traceFixture(t, b, func(e stepEvent) { events = append(events, e) }, func() bool { return true })
	if req.Header.Get("Authorization") != "Bearer private" || req.Header.Get("Tracestate") != "" {
		t.Fatal("request headers not safely preserved")
	}
	body, _ := io.ReadAll(req.Body)
	if string(body) != "original body" {
		t.Fatal("request body changed")
	}
	for range 2 {
		rec := tracePost(t, b, batch)
		if rec.Code != 200 {
			t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
		}
		if err := proto.Unmarshal(rec.Body.Bytes(), &collector.ExportTraceServiceResponse{}); err != nil {
			t.Fatal(err)
		}
	}
	if len(events) != 1 {
		t.Fatalf("got %d events, want 1", len(events))
	}
	e := events[0]
	if e.GF != "pseudonym" || e.Actor != "knooppunt" || e.ActionID != "action-1" || e.Action != "share" || e.ParentCallID != "parent-1" || e.CallID == "" || e.CallID == e.ParentCallID || e.Purpose != "pseudonymization" || e.ResourceType != "Patient" {
		t.Fatalf("wrong metadata: %+v", e)
	}
	if e.Outcome != "ok" || e.Response.Status != 200 || e.DurationMs != 20 || e.Request.Method != "POST" || e.Request.Path != "/oprf/eval" || e.TS.UnixNano() != int64(traceSpan(batch).EndTimeUnixNano) {
		t.Fatalf("wrong observed result: %+v", e)
	}
	encoded, _ := json.Marshal(e)
	if strings.Contains(string(encoded), "do-not-retain") || e.Request.Body != nil || e.Response.Body != nil || len(e.Request.Headers) != 0 || len(e.Response.Headers) != 0 {
		t.Fatalf("unsafe projection: %s", encoded)
	}
}

func TestTraceBridgeRejectsUnrelatedAndInvalidSpans(t *testing.T) {
	cases := map[string]func(*collector.ExportTraceServiceRequest){
		"service": func(b *collector.ExportTraceServiceRequest) {
			b.ResourceSpans[0].Resource.Attributes[0] = traceString("service.name", "other")
		},
		"no service": func(b *collector.ExportTraceServiceRequest) { b.ResourceSpans[0].Resource = nil },
		"server":     func(b *collector.ExportTraceServiceRequest) { traceSpan(b).Kind = tracepb.Span_SPAN_KIND_SERVER },
		"trace":      func(b *collector.ExportTraceServiceRequest) { traceSpan(b).TraceId = bytes.Repeat([]byte{9}, 16) },
		"zero span":  func(b *collector.ExportTraceServiceRequest) { traceSpan(b).SpanId = make([]byte, 8) },
		"short span": func(b *collector.ExportTraceServiceRequest) { traceSpan(b).SpanId = []byte{1} },
		"method": func(b *collector.ExportTraceServiceRequest) {
			traceSpan(b).Attributes[0] = traceString("http.request.method", "GET")
		},
		"method type": func(b *collector.ExportTraceServiceRequest) {
			traceSpan(b).Attributes[0] = traceInt("http.request.method", 1)
		},
		"host": func(b *collector.ExportTraceServiceRequest) {
			traceSpan(b).Attributes[1] = traceString("url.full", "http://evil/oprf/eval")
		},
		"port": func(b *collector.ExportTraceServiceRequest) {
			traceSpan(b).Attributes[1] = traceString("url.full", "http://mock-prs:9090/oprf/eval")
		},
		"path": func(b *collector.ExportTraceServiceRequest) {
			traceSpan(b).Attributes[1] = traceString("url.full", "http://mock-prs:8080/oauth/token")
		},
		"userinfo": func(b *collector.ExportTraceServiceRequest) {
			traceSpan(b).Attributes[1] = traceString("url.full", "http://secret@mock-prs:8080/oprf/eval")
		},
		"scheme": func(b *collector.ExportTraceServiceRequest) {
			traceSpan(b).Attributes[1] = traceString("url.full", "https://mock-prs:8080/oprf/eval")
		},
		"duplicate attribute": func(b *collector.ExportTraceServiceRequest) {
			s := traceSpan(b)
			s.Attributes = append(s.Attributes, traceString("http.request.method", "POST"))
		},
		"zero time": func(b *collector.ExportTraceServiceRequest) { traceSpan(b).StartTimeUnixNano = 0 },
		"reversed time": func(b *collector.ExportTraceServiceRequest) {
			s := traceSpan(b)
			s.EndTimeUnixNano = s.StartTimeUnixNano - 1
		},
		"future": func(b *collector.ExportTraceServiceRequest) {
			s := traceSpan(b)
			s.EndTimeUnixNano = uint64(time.Now().Add(time.Hour).UnixNano())
		},
		"ancient": func(b *collector.ExportTraceServiceRequest) {
			s := traceSpan(b)
			s.StartTimeUnixNano = 1
			s.EndTimeUnixNano = 1000
		},
		"invalid status": func(b *collector.ExportTraceServiceRequest) {
			traceSpan(b).Attributes[2] = traceInt("http.response.status_code", 999)
		},
		"missing outcome": func(b *collector.ExportTraceServiceRequest) { traceSpan(b).Attributes = traceSpan(b).Attributes[:2] },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			b := traceBridge(t)
			count := 0
			_, batch := traceFixture(t, b, func(stepEvent) { count++ }, func() bool { return true })
			mutate(batch)
			rec := tracePost(t, b, batch)
			if rec.Code != 200 || count != 0 {
				t.Fatalf("status=%d events=%d", rec.Code, count)
			}
		})
	}
}
func TestTraceBridgeObservedFailures(t *testing.T) {
	for _, tc := range []struct {
		name      string
		status    int64
		spanError bool
		outcome   string
	}{{"denied", 403, false, "deny"}, {"server error", 500, true, "error"}, {"transport", 0, true, "error"}, {"read error", 200, true, "error"}} {
		t.Run(tc.name, func(t *testing.T) {
			b := traceBridge(t)
			var events []stepEvent
			_, batch := traceFixture(t, b, func(e stepEvent) { events = append(events, e) }, func() bool { return true })
			s := traceSpan(batch)
			s.Attributes[2] = traceInt("http.response.status_code", tc.status)
			if tc.status == 0 {
				s.Attributes = append(s.Attributes[:2], s.Attributes[3:]...)
			}
			if tc.spanError {
				s.Status.Code = tracepb.Status_STATUS_CODE_ERROR
			}
			tracePost(t, b, batch)
			if len(events) != 1 || events[0].Outcome != tc.outcome || events[0].Response.Status != int(tc.status) {
				t.Fatalf("wrong result: %+v", events)
			}
		})
	}
}
func TestTraceBridgeIngestBoundary(t *testing.T) {
	for _, tc := range []struct {
		name, method, path, token, content, body string
		want                                     int
	}{
		{"missing token", "POST", "/v1/traces", "", "application/x-protobuf", "", 401},
		{"wrong token", "POST", "/v1/traces", "Bearer wrong", "application/x-protobuf", "", 401},
		{"method", "GET", "/v1/traces", "Bearer test-ingest-token", "application/x-protobuf", "", 405},
		{"path", "POST", "/events", "Bearer test-ingest-token", "application/x-protobuf", "", 404},
		{"content", "POST", "/v1/traces", "Bearer test-ingest-token", "application/json", "{}", 415},
		{"malformed", "POST", "/v1/traces", "Bearer test-ingest-token", "application/x-protobuf", "\xff", 400},
		{"huge", "POST", "/v1/traces", "Bearer test-ingest-token", "application/x-protobuf", strings.Repeat("a", 2*1024*1024), 413},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
			req.Header.Set("Authorization", tc.token)
			req.Header.Set("Content-Type", tc.content)
			rec := httptest.NewRecorder()
			traceBridge(t).handler().ServeHTTP(rec, req)
			if rec.Code != tc.want {
				t.Fatalf("status %d, want %d", rec.Code, tc.want)
			}
		})
	}
}
func TestTraceBridgeConcurrentDedupAndCallbacks(t *testing.T) {
	b := traceBridge(t)
	var count atomic.Int32
	_, batch := traceFixture(t, b, func(stepEvent) { b.mu.Lock(); b.mu.Unlock(); count.Add(1) }, func() bool { b.mu.Lock(); b.mu.Unlock(); return true })
	var wg sync.WaitGroup
	for range 20 {
		wg.Add(1)
		go func() { defer wg.Done(); tracePost(t, b, batch) }()
	}
	wg.Wait()
	if count.Load() != 1 {
		t.Fatalf("got %d callbacks", count.Load())
	}
}
func TestTraceBridgeClearedAndExpiredCorrelations(t *testing.T) {
	for _, expired := range []bool{false, true} {
		t.Run(map[bool]string{false: "cleared", true: "expired"}[expired], func(t *testing.T) {
			b := traceBridge(t)
			live := true
			count := 0
			_, batch := traceFixture(t, b, func(stepEvent) { count++ }, func() bool { return live })
			if expired {
				b.now = func() time.Time { return time.Now().Add(10 * time.Minute) }
			} else {
				live = false
			}
			tracePost(t, b, batch)
			if count != 0 {
				t.Fatal("stale correlation emitted")
			}
			b.mu.Lock()
			defer b.mu.Unlock()
			if len(b.correlations) != 0 {
				t.Fatal("stale callbacks retained")
			}
		})
	}
}
func TestTraceBridgeInputAndStorageBounds(t *testing.T) {
	b := traceBridge(t)
	var emitted atomic.Int32
	_, batch := traceFixture(t, b, func(stepEvent) { emitted.Add(1) }, func() bool { return true })
	s := traceSpan(batch)
	for i := 0; i < eventTraceMaxSpans; i++ {
		batch.ResourceSpans[0].ScopeSpans[0].Spans = append(batch.ResourceSpans[0].ScopeSpans[0].Spans, s)
	}
	if rec := tracePost(t, b, batch); rec.Code != 413 || emitted.Load() != 0 {
		t.Fatalf("oversized batch: status %d, emitted %d", rec.Code, emitted.Load())
	}
	for range eventTraceMaxCorrelations + 5 {
		traceFixture(t, b, func(stepEvent) {}, func() bool { return true })
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.correlations) > eventTraceMaxCorrelations {
		t.Fatal("unbounded correlations")
	}
}
func TestTraceBridgeConfigurationAndNonLocalization(t *testing.T) {
	for _, u := range []string{"", "://", "ftp://prs", "http://user:pass@prs", "http://prs?secret=1", "http://prs/#fragment"} {
		if _, err := newEventTraceBridge(u, "service", "token"); err == nil {
			t.Errorf("accepted invalid endpoint %q", u)
		}
	}
	if _, err := newEventTraceBridge("http://prs", "service", ""); err == nil {
		t.Error("accepted empty credential")
	}
	b := traceBridge(t)
	req := httptest.NewRequest("GET", "http://example", nil)
	if b.prepare(req, stepEvent{GF: "exchange"}, func(stepEvent) {}, func() bool { return true }) != req {
		t.Error("instrumented unrelated request")
	}
}

func TestTraceBridgeDedupBoundDropsNewSpansWithoutReplayingOld(t *testing.T) {
	b := traceBridge(t)
	count := 0
	_, batch := traceFixture(t, b, func(stepEvent) { count++ }, func() bool { return true })
	original := proto.Clone(batch).(*collector.ExportTraceServiceRequest)
	for i := 0; i < eventTraceMaxDedup+10; i++ {
		s := traceSpan(batch)
		s.SpanId = []byte{1, 2, 3, 4, 5, 6, byte(i >> 8), byte(i)}
		tracePost(t, b, batch)
	}
	tracePost(t, b, original)
	if count != eventTraceMaxDedup {
		t.Fatalf("dedup capacity failed: got %d events", count)
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, c := range b.correlations {
		if len(c.seen) > eventTraceMaxDedup {
			t.Fatal("unbounded dedup")
		}
	}
}

func TestTraceBridgeRequestsOwnIndependentCorrelations(t *testing.T) {
	b := traceBridge(t)
	first, second := 0, 0
	_, batch1 := traceFixture(t, b, func(stepEvent) { first++ }, func() bool { return true })
	_, batch2 := traceFixture(t, b, func(stepEvent) { second++ }, func() bool { return true })
	if bytes.Equal(traceSpan(batch1).TraceId, traceSpan(batch2).TraceId) {
		t.Fatal("same-URL calls shared authority")
	}
	tracePost(t, b, batch2)
	if first != 0 || second != 1 {
		t.Fatalf("wrong owner: %d, %d", first, second)
	}
	tracePost(t, b, batch1)
	if first != 1 || second != 1 {
		t.Fatalf("lost first owner: %d, %d", first, second)
	}
}

func TestTraceBridgePreservesOriginalAndContext(t *testing.T) {
	b := traceBridge(t)
	req := httptest.NewRequest("POST", "http://knooppunt/nvi/fhir/List", strings.NewReader("request"))
	req.Header.Set("Traceparent", "browser-supplied")
	req.Header.Set("Tracestate", "browser-state")
	cloned := b.prepare(req, stepEvent{GF: "localization", CallID: "call"}, func(stepEvent) {}, func() bool { return true })
	if req.Header.Get("Traceparent") != "browser-supplied" || req.Header.Get("Tracestate") != "browser-state" {
		t.Fatal("original request mutated")
	}
	if cloned == req || cloned.Body != req.Body || cloned.URL.String() != req.URL.String() || cloned.Method != req.Method {
		t.Fatal("request was not safely cloned")
	}
	if !trace.SpanContextFromContext(cloned.Context()).IsValid() || !trace.SpanContextFromContext(cloned.Context()).IsSampled() {
		t.Fatal("missing sampled local context")
	}
}

func TestTraceBridgeRejectsInvalidSpanStatus(t *testing.T) {
	b := traceBridge(t)
	count := 0
	_, batch := traceFixture(t, b, func(stepEvent) { count++ }, func() bool { return true })
	traceSpan(batch).Status.Code = tracepb.Status_StatusCode(99)
	tracePost(t, b, batch)
	if count != 0 {
		t.Fatal("unknown span status produced an observed success")
	}
}

func TestTraceBridgeIdleExpiryReleasesCallbacks(t *testing.T) {
	b := traceBridge(t)
	traceFixture(t, b, func(stepEvent) {}, func() bool { return true })
	b.mu.Lock()
	for _, correlation := range b.correlations {
		correlation.timer.Reset(time.Millisecond)
	}
	b.mu.Unlock()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		b.mu.Lock()
		remaining := len(b.correlations)
		b.mu.Unlock()
		if remaining == 0 {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("idle expiry retained callback references")
}
