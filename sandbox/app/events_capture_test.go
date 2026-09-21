package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/nuts-foundation/nuts-knooppunt/test/testdata/vectors/pool"
)

func TestCapturePreservesBodiesAndEmitsOnce(t *testing.T) {
	requestBody := `{"resourceType":"Subscription","status":"requested","reason":"secret"}`
	responseBody := `{"resourceType":"Bundle","type":"searchset","total":0,"entry":[]}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got, _ := io.ReadAll(r.Body)
		if string(got) != requestBody {
			t.Errorf("server body = %s", got)
		}
		if r.Header.Get("Authorization") != "Bearer secret" {
			t.Error("authorization was changed")
		}
		w.Header().Set("Content-Type", "application/fhir+json")
		w.Header().Set("Set-Cookie", "secret")
		_, _ = io.WriteString(w, responseBody)
	}))
	defer server.Close()
	var events []stepEvent
	ctx := withEventCapture(context.Background(), func(e stepEvent) { events = append(events, e) }, false)
	req, _ := http.NewRequestWithContext(ctx, "POST", server.URL+"/mitz/Subscription", strings.NewReader(requestBody))
	req.Header.Set("Content-Type", "application/fhir+json")
	req.Header.Set("Authorization", "Bearer secret")
	response, err := (&http.Client{Transport: newCaptureTransport("consent", nil)}).Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 0 {
		t.Fatal("event emitted before body consumption")
	}
	body, err := io.ReadAll(response.Body)
	if err != nil || string(body) != responseBody {
		t.Fatalf("response = %q, %v", body, err)
	}
	_ = response.Body.Close()
	_ = response.Body.Close()
	if len(events) != 1 {
		t.Fatalf("event count = %d", len(events))
	}
	e := events[0]
	if e.GF != "consent" || e.Actor != "sandbox-backend" || e.Outcome != "ok" || e.Response.Status != 200 || e.TS.IsZero() {
		t.Fatalf("event = %#v", e)
	}
	if e.Request.Body == nil || e.Response.Body == nil {
		t.Fatal("known bodies were omitted")
	}
	data, _ := json.Marshal(e)
	if strings.Contains(string(data), "secret") {
		t.Fatalf("secret in %s", data)
	}
}

func TestCaptureTokenReceiptEvidenceDoesNotRetainToken(t *testing.T) {
	for _, tc := range []struct {
		name, gf, method, path, body string
		status                       int
		partial, want                bool
	}{
		{"received", "authorization", "POST", "/nuts/internal/auth/v2/plataan/request-service-access-token", `{"access_token":"token-value-never-retain","token_type":"Bearer"}`, 200, false, true},
		{"empty", "authorization", "POST", "/nuts/internal/auth/v2/plataan/request-service-access-token", `{"access_token":""}`, 200, false, false},
		{"missing", "authorization", "POST", "/nuts/internal/auth/v2/plataan/request-service-access-token", `{"token_type":"Bearer"}`, 200, false, false},
		{"spoofed flag", "authorization", "POST", "/nuts/internal/auth/v2/plataan/request-service-access-token", `{"tokenReceived":true}`, 200, false, false},
		{"oversized", "authorization", "POST", "/nuts/internal/auth/v2/plataan/request-service-access-token", `{"access_token":"token-value-never-retain","padding":"` + strings.Repeat("x", eventBodyLimit) + `"}`, 200, false, false},
		{"wrong type", "authorization", "POST", "/nuts/internal/auth/v2/plataan/request-service-access-token", `{"access_token":123}`, 200, false, false},
		{"malformed", "authorization", "POST", "/nuts/internal/auth/v2/plataan/request-service-access-token", `{"access_token":"token-value-never-retain"`, 200, false, false},
		{"refused", "authorization", "POST", "/nuts/internal/auth/v2/plataan/request-service-access-token", `{"access_token":"token-value-never-retain"}`, 403, false, false},
		{"unread", "authorization", "POST", "/nuts/internal/auth/v2/plataan/request-service-access-token", `{"access_token":"token-value-never-retain"}`, 200, true, false},
		{"other endpoint", "authorization", "POST", "/nuts/internal/auth/v2/accesstoken/introspect", `{"access_token":"token-value-never-retain"}`, 200, false, false},
		{"other method", "authorization", "GET", "/nuts/internal/auth/v2/plataan/request-service-access-token", `{"access_token":"token-value-never-retain"}`, 200, false, false},
		{"other service", "exchange", "POST", "/nuts/internal/auth/v2/plataan/request-service-access-token", `{"access_token":"token-value-never-retain"}`, 200, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.status)
				_, _ = io.WriteString(w, tc.body)
			}))
			defer server.Close()
			var events []stepEvent
			ctx := withEventCapture(context.Background(), func(e stepEvent) { events = append(events, e) }, false)
			req, _ := http.NewRequestWithContext(ctx, tc.method, server.URL+tc.path, nil)
			res, err := (&http.Client{Transport: newCaptureTransport(tc.gf, nil)}).Do(req)
			if err != nil {
				t.Fatal(err)
			}
			if !tc.partial {
				body, err := io.ReadAll(res.Body)
				if err != nil || string(body) != tc.body {
					t.Fatalf("response changed: %q, %v", body, err)
				}
			}
			_ = res.Body.Close()
			if len(events) != 1 {
				t.Fatalf("events: %d", len(events))
			}
			encoded, _ := json.Marshal(events[0])
			var decoded struct {
				Response struct {
					TokenReceived bool `json:"tokenReceived"`
				} `json:"response"`
			}
			if err := json.Unmarshal(encoded, &decoded); err != nil {
				t.Fatal(err)
			}
			if decoded.Response.TokenReceived != tc.want {
				t.Fatalf("token receipt = %v, want %v", decoded.Response.TokenReceived, tc.want)
			}
			if strings.Contains(string(encoded), "token-value-never-retain") {
				t.Fatal("token leaked into event")
			}
		})
	}
}

func TestCaptureTokenReceiptFromChunkedClientResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		_, _ = io.WriteString(w, `{"access_token":"chunked-token-never-retain"}`)
		w.(http.Flusher).Flush()
	}))
	defer server.Close()
	var events []stepEvent
	ctx := withEventCapture(context.Background(), func(e stepEvent) { events = append(events, e) }, false)
	client := newNutsClient(nutsConfig{InternalBaseURL: server.URL, Subject: "example", Scope: "bgz"})
	token, err := client.requestToken(ctx, authSession{}, server.URL)
	if err != nil || token != "chunked-token-never-retain" {
		t.Fatalf("token response was not received: %v", err)
	}
	if len(events) != 1 || !events[0].Response.TokenReceived {
		t.Fatal("fully received chunked token response must provide key evidence")
	}
	encoded, _ := json.Marshal(events[0])
	if strings.Contains(string(encoded), token) {
		t.Fatal("token leaked into event")
	}
}

func TestCaptureAttachedKeyEvidenceDoesNotRetainCredential(t *testing.T) {
	for _, tc := range []struct {
		name, gf, header string
		want             bool
	}{
		{"bearer", "exchange", "Bearer private-access-key", true},
		{"case insensitive", "exchange", "bearer private-access-key", true},
		{"missing", "exchange", "", false},
		{"empty", "exchange", "Bearer ", false},
		{"basic", "exchange", "Basic private-access-key", false},
		{"other service", "authorization", "Bearer private-access-key", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var event stepEvent
			ctx := withEventCapture(context.Background(), func(e stepEvent) { event = e }, false)
			base := captureRoundTripFunc(func(r *http.Request) (*http.Response, error) {
				if r.Header.Get("Authorization") != tc.header {
					t.Fatal("capture changed the real credential")
				}
				return &http.Response{StatusCode: 200, Header: http.Header{}, Body: http.NoBody}, nil
			})
			req, _ := http.NewRequestWithContext(ctx, "POST", "http://source.example/fhir/Patient/_search", nil)
			req.Header.Set("Authorization", tc.header)
			res, err := newCaptureTransport(tc.gf, base).RoundTrip(req)
			if err != nil {
				t.Fatal(err)
			}
			_ = res.Body.Close()
			encoded, _ := json.Marshal(event)
			var decoded struct {
				Request struct {
					TokenAttached bool `json:"tokenAttached"`
				} `json:"request"`
			}
			if err := json.Unmarshal(encoded, &decoded); err != nil {
				t.Fatal(err)
			}
			if decoded.Request.TokenAttached != tc.want {
				t.Fatalf("attached key evidence = %v, want %v", decoded.Request.TokenAttached, tc.want)
			}
			if strings.Contains(string(encoded), "private-access-key") {
				t.Fatal("credential leaked into event")
			}
		})
	}
}

func TestCaptureNoContextAndOutcomes(t *testing.T) {
	for _, status := range []int{200, 201, 302, 401, 403, 500} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(status) }))
			defer server.Close()
			var events []stepEvent
			client := &http.Client{Transport: newCaptureTransport("exchange", nil)}
			req, _ := http.NewRequest("GET", server.URL, nil)
			res, err := client.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			_ = res.Body.Close()
			if len(events) != 0 {
				t.Fatal("event without capture context")
			}
			req = req.WithContext(withEventCapture(req.Context(), func(e stepEvent) { events = append(events, e) }, false))
			res, err = client.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			_ = res.Body.Close()
			expected := "error"
			if status >= 200 && status < 300 {
				expected = "ok"
			}
			if status == 401 || status == 403 {
				expected = "deny"
			}
			if len(events) != 1 || events[0].Outcome != expected {
				t.Fatalf("events = %#v", events)
			}
		})
	}
}

type captureRoundTripFunc func(*http.Request) (*http.Response, error)

func (f captureRoundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type captureBrokenBody struct{ reads int }

func (b *captureBrokenBody) Read(p []byte) (int, error) {
	b.reads++
	return copy(p, `{"resourceType":"Patient"}`), errors.New("secret-read-error")
}
func (*captureBrokenBody) Close() error { return nil }

func TestCaptureTransportAndReadErrors(t *testing.T) {
	for _, network := range []bool{false, true} {
		var events []stepEvent
		broken := &captureBrokenBody{}
		base := captureRoundTripFunc(func(*http.Request) (*http.Response, error) {
			if network {
				return nil, errors.New("secret-network-error")
			}
			return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/fhir+json"}}, Body: broken}, nil
		})
		req, _ := http.NewRequestWithContext(withEventCapture(context.Background(), func(e stepEvent) { events = append(events, e) }, false), "GET", "http://example.invalid/Patient", nil)
		res, err := newCaptureTransport("exchange", base).RoundTrip(req)
		if network {
			if err == nil {
				t.Fatal("lost network error")
			}
		} else {
			if broken.reads != 0 {
				t.Fatal("eager response body read")
			}
			_, err = io.ReadAll(res.Body)
			if err == nil {
				t.Fatal("lost read error")
			}
			_ = res.Body.Close()
		}
		if len(events) != 1 || events[0].Outcome != "error" || events[0].Response.Body != nil {
			t.Fatalf("events = %#v", events)
		}
		data, _ := json.Marshal(events)
		if strings.Contains(string(data), "secret") {
			t.Fatalf("leaked error: %s", data)
		}
	}
}

func TestCaptureFailsClosedForSecrets(t *testing.T) {
	bsn := pool.Patients()[0].BSN
	for _, reveal := range []bool{false, true} {
		for _, body := range []string{
			`{"resourceType":"Patient","identifier":[{"system":"http://fhir.nl/fhir/NamingSystem/bsn","value":"` + bsn + `"},{"system":"http://fhir.nl/fhir/NamingSystem/bsn","value":"123456789"}],"name":[{"text":"secret"}],"text":{"div":"secret"}}`,
			`{"resourceType":"List","status":"secret","mode":"secret","subject":{"identifier":{"system":"secret","value":"secret"}},"code":{"coding":[{"system":"secret","code":"secret"}]}}`,
			`{"resourceType":"Bundle","type":"secret","total":"secret","entry":[{"fullUrl":"secret","resource":{"resourceType":"secret","status":"secret"}}]}`,
			`{"access_token":"secret","id_token":"secret","scope":"secret","token_type":"secret","active":"secret","expires_in":"secret","credentials":["secret"]}`,
			`{"resourceType":"OperationOutcome","issue":[{"code":"secret","severity":"secret","diagnostics":"secret","details":{"text":"secret"}}]}`,
			`{"resourceType":"secret","identifier":"secret"}`,
			`secret`,
			`{"resourceType":"Patient","text":"` + strings.Repeat("secret", 12000) + `"}`,
		} {
			var events []stepEvent
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = io.Copy(io.Discard, r.Body)
				w.Header().Set("Content-Type", "application/fhir+json; secret=secret")
				w.Header().Set("X-Secret", "secret")
				w.Header().Set("Location", "https://secret.invalid")
				_, _ = io.WriteString(w, body)
			}))
			ctx := withEventCapture(context.Background(), func(e stepEvent) { events = append(events, e) }, reveal)
			req, _ := http.NewRequestWithContext(ctx, "POST", server.URL+"/secret/Patient/secret?token=secret&_count=secret&_include=secret&identifier=secret&patientid="+bsn, strings.NewReader(body))
			req.Header.Set("Authorization", "Bearer secret")
			req.Header.Set("Cookie", "secret")
			req.Header.Set("Content-Type", "application/fhir+json; secret=secret")
			res, err := (&http.Client{Transport: newCaptureTransport("exchange", nil)}).Do(req)
			if err != nil {
				t.Fatal(err)
			}
			_, _ = io.Copy(io.Discard, res.Body)
			_ = res.Body.Close()
			server.Close()
			if len(events) != 1 {
				t.Fatalf("count = %d", len(events))
			}
			data, _ := json.Marshal(events[0])
			encoded := string(data)
			if strings.Contains(encoded, "secret") || strings.Contains(encoded, "123456789") {
				t.Fatalf("leak: %s", encoded)
			}
			if strings.Contains(encoded, bsn) != reveal {
				t.Fatalf("synthetic visibility %v: %s", reveal, encoded)
			}
			if (body == "secret" || len(body) > 65536 || strings.Contains(body, `"resourceType":"secret","identifier"`)) && (events[0].Request.Body != nil || events[0].Response.Body != nil) {
				t.Fatalf("unknown body captured: %s", encoded)
			}
		}
	}
}

func TestCapturePartialBodyCloseOmitsBody(t *testing.T) {
	var event stepEvent
	base := captureRoundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/fhir+json"}}, Body: io.NopCloser(strings.NewReader(`{"resourceType":"Patient"}secret`))}, nil
	})
	req, _ := http.NewRequestWithContext(withEventCapture(context.Background(), func(e stepEvent) { event = e }, false), "GET", "http://example.invalid/Patient", nil)
	res, err := newCaptureTransport("exchange", base).RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, len(`{"resourceType":"Patient"}`))
	_, _ = io.ReadFull(res.Body, buf)
	_ = res.Body.Close()
	if event.Response.Status != 200 || event.Response.Body != nil {
		t.Fatalf("partial capture = %#v", event)
	}
}

func TestCaptureFormAndUnreplayableRequest(t *testing.T) {
	bsn := pool.Patients()[0].BSN
	form := "subject%3Aidentifier=http%3A%2F%2Ffhir.nl%2Ffhir%2FNamingSystem%2Fbsn%7C" + bsn + "&_count=1000&token=secret"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, _ := io.ReadAll(r.Body)
		if string(data) != form {
			t.Errorf("changed form: %s", data)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	for _, replayable := range []bool{true, false} {
		var events []stepEvent
		ctx := withEventCapture(context.Background(), func(e stepEvent) { events = append(events, e) }, false)
		var body io.Reader = strings.NewReader(form)
		if !replayable {
			body = io.NopCloser(body)
		}
		req, _ := http.NewRequestWithContext(ctx, "POST", server.URL+"/nvi/List/_search", body)
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		res, err := (&http.Client{Transport: newCaptureTransport("localization", nil)}).Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_ = res.Body.Close()
		if len(events) != 1 {
			t.Fatalf("count = %d", len(events))
		}
		if (events[0].Request.Body != nil) != replayable {
			t.Fatalf("replayable=%v body=%v", replayable, events[0].Request.Body)
		}
		data, _ := json.Marshal(events[0])
		if strings.Contains(string(data), bsn) || strings.Contains(string(data), "secret") {
			t.Fatalf("leak: %s", data)
		}
	}
}

func TestCaptureRealClientWiring(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		w.Header().Set("Content-Type", "application/fhir+json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/request-service-access-token"):
			_, _ = io.WriteString(w, `{"access_token":"secret"}`)
		case strings.HasSuffix(r.URL.Path, "/introspect"):
			_, _ = io.WriteString(w, `{"active":true,"subject":"secret"}`)
		case r.URL.Path == "/mitz/Subscription":
			w.WriteHeader(http.StatusCreated)
		case r.URL.Path == "/nvi/List":
			w.WriteHeader(http.StatusCreated)
			_, _ = io.WriteString(w, `{"resourceType":"List","status":"current","mode":"working"}`)
		default:
			_, _ = io.WriteString(w, `{"resourceType":"Bundle","type":"searchset","total":0,"entry":[]}`)
		}
	}))
	defer server.Close()
	base, _ := url.Parse(server.URL)
	var events []stepEvent
	ctx := withEventCapture(context.Background(), func(e stepEvent) { events = append(events, e) }, false)
	bsn := pool.Patients()[0].BSN
	lookup, register := nviFuncs(base, "example")
	if _, err := lookup(ctx, bsn); err != nil {
		t.Fatal(err)
	}
	if err := register(ctx, bsn, []string{"Patient"}); err != nil {
		t.Fatal(err)
	}
	if _, err := nviLocalizeFunc(base)(ctx, bsn); err != nil {
		t.Fatal(err)
	}
	if err := mitzSubscribeFunc(base, "00000010", "Z3")(ctx, bsn); err != nil {
		t.Fatal(err)
	}
	if _, err := mitzSubscribedFunc(base, "00000010")(ctx, bsn); err != nil {
		t.Fatal(err)
	}
	if _, err := mcsdResolveFunc(base)(ctx, "00000010"); err == nil {
		t.Fatal("expected no matching organization")
	}
	nuts := newNutsClient(nutsConfig{InternalBaseURL: server.URL, Subject: "example", Scope: "bgz"})
	if token, err := nuts.requestToken(ctx, authSession{Attestation: "secret"}, server.URL); err != nil || token != "secret" {
		t.Fatalf("token = %q, %v", token, err)
	}
	if _, err := nuts.introspect(ctx, "secret"); err != nil {
		t.Fatal(err)
	}
	if _, err := retrieveBGZ(ctx, sourceAddress{Address: server.URL}, "secret", bsn, []string{"Patient"}); err != nil {
		t.Fatal(err)
	}
	counts := map[string]int{}
	for _, event := range events {
		counts[event.GF]++
	}
	for gf, want := range map[string]int{"localization": 4, "consent": 2, "addressing": 1, "authorization": 2, "exchange": 1} {
		if counts[gf] != want {
			t.Errorf("%s events=%d want=%d", gf, counts[gf], want)
		}
	}
	data, _ := json.Marshal(events)
	if strings.Contains(string(data), bsn) || strings.Contains(string(data), "secret") {
		t.Fatalf("client wiring leaked: %s", data)
	}
}

func TestCaptureRejectsMalformedResourceDiscriminator(t *testing.T) {
	for _, body := range []string{`{"resourceType":false,"active":true}`, `{"resourceType":null,"active":true}`, `{"resourceType":"","active":true}`} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, body)
		}))
		var event stepEvent
		ctx := withEventCapture(context.Background(), func(e stepEvent) { event = e }, false)
		req, _ := http.NewRequestWithContext(ctx, "GET", server.URL, nil)
		res, err := (&http.Client{Transport: newCaptureTransport("exchange", nil)}).Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = io.Copy(io.Discard, res.Body)
		_ = res.Body.Close()
		server.Close()
		if event.Response.Body != nil {
			t.Errorf("malformed resource captured as %v", event.Response.Body)
		}
	}
}
