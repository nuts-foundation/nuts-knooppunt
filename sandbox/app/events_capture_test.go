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
	"time"

	"github.com/nuts-foundation/nuts-knooppunt/test/testdata/vectors/nvi"
	"github.com/nuts-foundation/nuts-knooppunt/test/testdata/vectors/pool"
	"github.com/stretchr/testify/require"
)

func TestCapturePreservesBodiesAndEmitsOnce(t *testing.T) {
	requestBody := `{"resourceType":"Subscription","status":"requested","reason":"OTV"}`
	responseBody := `{"resourceType":"Bundle","type":"searchset","total":0,"entry":[]}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got, _ := io.ReadAll(r.Body)
		if string(got) != requestBody {
			t.Errorf("server body = %s", got)
		}
		if r.Header.Get("Authorization") != "Bearer private-credential" {
			t.Error("authorization was changed")
		}
		w.Header().Set("Content-Type", "application/fhir+json")
		w.Header().Set("Set-Cookie", "session=private-cookie")
		_, _ = io.WriteString(w, responseBody)
	}))
	defer server.Close()
	var events []stepEvent
	ctx := withEventCapture(context.Background(), func(e stepEvent) { events = append(events, e) }, false)
	req, _ := http.NewRequestWithContext(ctx, "POST", server.URL+"/mitz/Subscription", strings.NewReader(requestBody))
	req.Header.Set("Content-Type", "application/fhir+json")
	req.Header.Set("Authorization", "Bearer private-credential")
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
	require.JSONEq(t, requestBody, marshalled(t, e.Request.Body))
	require.JSONEq(t, responseBody, marshalled(t, e.Response.Body))
	require.Equal(t, "Bearer [redacted: credential]", e.Request.Headers["Authorization"])
	require.Equal(t, "[redacted: credential]", e.Response.Headers["Set-Cookie"])
	require.NotContains(t, marshalled(t, e), "private-")
}

func marshalled(t *testing.T, value any) string {
	t.Helper()
	data, err := json.Marshal(value)
	require.NoError(t, err)
	return string(data)
}

// The viewer shows the call as it went over the wire: every query parameter,
// including repeated ones, every header and the complete body.
func TestCaptureKeepsTheExactCall(t *testing.T) {
	const response = `{"resourceType":"Bundle","type":"searchset","total":1,` +
		`"link":[{"relation":"next","url":"https://source.example/fhir?_getpages=abc&_getpagesoffset=20"}],` +
		`"entry":[{"fullUrl":"https://source.example/fhir/MedicationRequest/m1","resource":{"resourceType":"MedicationRequest","id":"m1","status":"active","intent":"order",` +
		`"subject":{"reference":"Patient/p-1"},"medicationReference":{"reference":"Medication/1"},"dosageInstruction":[{"text":"50 mg 1dd"}]}},` +
		`{"resource":{"resourceType":"Endpoint","id":"e1","status":"active","address":"https://source.example/fhir",` +
		`"connectionType":{"system":"http://terminology.hl7.org/CodeSystem/endpoint-connection-type","code":"hl7-fhir-rest"},` +
		`"payloadType":[{"coding":[{"code":"MedicationRequest"}]}]}}]}`
	const target = "/fhir/MedicationRequest?_include=MedicationRequest%3Amedication&category=http%3A%2F%2Fsnomed.info%2Fsct%7C16076005" +
		"&patient=Patient%2Fp-1&_include=MedicationRequest%3Arequester"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.RequestURI() != target {
			t.Errorf("request = %s", r.URL.RequestURI())
		}
		w.Header().Set("Content-Type", "application/fhir+json;charset=UTF-8")
		w.Header().Set("ETag", `W/"1"`)
		_, _ = io.WriteString(w, response)
	}))
	defer server.Close()
	var event stepEvent
	ctx := withEventCapture(context.Background(), func(e stepEvent) { event = e }, false)
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+target, nil)
	req.Header.Set("Accept", "application/fhir+json")
	req.Header["X-Tenant-ID"] = []string{"http://fhir.nl/fhir/NamingSystem/ura|00000010"}
	res, err := (&http.Client{Transport: newCaptureTransport("exchange", nil)}).Do(req)
	require.NoError(t, err)
	_, _ = io.ReadAll(res.Body)
	require.NoError(t, res.Body.Close())

	require.Equal(t, http.MethodGet, event.Request.Method)
	require.Equal(t, target, event.Request.Path)
	require.Equal(t, "application/fhir+json", event.Request.Headers["Accept"])
	require.Equal(t, "http://fhir.nl/fhir/NamingSystem/ura|00000010", event.Request.Headers["X-Tenant-ID"])
	require.Nil(t, event.Request.Body, "a GET carries no body")
	require.Equal(t, "application/fhir+json;charset=UTF-8", event.Response.Headers["Content-Type"])
	require.Equal(t, `W/"1"`, event.Response.Headers["Etag"])
	require.JSONEq(t, response, marshalled(t, event.Response.Body))
}

// go-fhir-client rebuilds its requests without GetBody, so the NVI searches and
// registrations are exactly the calls whose bodies a GetBody-only capture loses.
func TestCaptureRecordsRequestBodiesTheClientCannotReplay(t *testing.T) {
	bsn := pool.Patients()[0].BSN
	var received []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, _ := io.ReadAll(r.Body)
		received = append(received, string(data))
		w.Header().Set("Content-Type", "application/fhir+json")
		if r.URL.Path == "/nvi/List/_search" {
			_, _ = io.WriteString(w, `{"resourceType":"Bundle","type":"searchset","entry":[]}`)
			return
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write(data)
	}))
	defer server.Close()
	base, _ := url.Parse(server.URL + "/nvi")
	var events []stepEvent
	ctx := withEventCapture(context.Background(), func(e stepEvent) { events = append(events, e) }, false)
	client := nvi.NewClient(base, &http.Client{Transport: newCaptureTransport("localization", nil)})
	_, err := client.ListsForPatient(ctx, "00000010", bsn)
	require.NoError(t, err)
	require.NoError(t, client.Register(ctx, nvi.Registration{CustodianURA: "00000010", BSN: bsn, ClientID: "example", Categories: []string{nvi.CategoryCondition}}))
	require.Len(t, events, 3, "patient search, cleanup search, registration")

	require.Contains(t, received[0], "subject%3Aidentifier=http%3A%2F%2Ffhir.nl%2Ffhir%2FNamingSystem%2Fbsn%7C"+bsn,
		"the real request still carries the identifier")
	require.JSONEq(t, `{"_count":["1000"],"subject:identifier":["http://fhir.nl/fhir/NamingSystem/bsn|[redacted: BSN]"]}`,
		marshalled(t, events[0].Request.Body))

	registration, ok := events[2].Request.Body.(map[string]any)
	require.True(t, ok, "registration body: %v", events[2].Request.Body)
	require.Equal(t, "List", registration["resourceType"])
	require.Contains(t, marshalled(t, registration["code"]), `"code":"Condition"`)
	require.Contains(t, marshalled(t, registration["subject"]), "[redacted: BSN]")
	require.Contains(t, received[2], bsn, "the real registration still carries the identifier")
	require.NotContains(t, marshalled(t, events), bsn)
}

// Replayable and one-shot request bodies reach the server byte for byte.
func TestCaptureLeavesTheRealRequestBodyIntact(t *testing.T) {
	form := "subject%3Aidentifier=http%3A%2F%2Ffhir.nl%2Ffhir%2FNamingSystem%2Fbsn%7C999900006&_count=1000"
	large := strings.Repeat("x", eventBodyLimit+10)
	for _, tc := range []struct {
		name, body string
		oneShot    bool
	}{
		{"replayable", form, false},
		{"one-shot", form, true},
		{"one-shot beyond the capture limit", large, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				data, _ := io.ReadAll(r.Body)
				if string(data) != tc.body {
					t.Errorf("server received %d bytes, want %d", len(data), len(tc.body))
				}
				w.WriteHeader(http.StatusNoContent)
			}))
			defer server.Close()
			var event stepEvent
			ctx := withEventCapture(context.Background(), func(e stepEvent) { event = e }, true)
			var body io.Reader = strings.NewReader(tc.body)
			if tc.oneShot {
				body = io.NopCloser(body)
			}
			req, _ := http.NewRequestWithContext(ctx, http.MethodPost, server.URL+"/nvi/List/_search", body)
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			res, err := (&http.Client{Transport: newCaptureTransport("localization", nil)}).Do(req)
			require.NoError(t, err)
			require.NoError(t, res.Body.Close())
			if tc.body == large {
				require.Equal(t, "[not captured: body exceeds the 65536-byte capture limit]", event.Request.Body)
				return
			}
			require.JSONEq(t, `{"_count":["1000"],"subject:identifier":["http://fhir.nl/fhir/NamingSystem/bsn|999900006"]}`,
				marshalled(t, event.Request.Body))
		})
	}
}

// captureFailingReader fails once and then reports a clean end, so a replay
// that dropped the error would send an empty body instead of failing.
type captureFailingReader struct{ failed bool }

func (r *captureFailingReader) Read([]byte) (int, error) {
	if r.failed {
		return 0, io.EOF
	}
	r.failed = true
	return 0, errors.New("request body broke")
}

// A body the capture could not read is marked, and the real request still
// fails on it rather than going out with whatever part was read.
func TestCaptureMarksAnUnreadableRequestBody(t *testing.T) {
	var event stepEvent
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer server.Close()
	ctx := withEventCapture(context.Background(), func(e stepEvent) { event = e }, false)
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, server.URL+"/nvi/List", io.NopCloser(&captureFailingReader{}))
	req.Header.Set("Content-Type", "application/fhir+json")
	_, err := (&http.Client{Transport: newCaptureTransport("localization", nil)}).Do(req)
	require.ErrorContains(t, err, "request body broke")
	require.Equal(t, "error", event.Outcome)
	require.Equal(t, "[not captured: the request body could not be read]", event.Request.Body)
}

// Redaction replaces a value with a visible marker and keeps its key, so the
// viewer shows where a credential travelled without showing the credential.
func TestCaptureRedactsCredentialsVisibly(t *testing.T) {
	const jws = "eyJhbGciOiJFUzI1NiJ9.eyJzdWIiOiJwcml2YXRlLWp3dCJ9.c2ln"
	const jwe = "eyJlbmMiOiJBMjU2R0NNIn0.a2V5.aXY.cHJpdmF0ZS1qd2U.dGFn"
	requestBody := `{"id_token":"private-1","client_assertion":"private-2","scope":"bgz","token_type":"Bearer",` +
		`"nested":{"assertion":"private-3"},"list":[{"client_secret":"private-4","password":"private-5"}],` +
		`"authorization_server":"https://source.example/oauth2","note":"` + jws + `"}`
	responseBody := `{"access_token":"private-6","refresh_token":"private-7","token_type":"Bearer","expires_in":900,` +
		`"jwe":"private-8","blind_factor":"private-9","pseudonym":"` + jwe + `","empty_token":"","access_token_hint":null}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Set-Cookie", "session=private-10")
		_, _ = io.WriteString(w, responseBody)
	}))
	defer server.Close()
	var events []stepEvent
	ctx := withEventCapture(context.Background(), func(e stepEvent) { events = append(events, e) }, false)
	client := &http.Client{Transport: newCaptureTransport("authorization", nil)}

	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, server.URL+"/token?code=visible&access_token=private-11", strings.NewReader(requestBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "DPoP private-12")
	req.Header.Set("DPoP", jws)
	req.Header.Set("Cookie", "a=private-13")
	req.Header.Set("X-Request-Id", "visible-request-id")
	res, err := client.Do(req)
	require.NoError(t, err)
	_, _ = io.ReadAll(res.Body)
	require.NoError(t, res.Body.Close())

	form, _ := http.NewRequestWithContext(ctx, http.MethodPost, server.URL+"/introspect",
		strings.NewReader("token=private-14&token_type_hint=access_token"))
	form.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res, err = client.Do(form)
	require.NoError(t, err)
	_, _ = io.ReadAll(res.Body)
	require.NoError(t, res.Body.Close())

	require.Len(t, events, 2)
	call := events[0]
	require.Equal(t, "/token?code=visible&access_token=[redacted: credential]", call.Request.Path)
	require.Equal(t, "DPoP [redacted: credential]", call.Request.Headers["Authorization"])
	require.Equal(t, "[redacted: credential]", call.Request.Headers["Dpop"])
	require.Equal(t, "[redacted: credential]", call.Request.Headers["Cookie"])
	require.Equal(t, "visible-request-id", call.Request.Headers["X-Request-Id"])
	require.Equal(t, "[redacted: credential]", call.Response.Headers["Set-Cookie"])
	require.JSONEq(t, `{"id_token":"[redacted: credential]","client_assertion":"[redacted: credential]","scope":"bgz","token_type":"Bearer",`+
		`"nested":{"assertion":"[redacted: credential]"},"list":[{"client_secret":"[redacted: credential]","password":"[redacted: credential]"}],`+
		`"authorization_server":"https://source.example/oauth2","note":"[redacted: JWT]"}`, marshalled(t, call.Request.Body))
	require.JSONEq(t, `{"access_token":"[redacted: credential]","refresh_token":"[redacted: credential]","token_type":"Bearer","expires_in":900,`+
		`"jwe":"[redacted: JWE]","blind_factor":"[redacted: blind_factor]","pseudonym":"[redacted: JWE]","empty_token":"","access_token_hint":null}`,
		marshalled(t, call.Response.Body))
	require.JSONEq(t, `{"token":["[redacted: credential]"],"token_type_hint":["access_token"]}`, marshalled(t, events[1].Request.Body))
	require.NotContains(t, marshalled(t, events), "private-")
}

