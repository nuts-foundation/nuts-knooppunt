package main

import (
	"context"
	"encoding/json"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/nuts-foundation/nuts-knooppunt/test/testdata/vectors/nvi"
	"github.com/nuts-foundation/nuts-knooppunt/test/testdata/vectors/pool"
)

const eventBodyLimit = 64 * 1024
const eventBSNSystem = "http://fhir.nl/fhir/NamingSystem/bsn"
const eventURASystem = "http://fhir.nl/fhir/NamingSystem/ura"

type stepEvent struct {
	RunID        string        `json:"runId"`
	Seq          uint64        `json:"seq"`
	GF           string        `json:"gf"`
	Actor        string        `json:"actor"`
	Request      eventRequest  `json:"request"`
	Response     eventResponse `json:"response"`
	Outcome      string        `json:"outcome"`
	DurationMs   int64         `json:"durationMs"`
	TS           time.Time     `json:"ts"`
	ActionID     string        `json:"actionId,omitempty"`
	Action       string        `json:"action,omitempty"`
	Purpose      string        `json:"purpose,omitempty"`
	ResourceType string        `json:"resourceType,omitempty"`
	CallID       string        `json:"callId,omitempty"`
	ParentCallID string        `json:"parentCallId,omitempty"`
}

type eventRequest struct {
	Method        string            `json:"method"`
	Path          string            `json:"path"`
	Headers       map[string]string `json:"headers"`
	Body          any               `json:"body"`
	TokenAttached bool              `json:"tokenAttached,omitempty"`
}

type eventResponse struct {
	Status        int               `json:"status"`
	Headers       map[string]string `json:"headers"`
	Body          any               `json:"body"`
	TokenReceived bool              `json:"tokenReceived,omitempty"`
}

type eventCaptureKey struct{}
type eventCapture struct {
	emit            func(stepEvent)
	revealSynthetic bool
	traces          *eventTraceBridge
	live            func() bool
}

func withEventCapture(ctx context.Context, emit func(stepEvent), revealSynthetic bool) context.Context {
	return context.WithValue(ctx, eventCaptureKey{}, eventCapture{emit: emit, revealSynthetic: revealSynthetic})
}

type captureTransport struct {
	gf   string
	base http.RoundTripper
}

func newCaptureTransport(gf string, base http.RoundTripper) http.RoundTripper {
	if base == nil {
		base = http.DefaultTransport
	}
	if !eventEnum(gf, "localization", "consent", "addressing", "authorization", "exchange", "authentication", "pseudonym") {
		gf = "exchange"
	}
	return &captureTransport{gf: gf, base: base}
}

