package main

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"io"
	"mime"
	"net/http"
	"net/url"
	"path"
	"strings"
	"sync"
	"time"

	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
	collector "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	common "go.opentelemetry.io/proto/otlp/common/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
	"google.golang.org/protobuf/proto"
)

const (
	eventTraceMaxBytes        = 1024 * 1024
	eventTraceMaxSpans        = 1024
	eventTraceMaxCorrelations = 512
	eventTraceMaxDedup        = 128
	eventTraceLifetime        = 5 * time.Minute
	eventTraceMaxDuration     = 2 * time.Minute
	eventTraceClockSkew       = 5 * time.Second
)

type eventTraceCorrelation struct {
	parent  stepEvent
	emit    func(stepEvent)
	live    func() bool
	created time.Time
	seen    map[string]struct{}
	timer   *time.Timer
}

type eventTraceBridge struct {
	mu           sync.Mutex
	now          func() time.Time
	correlations map[string]*eventTraceCorrelation
	endpoint     *url.URL
	serviceName  string
	tokenHash    [32]byte
}

func newEventTraceBridge(prsURL, serviceName, token string) (*eventTraceBridge, error) {
	endpoint, err := url.Parse(prsURL)
	if err != nil || endpoint.Hostname() == "" || !eventEnum(endpoint.Scheme, "http", "https") || endpoint.User != nil || endpoint.RawQuery != "" || endpoint.ForceQuery || endpoint.Fragment != "" || endpoint.RawPath != "" {
		return nil, errors.New("invalid sandbox PRS base URL")
	}
	if strings.TrimSpace(token) == "" || len(token) > 512 || strings.ContainsAny(token, "\r\n\t") {
		return nil, errors.New("invalid sandbox OTLP token")
	}
	if serviceName == "" {
		serviceName = "nuts-knooppunt"
	}
	if len(serviceName) > 128 || strings.TrimSpace(serviceName) != serviceName {
		return nil, errors.New("invalid sandbox trace service name")
	}
	endpoint.Path = path.Join(endpoint.Path, "oprf/eval")
	if !strings.HasPrefix(endpoint.Path, "/") {
		endpoint.Path = "/" + endpoint.Path
	}
	return &eventTraceBridge{now: time.Now, correlations: make(map[string]*eventTraceCorrelation), endpoint: endpoint, serviceName: serviceName, tokenHash: sha256.Sum256([]byte("Bearer " + token))}, nil
}

func (b *eventTraceBridge) prepare(req *http.Request, parent stepEvent, emit func(stepEvent), live func() bool) *http.Request {
	if b == nil || req == nil || parent.GF != "localization" || parent.CallID == "" || emit == nil || live == nil {
		return req
	}
	b.prune()
	if !live() {
		return req
	}
	var traceID trace.TraceID
	var spanID trace.SpanID
	if _, err := rand.Read(traceID[:]); err != nil {
		return req
	}
	if _, err := rand.Read(spanID[:]); err != nil {
		return req
	}
	sc := trace.NewSpanContext(trace.SpanContextConfig{TraceID: traceID, SpanID: spanID, TraceFlags: trace.FlagsSampled})
	if !sc.IsValid() {
		return req
	}
	cloned := req.Clone(trace.ContextWithSpanContext(req.Context(), sc))
	cloned.Header.Del("Traceparent")
	cloned.Header.Del("Tracestate")
	propagation.TraceContext{}.Inject(cloned.Context(), propagation.HeaderCarrier(cloned.Header))
	correlation := &eventTraceCorrelation{
		parent: stepEvent{ActionID: parent.ActionID, Action: parent.Action, ResourceType: parent.ResourceType, CallID: parent.CallID},
		emit:   emit, live: live, created: b.now(), seen: make(map[string]struct{}),
	}
	key := string(traceID[:])
	b.mu.Lock()
	if len(b.correlations) >= eventTraceMaxCorrelations {
		var oldestKey string
		var oldest *eventTraceCorrelation
		for id, c := range b.correlations {
			if oldest == nil || c.created.Before(oldest.created) {
				oldestKey, oldest = id, c
			}
		}
		b.removeLocked(oldestKey)
	}
	correlation.timer = time.AfterFunc(eventTraceLifetime, func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		if b.correlations[key] == correlation {
			b.removeLocked(key)
		}
	})
	b.correlations[key] = correlation
	b.mu.Unlock()
	return cloned
}

