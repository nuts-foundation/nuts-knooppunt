package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/nuts-foundation/nuts-knooppunt/test/testdata/vectors/pool"
)

const eventBodyLimit = 64 * 1024

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
	redact := eventRedactor{reveal: capture.revealSynthetic}
	method := req.Method
	if method == "" {
		method = http.MethodGet
	}
	requestBody, readable, req := bufferRequestBody(req)
	event := stepEvent{
		GF: t.gf, Actor: "sandbox-backend", Outcome: "error",
		Request:  eventRequest{Method: method, Path: redact.uri(req.URL), Body: redact.body(requestBody, req.Header.Get("Content-Type"))},
		Response: eventResponse{Headers: map[string]string{}},
	}
	if !readable {
		event.Request.Body = requestUnreadable
	}
	applyEventAction(req.Context(), &event)
	credential := strings.Fields(req.Header.Get("Authorization"))
	event.Request.TokenAttached = t.gf == "exchange" && len(credential) == 2 && strings.EqualFold(credential[0], "Bearer")
	publish := func() {
		event.DurationMs = time.Since(started).Milliseconds()
		event.TS = time.Now().UTC()
		capture.emit(event)
	}
	if capture.traces != nil {
		req = capture.traces.prepare(req, event, capture.emit, capture.live)
	}
	event.Request.Headers = redact.headers(req.Header)
	response, err := t.base.RoundTrip(req)
	if err != nil {
		publish()
		return response, err
	}
	event.Response.Status = response.StatusCode
	event.Response.Headers = redact.headers(response.Header)
	switch {
	case response.StatusCode == 401 || response.StatusCode == 403:
		event.Outcome = "deny"
	case response.StatusCode >= 200 && response.StatusCode < 300:
		event.Outcome = "ok"
	}
	tokenRequest := t.gf == "authorization" && method == http.MethodPost && response.StatusCode == http.StatusOK &&
		strings.HasSuffix(req.URL.Path, "/request-service-access-token")
	contentType := response.Header.Get("Content-Type")
	finish := func(data []byte, state bodyState) {
		switch state {
		case bodyFailed:
			event.Outcome = "error"
			event.Response.Body = bodyUnreadable
		case bodyOverflowed:
			event.Response.Body = bodyTooLarge
		case bodyIncomplete:
			event.Response.Body = bodyUnread
		default:
			event.Response.Body = redact.body(data, contentType)
			if tokenRequest {
				var token struct {
					AccessToken string `json:"access_token"`
				}
				event.Response.TokenReceived = json.Unmarshal(data, &token) == nil && token.AccessToken != ""
			}
		}
		publish()
	}
	if response.Body == nil || response.Body == http.NoBody {
		finish(nil, bodyComplete)
	} else {
		response.Body = &captureBody{body: response.Body, finish: finish, expected: response.ContentLength}
	}
	return response, nil
}

// readJSON decodes a response body read to its end. json.Decoder stops at the
// end of the value, which leaves a chunked body unfinished and its capture
// without the body.
func readJSON(body io.Reader, target any) error {
	data, err := io.ReadAll(body)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, target)
}

// bufferRequestBody returns up to eventBodyLimit+1 bytes of the request body
// and a request that still sends all of it. A body without GetBody, which is
// how go-fhir-client sends every NVI request, is read ahead once and replayed in
// front of the rest; a read error is replayed too, so the real request still
// fails on it.
func bufferRequestBody(req *http.Request) ([]byte, bool, *http.Request) {
	if req.Body == nil || req.Body == http.NoBody {
		return nil, true, req
	}
	if req.GetBody != nil {
		body, err := req.GetBody()
		if err != nil {
			return nil, false, req
		}
		defer body.Close()
		data, err := io.ReadAll(io.LimitReader(body, eventBodyLimit+1))
		return data, err == nil, req
	}
	data, err := io.ReadAll(io.LimitReader(req.Body, eventBodyLimit+1))
	rest := io.Reader(req.Body)
	if err != nil {
		rest = failingReader{err}
	}
	clone := req.Clone(req.Context())
	clone.Body = replayedBody{Reader: io.MultiReader(bytes.NewReader(data), rest), Closer: req.Body}
	return data, err == nil, clone
}