func (t *captureTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	capture, ok := req.Context().Value(eventCaptureKey{}).(eventCapture)
	if !ok || capture.emit == nil {
		return t.base.RoundTrip(req)
	}
	started := time.Now()
	method := req.Method
	if method == "" {
		method = http.MethodGet
	}
	if !eventEnum(method, "GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS") {
		method = "OTHER"
	}
	event := stepEvent{
		GF: t.gf, Actor: "sandbox-backend", Outcome: "error",
		Request:  eventRequest{Method: method, Path: eventPath(req.URL, capture.revealSynthetic), Headers: eventHeaders(req.Header)},
		Response: eventResponse{Headers: map[string]string{}},
	}
	applyEventAction(req.Context(), &event)
	credential := strings.Fields(req.Header.Get("Authorization"))
	event.Request.TokenAttached = t.gf == "exchange" && len(credential) == 2 && strings.EqualFold(credential[0], "Bearer")
	if req.GetBody != nil {
		if body, err := req.GetBody(); err == nil {
			data, readErr := io.ReadAll(io.LimitReader(body, eventBodyLimit+1))
			_ = body.Close()
			if readErr == nil {
				event.Request.Body = eventBody(data, req.Header.Get("Content-Type"), capture.revealSynthetic)
			}
		}
	}
	finish := func(data []byte, complete bool, failed bool) {
		if complete {
			event.Response.Body = eventBody(data, event.Response.Headers["Content-Type"], capture.revealSynthetic)
			if !failed && t.gf == "authorization" && method == http.MethodPost &&
				event.Response.Status == http.StatusOK && strings.HasSuffix(req.URL.Path, "/request-service-access-token") {
				var token struct {
					AccessToken string `json:"access_token"`
				}
				event.Response.TokenReceived = json.Unmarshal(data, &token) == nil && token.AccessToken != ""
			}
		}
		if failed {
			event.Outcome = "error"
		}
		event.DurationMs = time.Since(started).Milliseconds()
		event.TS = time.Now().UTC()
		capture.emit(event)
	}
	if capture.traces != nil {
		req = capture.traces.prepare(req, event, capture.emit, capture.live)
	}
	response, err := t.base.RoundTrip(req)
	if err != nil {
		finish(nil, false, true)
		return response, err
	}
	event.Response.Status = response.StatusCode
	event.Response.Headers = eventHeaders(response.Header)
	switch {
	case response.StatusCode == 401 || response.StatusCode == 403:
		event.Outcome = "deny"
	case response.StatusCode >= 200 && response.StatusCode < 300:
		event.Outcome = "ok"
	}
	if response.Body == nil || response.Body == http.NoBody {
		finish(nil, false, false)
	} else {
		response.Body = &captureBody{body: response.Body, finish: finish, expected: response.ContentLength}
	}
	return response, nil
}

type captureBody struct {
	body     io.ReadCloser
	mu       sync.Mutex
	once     sync.Once
	data     []byte
	size     int64
	expected int64
	overflow bool
	finish   func([]byte, bool, bool)
}

func (b *captureBody) Read(p []byte) (int, error) {
	n, err := b.body.Read(p)
	b.mu.Lock()
	defer b.mu.Unlock()
	b.size += int64(n)
	if !b.overflow {
		if n > eventBodyLimit-len(b.data) {
			b.data = nil
			b.overflow = true
		} else {
			b.data = append(b.data, p[:n]...)
		}
	}
	if err != nil {
		b.publish(err == io.EOF, err != io.EOF)
	}
	return n, err
}

func (b *captureBody) Close() error {
	err := b.body.Close()
	b.mu.Lock()
	defer b.mu.Unlock()
	b.publish(err == nil && b.expected > 0 && b.size == b.expected, err != nil)
	return err
}

func (b *captureBody) publish(complete, failed bool) {
	b.once.Do(func() { b.finish(b.data, complete && !b.overflow, failed); b.data = nil })
}

func eventEnum(value string, allowed ...string) bool {
	for _, candidate := range allowed {
		if value == candidate {
			return true
		}
	}
	return false
}

func eventHeaders(h http.Header) map[string]string {
	out := map[string]string{}
	for _, name := range []string{"Content-Type", "Accept"} {
		mediaType, _, err := mime.ParseMediaType(h.Get(name))
		if err == nil && eventEnum(mediaType, "application/json", "application/fhir+json", "application/x-www-form-urlencoded") {
			out[name] = mediaType
		}
	}
	return out
}

func eventPath(u *url.URL, reveal bool) string {
	if u == nil {
		return "/"
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) > 16 {
		return "/{path}"
	}
	for i, part := range parts {
		if !eventEnum(part, "", "nvi", "mitz", "abonnementen", "fhir", "knpt-mcsd-query", "nuts", "internal", "auth", "v2", "accesstoken", "introspect", "request-service-access-token", "List", "Subscription", "Organization", "Endpoint", "Patient", "Condition", "MedicationRequest", "AllergyIntolerance", "_search") {
			parts[i] = "{id}"
		}
	}
	path := "/" + strings.Join(parts, "/")
	query := eventQuery(u.Query(), reveal)
	if len(query) > 0 {
		path += "?" + query.Encode()
	}
	return path
}