func (b *eventTraceBridge) removeLocked(key string) {
	if c := b.correlations[key]; c != nil {
		if c.timer != nil {
			c.timer.Stop()
		}
		delete(b.correlations, key)
	}
}

func (b *eventTraceBridge) prune() {
	b.mu.Lock()
	snapshot := make(map[string]*eventTraceCorrelation, len(b.correlations))
	for id, c := range b.correlations {
		snapshot[id] = c
	}
	b.mu.Unlock()
	now := b.now()
	for id, c := range snapshot {
		if now.Sub(c.created) < eventTraceLifetime && c.live() {
			continue
		}
		b.mu.Lock()
		if b.correlations[id] == c {
			b.removeLocked(id)
		}
		b.mu.Unlock()
	}
}

func (b *eventTraceBridge) handler() http.Handler { return http.HandlerFunc(b.serveHTTP) }

func (b *eventTraceBridge) serveHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/v1/traces" {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	provided := sha256.Sum256([]byte(r.Header.Get("Authorization")))
	if subtle.ConstantTimeCompare(provided[:], b.tokenHash[:]) != 1 {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/x-protobuf" || r.Header.Get("Content-Encoding") != "" {
		http.Error(w, "unsupported content type", http.StatusUnsupportedMediaType)
		return
	}
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, eventTraceMaxBytes))
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			http.Error(w, "trace batch too large", http.StatusRequestEntityTooLarge)
		} else {
			http.Error(w, "invalid trace body", http.StatusBadRequest)
		}
		return
	}
	var batch collector.ExportTraceServiceRequest
	if err := (proto.UnmarshalOptions{DiscardUnknown: true, RecursionLimit: 32}).Unmarshal(data, &batch); err != nil {
		http.Error(w, "invalid trace batch", http.StatusBadRequest)
		return
	}
	count := 0
	for _, res := range batch.ResourceSpans {
		for _, scope := range res.GetScopeSpans() {
			count += len(scope.GetSpans())
			if count > eventTraceMaxSpans {
				http.Error(w, "too many spans", http.StatusRequestEntityTooLarge)
				return
			}
		}
	}
	b.prune()
	for _, res := range batch.ResourceSpans {
		service, ok := traceAttribute(res.GetResource().GetAttributes(), "service.name")
		if !ok || service.GetStringValue() != b.serviceName {
			continue
		}
		for _, scope := range res.GetScopeSpans() {
			for _, span := range scope.GetSpans() {
				b.consume(span)
			}
		}
	}
	response, _ := proto.Marshal(&collector.ExportTraceServiceResponse{})
	w.Header().Set("Content-Type", "application/x-protobuf")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(response)
}

// Only a single occurrence of each selected attribute is authoritative.
func traceAttribute(attrs []*common.KeyValue, key string) (*common.AnyValue, bool) {
	var found *common.AnyValue
	for _, attr := range attrs {
		if attr.GetKey() == key {
			if found != nil || attr.GetValue() == nil {
				return nil, false
			}
			found = attr.GetValue()
		}
	}
	return found, found != nil
}