// Only BSNs are masked: a 9-digit value that fails the BSN check digit is shown,
// and synthetic mode reveals the demo pool while still masking any other BSN.
func TestCaptureMasksBSNsUnlessTheSyntheticPoolIsRevealed(t *testing.T) {
	const poolBSN, otherBSN, notABSN = "999900006", "111222333", "123456789"
	responseBody := `{"resourceType":"Patient","identifier":[{"system":"http://fhir.nl/fhir/NamingSystem/bsn","value":"` + poolBSN + `"},` +
		`{"system":"http://fhir.nl/fhir/NamingSystem/bsn","value":"` + otherBSN + `"}],"telecom":[{"value":"` + notABSN + `"}]}`
	for _, tc := range []struct {
		reveal                     bool
		path, header, pool, others string
	}{
		{false, "/abonnementen/fhir/Subscription?patientid=[redacted: BSN]&providerid=00000010", "[redacted: BSN]", "[redacted: BSN]", "[redacted: BSN]"},
		{true, "/abonnementen/fhir/Subscription?patientid=" + poolBSN + "&providerid=00000010", "[redacted: BSN]", poolBSN, "[redacted: BSN]"},
	} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/fhir+json")
			_, _ = io.WriteString(w, responseBody)
		}))
		var event stepEvent
		ctx := withEventCapture(context.Background(), func(e stepEvent) { event = e }, tc.reveal)
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+"/abonnementen/fhir/Subscription?patientid="+poolBSN+"&providerid=00000010", nil)
		req.Header.Set("X-Subject", otherBSN)
		res, err := (&http.Client{Transport: newCaptureTransport("consent", nil)}).Do(req)
		require.NoError(t, err)
		_, _ = io.ReadAll(res.Body)
		require.NoError(t, res.Body.Close())
		server.Close()

		require.Equal(t, tc.path, event.Request.Path, "reveal=%v", tc.reveal)
		require.Equal(t, tc.header, event.Request.Headers["X-Subject"])
		require.JSONEq(t, `{"resourceType":"Patient","identifier":[{"system":"http://fhir.nl/fhir/NamingSystem/bsn","value":"`+tc.pool+`"},`+
			`{"system":"http://fhir.nl/fhir/NamingSystem/bsn","value":"`+tc.others+`"}],"telecom":[{"value":"`+notABSN+`"}]}`,
			marshalled(t, event.Response.Body))
	}
}