func eventQuery(query url.Values, reveal bool) url.Values {
	out := url.Values{}
	for key, values := range query {
		if len(values) != 1 {
			continue
		}
		value := values[0]
		switch key {
		case "_count":
			n, err := strconv.Atoi(value)
			if err == nil && n >= 1 && n <= 1000 {
				out.Set(key, strconv.Itoa(n))
			}
		case "_include":
			if eventEnum(value, "Organization:endpoint", "MedicationRequest:medication") {
				out.Set(key, value)
			}
		case "_query":
			if value == "otv" {
				out.Set(key, value)
			}
		case "patientid":
			if safe := eventBSN(value, reveal); safe != "" {
				out.Set(key, safe)
			}
		case "identifier", "subject:identifier":
			system, identifier, ok := strings.Cut(value, "|")
			if !ok {
				continue
			}
			if safe := eventIdentifier(system, identifier, reveal); safe != nil {
				out.Set(key, system+"|"+safe["value"].(string))
			}
		case "providerid":
			if eventEnum(value, "00000010", "00000020") {
				out.Set(key, value)
			}
		case "providertype":
			if eventEnum(value, "Z3") {
				out.Set(key, value)
			}
		case "patient", "subject":
			if strings.HasPrefix(value, "Patient/") {
				out.Set(key, "Patient/{id}")
			}
		}
	}
	return out
}

func eventBSN(value string, reveal bool) string {
	if len(value) != 9 {
		return ""
	}
	for _, c := range value {
		if c < '0' || c > '9' {
			return ""
		}
	}
	if reveal {
		for _, patient := range pool.Patients() {
			if value == patient.BSN {
				return value
			}
		}
	}
	return "[redacted]"
}

func eventIdentifier(system, value string, reveal bool) map[string]any {
	safe := ""
	switch system {
	case eventBSNSystem:
		safe = eventBSN(value, reveal)
	case eventURASystem:
		if eventEnum(value, "00000010", "00000020") {
			safe = value
		}
	}
	if safe == "" {
		return nil
	}
	return map[string]any{"system": system, "value": safe}
}

func eventBody(data []byte, contentType string, reveal bool) any {
	if len(data) == 0 || len(data) > eventBodyLimit {
		return nil
	}
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return nil
	}
	if mediaType == "application/x-www-form-urlencoded" {
		values, err := url.ParseQuery(string(data))
		if err != nil {
			return nil
		}
		safe := eventQuery(values, reveal)
		if len(safe) == 0 {
			return nil
		}
		return safe
	}
	if !eventEnum(mediaType, "application/json", "application/fhir+json") {
		return nil
	}
	var body map[string]any
	if err := json.Unmarshal(data, &body); err != nil {
		return nil
	}
	return eventObject(body, reveal, 0)
}