func (b *eventTraceBridge) consume(span *tracepb.Span) {
	if span == nil || span.Kind != tracepb.Span_SPAN_KIND_CLIENT || len(span.TraceId) != 16 || len(span.SpanId) != 8 {
		return
	}
	if code := span.GetStatus().GetCode(); code != tracepb.Status_STATUS_CODE_UNSET && code != tracepb.Status_STATUS_CODE_OK && code != tracepb.Status_STATUS_CODE_ERROR {
		return
	}
	var spanID trace.SpanID
	copy(spanID[:], span.SpanId)
	if !spanID.IsValid() {
		return
	}
	method, ok := traceAttribute(span.Attributes, "http.request.method")
	if !ok || method.GetStringValue() != "POST" {
		return
	}
	fullURL, ok := traceAttribute(span.Attributes, "url.full")
	if !ok {
		return
	}
	endpoint, err := url.Parse(fullURL.GetStringValue())
	if err != nil || endpoint.Scheme != b.endpoint.Scheme || endpoint.Host != b.endpoint.Host || endpoint.Path != b.endpoint.Path || endpoint.RawPath != "" || endpoint.User != nil || endpoint.Fragment != "" {
		return
	}
	status := int64(0)
	statusAttr, hasStatus := traceAttribute(span.Attributes, "http.response.status_code")
	if hasStatus {
		value, valid := statusAttr.Value.(*common.AnyValue_IntValue)
		if !valid || value.IntValue < 100 || value.IntValue > 599 {
			return
		}
		status = value.IntValue
	} else {
		// Missing status is valid only for an observed transport failure. A malformed
		// or duplicate status must not masquerade as an absent status.
		for _, attr := range span.Attributes {
			if attr.GetKey() == "http.response.status_code" {
				return
			}
		}
		if span.GetStatus().GetCode() != tracepb.Status_STATUS_CODE_ERROR {
			return
		}
	}
	now := b.now()
	if span.StartTimeUnixNano == 0 || span.EndTimeUnixNano < span.StartTimeUnixNano || span.EndTimeUnixNano > uint64(now.Add(eventTraceClockSkew).UnixNano()) || span.EndTimeUnixNano-span.StartTimeUnixNano > uint64(eventTraceMaxDuration) {
		return
	}
	traceKey := string(span.TraceId)
	dedupKey := string(span.SpanId)
	b.mu.Lock()
	correlation := b.correlations[traceKey]
	b.mu.Unlock()
	if correlation == nil || !correlation.live() || now.Sub(correlation.created) >= eventTraceLifetime || span.StartTimeUnixNano < uint64(correlation.created.Add(-eventTraceClockSkew).UnixNano()) {
		return
	}
	event := stepEvent{
		GF: "pseudonym", Actor: "knooppunt", ActionID: correlation.parent.ActionID, Action: correlation.parent.Action, ResourceType: correlation.parent.ResourceType,
		Purpose: "pseudonymization", CallID: "prs-" + hex.EncodeToString(span.TraceId) + "-" + hex.EncodeToString(span.SpanId), ParentCallID: correlation.parent.CallID,
		Request: eventRequest{Method: "POST", Path: b.endpoint.Path}, Response: eventResponse{Status: int(status)}, Outcome: "error",
		DurationMs: int64((span.EndTimeUnixNano - span.StartTimeUnixNano) / uint64(time.Millisecond)), TS: time.Unix(0, int64(span.EndTimeUnixNano)).UTC(),
	}
	switch {
	case status == 401 || status == 403:
		event.Outcome = "deny"
	case status >= 200 && status < 300 && span.GetStatus().GetCode() != tracepb.Status_STATUS_CODE_ERROR:
		event.Outcome = "ok"
	}
	b.mu.Lock()
	if b.correlations[traceKey] != correlation || len(correlation.seen) >= eventTraceMaxDedup {
		b.mu.Unlock()
		return
	}
	if _, seen := correlation.seen[dedupKey]; seen {
		b.mu.Unlock()
		return
	}
	correlation.seen[dedupKey] = struct{}{}
	b.mu.Unlock()
	// The emitter performs the final ownership/liveness check atomically with
	// publication, including a reset racing with this final callback.
	correlation.emit(event)
}