type replayedBody struct {
	io.Reader
	io.Closer
}

type failingReader struct{ err error }

func (r failingReader) Read([]byte) (int, error) { return 0, r.err }

type bodyState int

const (
	bodyComplete bodyState = iota
	bodyOverflowed
	bodyIncomplete
	bodyFailed
)

type captureBody struct {
	body     io.ReadCloser
	mu       sync.Mutex
	once     sync.Once
	data     []byte
	size     int64
	expected int64
	overflow bool
	finish   func([]byte, bodyState)
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
	switch {
	case err == io.EOF:
		b.publish(bodyComplete)
	case err != nil:
		b.publish(bodyFailed)
	}
	return n, err
}

func (b *captureBody) Close() error {
	err := b.body.Close()
	b.mu.Lock()
	defer b.mu.Unlock()
	switch {
	case err != nil:
		b.publish(bodyFailed)
	case b.expected > 0 && b.size == b.expected:
		b.publish(bodyComplete)
	default:
		b.publish(bodyIncomplete)
	}
	return err
}

func (b *captureBody) publish(state bodyState) {
	if state == bodyComplete && b.overflow {
		state = bodyOverflowed
	}
	b.once.Do(func() { b.finish(b.data, state); b.data = nil })
}

func eventEnum(value string, allowed ...string) bool {
	for _, candidate := range allowed {
		if value == candidate {
			return true
		}
	}
	return false
}

// Markers stand in for what an event does not show, so the viewer never
// presents a redacted or uncaptured value as an absent one.
const (
	redactedCredential  = "[redacted: credential]"
	redactedJWT         = "[redacted: JWT]"
	redactedJWE         = "[redacted: JWE]"
	redactedBlindFactor = "[redacted: blind_factor]"
	redactedBSN         = "[redacted: BSN]"
	bodyUnread          = "[not captured: the response body was not read to the end]"
	bodyUnreadable      = "[not captured: the response body could not be read]"
	requestUnreadable   = "[not captured: the request body could not be read]"
	bodyMalformedJSON   = "[not captured: malformed JSON body]"
)

var bodyTooLarge = fmt.Sprintf("[not captured: body exceeds the %d-byte capture limit]", eventBodyLimit)

// redactedKeys are JSON members, form fields and query parameters whose value
// is a credential or a pseudonymisation value the IG forbids retaining.
var redactedKeys = map[string]string{
	"access_token": redactedCredential, "refresh_token": redactedCredential, "id_token": redactedCredential,
	"client_assertion": redactedCredential, "assertion": redactedCredential, "client_secret": redactedCredential,
	"password": redactedCredential, "token": redactedCredential,
	"jwe": redactedJWE, "blind_factor": redactedBlindFactor, "blindFactor": redactedBlindFactor,
}

var (
	// A compact JWS has three segments, a compact JWE five; both start with a
	// base64url JSON header, which always encodes to "eyJ".
	compactTokenPattern = regexp.MustCompile(`\beyJ[A-Za-z0-9_-]*(?:\.[A-Za-z0-9_-]*){2,4}`)
	// Percent-escapes are matched on their own so their hex digits never join
	// the digits that follow them.
	digitRunPattern = regexp.MustCompile(`%[0-9A-Fa-f]{2}|[0-9]+`)
)

// eventRedactor keeps a captured call exact except for credentials, JWE and
// blind_factor values, and BSNs. BSNs are recognized by their check digit; in
// synthetic mode those of the demo pool stay visible.
type eventRedactor struct{ reveal bool }