func eventObject(body map[string]any, reveal bool, depth int) any {
	if depth > 4 {
		return nil
	}
	kind, _ := body["resourceType"].(string)
	if _, exists := body["resourceType"]; exists && kind == "" {
		return nil
	}
	out := map[string]any{}
	if kind == "" {
		if active, ok := body["active"].(bool); ok {
			out["active"] = active
		}
		eventString(out, body, "token_type", "Bearer", "DPoP")
		eventString(out, body, "scope", "bgz")
		eventNumber(out, body, "expires_in", 86400)
		if len(out) == 0 {
			return nil
		}
		return out
	}
	if !eventEnum(kind, "Bundle", "Patient", "List", "Subscription", "Organization", "Endpoint", "Condition", "MedicationRequest", "AllergyIntolerance", "OperationOutcome") {
		return nil
	}
	out["resourceType"] = kind
	switch kind {
	case "Bundle":
		eventString(out, body, "type", "searchset", "collection", "transaction", "transaction-response", "batch", "batch-response", "history", "document", "message")
		eventNumber(out, body, "total", 1000000)
		if entries, ok := body["entry"].([]any); ok {
			projected := []any{}
			for _, entry := range entries {
				if len(projected) == 32 {
					break
				}
				if m, ok := entry.(map[string]any); ok {
					if resource, ok := m["resource"].(map[string]any); ok {
						if safe := eventObject(resource, reveal, depth+1); safe != nil {
							projected = append(projected, map[string]any{"resource": safe})
						}
					}
				}
			}
			out["entry"] = projected
		}
	case "List":
		eventString(out, body, "status", "current", "retired", "entered-in-error")
		eventString(out, body, "mode", "working", "snapshot", "changes")
		if subject, ok := body["subject"].(map[string]any); ok {
			if safe := eventIdentifierObject(subject["identifier"], reveal); safe != nil {
				out["subject"] = map[string]any{"identifier": safe}
			}
		}
		if code, ok := body["code"].(map[string]any); ok {
			if codings, ok := code["coding"].([]any); ok {
				for _, item := range codings {
					if c, ok := item.(map[string]any); ok && c["system"] == nvi.DataCategorySystem {
						if value, ok := c["code"].(string); ok && eventEnum(value, "Patient", "Condition", "MedicationRequest", "AllergyIntolerance") {
							out["code"] = map[string]any{"coding": []any{map[string]any{"system": nvi.DataCategorySystem, "code": value}}}
							break
						}
					}
				}
			}
		}
	case "Subscription":
		eventString(out, body, "status", "requested", "active", "error", "off")
		if channel, ok := body["channel"].(map[string]any); ok {
			safe := map[string]any{}
			eventString(safe, channel, "type", "rest-hook", "websocket", "email", "sms", "message")
			if len(safe) > 0 {
				out["channel"] = safe
			}
		}
	case "Patient", "Organization":
		if ids, ok := body["identifier"].([]any); ok {
			safe := []any{}
			for _, id := range ids {
				if len(safe) == 8 {
					break
				}
				if value := eventIdentifierObject(id, reveal); value != nil {
					safe = append(safe, value)
				}
			}
			if len(safe) > 0 {
				out["identifier"] = safe
			}
		}
	case "Endpoint":
		eventString(out, body, "status", "active", "suspended", "error", "off", "entered-in-error", "test")
	case "OperationOutcome":
		if issues, ok := body["issue"].([]any); ok {
			safe := []any{}
			for _, issue := range issues {
				if len(safe) == 8 {
					break
				}
				if m, ok := issue.(map[string]any); ok {
					projected := map[string]any{}
					eventString(projected, m, "severity", "fatal", "error", "warning", "information")
					eventString(projected, m, "code", "invalid", "structure", "required", "value", "invariant", "security", "login", "unknown", "expired", "forbidden", "suppressed", "processing", "not-supported", "duplicate", "multiple-matches", "not-found", "deleted", "too-long", "code-invalid", "extension", "too-costly", "business-rule", "conflict", "transient", "lock-error", "no-store", "exception", "timeout", "incomplete", "throttled", "informational")
					if len(projected) > 0 {
						safe = append(safe, projected)
					}
				}
			}
			if len(safe) > 0 {
				out["issue"] = safe
			}
		}
	}
	return out
}

func eventIdentifierObject(value any, reveal bool) map[string]any {
	object, ok := value.(map[string]any)
	if !ok {
		return nil
	}
	system, _ := object["system"].(string)
	identifier, _ := object["value"].(string)
	return eventIdentifier(system, identifier, reveal)
}

func eventString(out, body map[string]any, key string, allowed ...string) {
	if value, ok := body[key].(string); ok && eventEnum(value, allowed...) {
		out[key] = value
	}
}

func eventNumber(out, body map[string]any, key string, maximum int64) {
	if value, ok := body[key].(float64); ok && value >= 0 && value <= float64(maximum) && value == float64(int64(value)) {
		out[key] = int64(value)
	}
}