// A body the viewer cannot show exactly says why, instead of looking empty.
func TestCaptureMarksBodiesItDidNotCapture(t *testing.T) {
	for _, tc := range []struct {
		name, contentType, body string
		readFully               bool
		want                    any
	}{
		{"oversized", "application/fhir+json", `{"text":"` + strings.Repeat("x", eventBodyLimit) + `"}`, true, "[not captured: body exceeds the 65536-byte capture limit]"},
		{"partially read", "application/fhir+json", `{"resourceType":"Patient"}`, false, "[not captured: the response body was not read to the end]"},
		{"binary", "image/png", "\x89PNG\xff\xfe", true, "[not captured: image/png body of 6 bytes]"},
		{"malformed JSON", "application/json", `{"access_token":"private-credential"`, true, "[not captured: malformed JSON body]"},
		{"text", "text/plain; charset=utf-8", "upstream said no", true, "upstream said no"},
		{"empty", "application/fhir+json", "", true, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var event stepEvent
			base := captureRoundTripFunc(func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {tc.contentType}},
					Body: io.NopCloser(strings.NewReader(tc.body)), ContentLength: -1}, nil
			})
			req, _ := http.NewRequestWithContext(withEventCapture(context.Background(), func(e stepEvent) { event = e }, false), "GET", "http://example.invalid/Patient", nil)
			res, err := newCaptureTransport("exchange", base).RoundTrip(req)
			require.NoError(t, err)
			if tc.readFully {
				_, _ = io.ReadAll(res.Body)
			} else {
				_, _ = io.ReadFull(res.Body, make([]byte, 5))
			}
			require.NoError(t, res.Body.Close())
			require.Equal(t, 200, event.Response.Status)
			require.Equal(t, tc.want, event.Response.Body)
			require.NotContains(t, marshalled(t, event), "private-")
		})
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
		if len(events) != 1 || events[0].Outcome != "error" {
			t.Fatalf("events = %#v", events)
		}
		if !network {
			require.Equal(t, "[not captured: the response body could not be read]", events[0].Response.Body)
		}
		data, _ := json.Marshal(events)
		if strings.Contains(string(data), "secret") {
			t.Fatalf("leaked error: %s", data)
		}
	}
}