func (r eventRedactor) uri(u *url.URL) string {
	if u == nil {
		return "/"
	}
	uri := u.EscapedPath()
	if uri == "" {
		uri = "/"
	}
	if u.RawQuery != "" {
		parameters := strings.Split(u.RawQuery, "&")
		for i, parameter := range parameters {
			name, _, _ := strings.Cut(parameter, "=")
			if key, err := url.QueryUnescape(name); err == nil {
				if marker, ok := redactedKeys[key]; ok {
					parameters[i] = name + "=" + marker
				}
			}
		}
		uri += "?" + strings.Join(parameters, "&")
	}
	return r.text(uri)
}

func (r eventRedactor) headers(h http.Header) map[string]string {
	out := make(map[string]string, len(h))
	for name, values := range h {
		value := strings.Join(values, ", ")
		switch http.CanonicalHeaderKey(name) {
		case "Authorization", "Proxy-Authorization":
			if scheme, _, found := strings.Cut(value, " "); found && scheme != "" {
				value = scheme + " " + redactedCredential
			} else {
				value = redactedCredential
			}
		case "Cookie", "Set-Cookie", "Dpop":
			value = redactedCredential
		default:
			value = r.text(value)
		}
		out[name] = value
	}
	return out
}

func (r eventRedactor) body(data []byte, contentType string) any {
	if len(data) == 0 {
		return nil
	}
	if len(data) > eventBodyLimit {
		return bodyTooLarge
	}
	mediaType, _, _ := mime.ParseMediaType(contentType)
	switch {
	case mediaType == "application/x-www-form-urlencoded":
		values, err := url.ParseQuery(string(data))
		if err != nil {
			return "[not captured: malformed form body]"
		}
		form := make(map[string][]string, len(values))
		for key, list := range values {
			for _, value := range list {
				form[key] = append(form[key], r.value(key, value))
			}
		}
		return form
	case json.Valid(data):
		decoder := json.NewDecoder(bytes.NewReader(data))
		decoder.UseNumber()
		var value any
		if err := decoder.Decode(&value); err != nil {
			return bodyMalformedJSON
		}
		return r.json(value)
	case mediaType == "application/json" || strings.HasSuffix(mediaType, "+json"):
		return bodyMalformedJSON
	case utf8.Valid(data):
		return r.text(string(data))
	default:
		if mediaType == "" {
			mediaType = "untyped"
		}
		return fmt.Sprintf("[not captured: %s body of %d bytes]", mediaType, len(data))
	}
}

func (r eventRedactor) json(value any) any {
	switch v := value.(type) {
	case map[string]any:
		for key, item := range v {
			if marker, ok := redactedKeys[key]; ok && item != nil && item != "" {
				v[key] = marker
				continue
			}
			v[key] = r.json(item)
		}
		return v
	case []any:
		for i, item := range v {
			v[i] = r.json(item)
		}
		return v
	case string:
		return r.text(v)
	default:
		return v
	}
}

func (r eventRedactor) value(key, value string) string {
	if marker, ok := redactedKeys[key]; ok && value != "" {
		return marker
	}
	return r.text(value)
}

func (r eventRedactor) text(s string) string {
	s = compactTokenPattern.ReplaceAllStringFunc(s, func(token string) string {
		if strings.Count(token, ".") == 4 {
			return redactedJWE
		}
		return redactedJWT
	})
	return digitRunPattern.ReplaceAllStringFunc(s, func(run string) string {
		if len(run) != 9 || !validBSN(run) || (r.reveal && syntheticBSN(run)) {
			return run
		}
		return redactedBSN
	})
}

// validBSN applies the BSN check digit (elfproef) to nine ASCII digits.
func validBSN(digits string) bool {
	sum := 0
	for i := range 8 {
		sum += int(digits[i]-'0') * (9 - i)
	}
	sum -= int(digits[8] - '0')
	return sum%11 == 0
}

func syntheticBSN(value string) bool {
	for _, patient := range pool.Patients() {
		if value == patient.BSN {
			return true
		}
	}
	return false
}