func TestCaptureRealClientWiring(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		w.Header().Set("Content-Type", "application/fhir+json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/request-service-access-token"):
			_, _ = io.WriteString(w, `{"access_token":"private-token"}`)
		case strings.HasSuffix(r.URL.Path, "/introspect"):
			_, _ = io.WriteString(w, `{"active":true,"subject":"visible-subject"}`)
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
	if token, err := nuts.requestToken(ctx, authSession{Attestation: "private-attestation"}, server.URL); err != nil || token != "private-token" {
		t.Fatalf("token = %q, %v", token, err)
	}
	if _, err := nuts.introspect(ctx, "private-token"); err != nil {
		t.Fatal(err)
	}
	if _, err := retrieveBGZ(ctx, sourceAddress{Address: server.URL}, "private-token", bsn, []string{"Patient"}); err != nil {
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
	if strings.Contains(string(data), bsn) || strings.Contains(string(data), "private-") {
		t.Fatalf("client wiring leaked: %s", data)
	}
	require.Contains(t, string(data), "visible-subject", "introspection claims are shown as returned")
}

// HAPI answers chunked, and a final chunk that arrives after the JSON value is
// ordinary. A client that stops reading at the closing brace would leave the
// capture without the end of the body.
func TestCaptureSeesChunkedAnswersToDecodingClients(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/fhir+json")
		if strings.HasSuffix(r.URL.Path, "/introspect") {
			_, _ = io.WriteString(w, `{"active":true}`)
		} else {
			_, _ = io.WriteString(w, `{"resourceType":"Bundle","type":"searchset","entry":[]}`)
		}
		w.(http.Flusher).Flush()
		time.Sleep(100 * time.Millisecond)
	}))
	defer server.Close()
	base, _ := url.Parse(server.URL)
	for name, call := range map[string]func(context.Context){
		"directory lookup": func(ctx context.Context) { _, _ = mcsdResolveFunc(base)(ctx, "00000020") },
		"Mitz lookup":      func(ctx context.Context) { _, _ = mitzSubscribedFunc(base, "00000010")(ctx, "999900006") },
		"token introspection": func(ctx context.Context) {
			_, _ = newNutsClient(nutsConfig{InternalBaseURL: server.URL}).introspect(ctx, "private-token")
		},
	} {
		t.Run(name, func(t *testing.T) {
			var event stepEvent
			call(withEventCapture(context.Background(), func(e stepEvent) { event = e }, false))
			require.Equal(t, http.StatusOK, event.Response.Status)
			_, captured := event.Response.Body.(map[string]any)
			require.True(t, captured, "body: %v", event.Response.Body)
		})
	}
}
