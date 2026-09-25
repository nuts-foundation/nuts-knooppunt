package main

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/nuts-foundation/nuts-knooppunt/test/testdata/vectors/pool"
	"github.com/stretchr/testify/require"
)

type streamedFrame struct{ event, id, data string }

func readEventFrame(t *testing.T, reader *bufio.Reader) streamedFrame {
	t.Helper()
	var frame streamedFrame
	for {
		line, err := reader.ReadString('\n')
		require.NoError(t, err)
		line = strings.TrimSuffix(line, "\n")
		if line == "" {
			if frame.event != "" {
				return frame
			}
			continue
		}
		if value, ok := strings.CutPrefix(line, "event: "); ok {
			frame.event = value
		}
		if value, ok := strings.CutPrefix(line, "id: "); ok {
			frame.id = value
		}
		if value, ok := strings.CutPrefix(line, "data: "); ok {
			frame.data = value
		}
	}
}

func eventDemo(t *testing.T) (Config, *httptest.Server, *http.Client, string, string) {
	t.Helper()
	cfg := testConfig()
	cfg.Runs = newRunStore(cfg.Locks)
	srv, client := demoServer(t, cfg)
	openPatient(t, client, srv, "anna")
	owner := lockOwner(&authSession{ID: sessionCookieValue(t, client, srv.URL)})
	id := cfg.Runs.current(owner, "anna")
	require.NotEmpty(t, id)
	return cfg, srv, client, owner, id
}

func TestEventHTTP_ReplayLiveDeliveryAndResume(t *testing.T) {
	cfg, srv, client, _, id := eventDemo(t)
	for range 2 {
		require.True(t, cfg.Runs.append(id, stepEvent{GF: "localization", Outcome: "ok"}))
	}
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"/demo/runs/"+id+"/events", nil)
	require.NoError(t, err)
	req.Header.Set("Last-Event-ID", id+":1")
	res, err := client.Do(req)
	require.NoError(t, err)
	defer res.Body.Close()
	require.Equal(t, http.StatusOK, res.StatusCode)
	require.Equal(t, "text/event-stream", res.Header.Get("Content-Type"))
	require.Equal(t, "no-store", res.Header.Get("Cache-Control"))
	reader := bufio.NewReader(res.Body)
	frame := readEventFrame(t, reader)
	require.Equal(t, "step", frame.event)
	require.Equal(t, id+":2", frame.id)
	var event stepEvent
	require.NoError(t, json.Unmarshal([]byte(frame.data), &event))
	require.Equal(t, uint64(2), event.Seq)
	require.Equal(t, id, event.RunID)
	frame = readEventFrame(t, reader)
	require.Equal(t, "snapshot", frame.event)
	require.JSONEq(t, `{"lastSeq":2}`, frame.data)
	require.True(t, cfg.Runs.append(id, stepEvent{GF: "exchange", Outcome: "deny"}))
	frame = readEventFrame(t, reader)
	require.Equal(t, id+":3", frame.id)
	require.Contains(t, frame.data, `"outcome":"deny"`)
	frame = readEventFrame(t, reader)
	require.Equal(t, "snapshot", frame.event)
	require.JSONEq(t, `{"lastSeq":3}`, frame.data)
	cfg.Runs.clearPatient("anna")
	frame = readEventFrame(t, reader)
	require.Equal(t, "run-ended", frame.event)
	_, err = reader.ReadString('\n')
	require.ErrorIs(t, err, io.EOF)
}

func TestEventHTTP_AuthorizationAndCursorChecks(t *testing.T) {
	cfg, srv, client, _, id := eventDemo(t)
	cfg.Runs.append(id, stepEvent{GF: "localization"})
	other := signInViaDezi(t, srv)
	for _, tc := range []struct {
		name   string
		client *http.Client
		cursor string
		origin string
		status int
	}{
		{"anonymous", http.DefaultClient, "", "", http.StatusUnauthorized},
		{"another session", other, "", "", http.StatusNotFound},
		{"another session malformed cursor", other, "broken", "", http.StatusNotFound},
		{"wrong run", client, "another:1", "", http.StatusBadRequest},
		{"future sequence", client, id + ":2", "", http.StatusBadRequest},
		{"malformed", client, "broken", "", http.StatusBadRequest},
		{"negative", client, id + ":-1", "", http.StatusBadRequest},
		{"cross origin", client, "", "https://unrelated.example", http.StatusForbidden},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodGet, srv.URL+"/demo/runs/"+id+"/events", nil)
			require.NoError(t, err)
			req.Header.Set("Last-Event-ID", tc.cursor)
			req.Header.Set("Origin", tc.origin)
			res, err := tc.client.Do(req)
			require.NoError(t, err)
			defer res.Body.Close()
			require.Equal(t, tc.status, res.StatusCode)
			body, err := io.ReadAll(res.Body)
			require.NoError(t, err)
			require.NotContains(t, string(body), `"gf"`)
		})
	}
}

// A GET route also serves HEAD. net/http discards body writes for HEAD, so a
// handler that streams would never see a write error and would hold the
// connection until the run ends.
func TestEventHTTP_HeadAnswersWithoutStreaming(t *testing.T) {
	_, srv, client, _, id := eventDemo(t)
	other := signInViaDezi(t, srv)
	for _, tc := range []struct {
		name   string
		client *http.Client
		status int
	}{
		{"owner", client, http.StatusOK},
		{"another session", other, http.StatusNotFound},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			req := httptest.NewRequestWithContext(ctx, http.MethodHead, "/demo/runs/"+id+"/events", nil)
			req.AddCookie(&http.Cookie{Name: sessionCookie, Value: sessionCookieValue(t, tc.client, srv.URL)})
			res := httptest.NewRecorder()
			done := make(chan struct{})
			go func() {
				defer close(done)
				srv.Config.Handler.ServeHTTP(res, req)
			}()
			select {
			case <-done:
			case <-time.After(2 * time.Second):
				cancel()
				<-done
				t.Fatal("HEAD entered the event stream instead of answering")
			}
			require.Equal(t, tc.status, res.Code)
			if tc.status == http.StatusOK {
				require.Equal(t, "text/event-stream", res.Header().Get("Content-Type"))
				require.Equal(t, "no-store", res.Header().Get("Cache-Control"))
				require.Zero(t, res.Body.Len())
			}
		})
	}
}

func TestEventHTTP_RetentionGapIsExplicit(t *testing.T) {
	cfg, srv, client, _, id := eventDemo(t)
	cfg.Runs.maxEvents = 2
	for range 4 {
		cfg.Runs.append(id, stepEvent{GF: "localization"})
	}
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"/demo/runs/"+id+"/events", nil)
	require.NoError(t, err)
	res, err := client.Do(req)
	require.NoError(t, err)
	defer res.Body.Close()
	reader := bufio.NewReader(res.Body)
	gap := readEventFrame(t, reader)
	require.Equal(t, "replay-gap", gap.event)
	require.JSONEq(t, `{"firstSeq":3}`, gap.data)
	require.Equal(t, id+":3", readEventFrame(t, reader).id)
	require.Equal(t, id+":4", readEventFrame(t, reader).id)
}

func TestEventHTTP_PatientRefreshKeepsRunAndLogoutClearsIt(t *testing.T) {
	cfg, srv, client, owner, id := eventDemo(t)
	for range 2 {
		status, body := getPageWithClient(t, client, srv.URL+"/demo/ehr/patients/anna")
		require.Equal(t, http.StatusOK, status)
		require.Contains(t, body, `data-run-id="`+id+`"`)
		require.Equal(t, id, cfg.Runs.current(owner, "anna"))
	}
	res := postForm(t, client, srv, "/demo/logout", nil)
	res.Body.Close()
	require.Empty(t, cfg.Runs.current(owner, "anna"))
	require.False(t, cfg.Runs.append(id, stepEvent{GF: "exchange"}))
}

func TestEventHTTP_RunCapacityReturnsUnavailableWithoutTakingLock(t *testing.T) {
	for _, path := range []string{"/demo/ehr/patients/pool-02/open", "/demo/patients/pool-02/lock"} {
		t.Run(path, func(t *testing.T) {
			cfg := testConfig()
			cfg.Runs = newRunStore(cfg.Locks)
			cfg.Runs.maxRuns = 1
			srv, first := demoServer(t, cfg)
			openPatient(t, first, srv, "anna")
			other := signInViaDezi(t, srv)
			res := postForm(t, other, srv, path, nil)
			defer res.Body.Close()
			require.Equal(t, http.StatusServiceUnavailable, res.StatusCode)
			require.False(t, cfg.Locks.IsLocked("pool-02"))
			require.True(t, cfg.Locks.IsLocked("anna"))
		})
	}
}

func TestEventHTTP_SessionExpiryDuringPatientOpenReturnsToLogin(t *testing.T) {
	for _, manual := range []bool{false, true} {
		t.Run(map[bool]string{false: "open", true: "manual lock"}[manual], func(t *testing.T) {
			cfg := testConfig()
			cfg.Runs = newRunStore(cfg.Locks)
			cfg.sessions = newSessionStore()
			now := time.Now()
			cfg.sessions.now = func() time.Time { return now }
			id := cfg.sessions.create(authSession{ExpiresAt: now.Add(time.Minute)})
			session, ok := cfg.sessions.get(id)
			require.True(t, ok)
			now = now.Add(2 * time.Minute)
			req := httptest.NewRequest(http.MethodPost, "/", nil)
			req.SetPathValue("key", "anna")
			res := httptest.NewRecorder()
			handler := cfg.handleOpenPatient
			if manual {
				handler = cfg.handleLock
			}
			handler(res, req, &session)
			require.Equal(t, http.StatusSeeOther, res.Code)
			require.Equal(t, "/demo/login", res.Header().Get("Location"))
			require.False(t, cfg.Locks.IsLocked("anna"))
		})
	}
}

func TestEventIdentifierConfig_DefaultsMaskedAndRestrictsSyntheticMode(t *testing.T) {
	for _, tc := range []struct {
		mode, public    string
		reveal, invalid bool
	}{
		{"", "", false, false},
		{"masked", "https://sandbox.example", false, false},
		{"synthetic", "http://localhost:8091", true, false},
		{"synthetic", "http://127.0.0.1:8091", true, false},
		{"synthetic", "http://[::1]:8091", true, false},
		{"synthetic", "https://sandbox.example", false, true},
		{"synthetic", "http://localhost.attacker.example", false, true},
		{"anything", "", false, true},
	} {
		t.Run(tc.mode+tc.public, func(t *testing.T) {
			cfg, err := NewConfigFromEnv(func(key string) string {
				switch key {
				case "SANDBOX_EVENT_IDENTIFIERS":
					return tc.mode
				case "SANDBOX_PUBLIC_URL":
					return tc.public
				}
				return ""
			})
			if tc.invalid {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tc.reveal, cfg.revealEventIdentifiers)
			}
		})
	}
}

func TestEventHTTP_ActualPatientCallsAreSanitizedBeforeRetention(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/nvi/List/_search" || r.Method != http.MethodPost {
			t.Errorf("unexpected upstream call: %s %s", r.Method, r.URL.Path)
			http.Error(w, "unexpected call", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/fhir+json")
		w.Header().Set("Set-Cookie", "private-cookie=secret-cookie")
		_, _ = io.WriteString(w, `{"resourceType":"Bundle","type":"searchset","total":0,"access_token":"secret-token","text":{"div":"secret-patient-text"}}`)
	}))
	defer upstream.Close()
	cfg, err := NewConfigFromEnv(func(key string) string {
		if key == "KNOOPPUNT_INTERNAL_URL" {
			return upstream.URL
		}
		return ""
	})
	require.NoError(t, err)
	cfg.Runs = newRunStore(cfg.Locks)
	srv, client := demoServer(t, cfg)
	openPatient(t, client, srv, "anna")
	for _, site := range []string{"same-origin", "none", "cross-site", "same-site"} {
		req, err := http.NewRequest(http.MethodGet, srv.URL+"/demo/ehr/patients/anna", nil)
		require.NoError(t, err)
		req.Header.Set("Sec-Fetch-Site", site)
		res, err := client.Do(req)
		require.NoError(t, err)
		_, err = io.Copy(io.Discard, res.Body)
		require.NoError(t, err)
		res.Body.Close()
		require.Equal(t, http.StatusOK, res.StatusCode)
	}
	for _, action := range []string{"share", "subscribe", "retrieve"} {
		req, err := http.NewRequest(http.MethodPost, srv.URL+"/demo/ehr/patients/anna/"+action, nil)
		require.NoError(t, err)
		req.Header.Set("Sec-Fetch-Site", "cross-site")
		res, err := client.Do(req)
		require.NoError(t, err)
		res.Body.Close()
		require.Equal(t, http.StatusForbidden, res.StatusCode, "capture must not weaken the mutation guards")
	}
	owner := lockOwner(&authSession{ID: sessionCookieValue(t, client, srv.URL)})
	id := cfg.Runs.current(owner, "anna")
	snapshot, err := cfg.Runs.snapshot(id, owner, 0)
	require.NoError(t, err)
	events := decodedSteps(t, snapshot)
	require.Len(t, events, 4, "every authenticated record navigation captures its actual calls")
	require.Equal(t, "localization", events[0].GF)
	require.Equal(t, "ok", events[0].Outcome)
	seenActions := make(map[string]bool)
	for _, event := range events {
		require.Equal(t, "record", event.Action)
		require.Equal(t, "status", event.Purpose)
		require.NotEmpty(t, event.ActionID)
		require.False(t, seenActions[event.ActionID], "each navigation is a distinct action")
		seenActions[event.ActionID] = true
	}
	for _, raw := range snapshot.Events {
		for _, forbidden := range []string{"secret-token", "secret-cookie", "secret-patient-text", pool.Patients()[0].BSN, sessionCookie} {
			require.NotContains(t, string(raw), forbidden)
		}
	}
}

func TestEventHTTP_HTTP2ConnectionSurvivesIdleBetweenWrites(t *testing.T) {
	cfg := testConfig()
	cfg.Runs = newRunStore(cfg.Locks)
	session := &authSession{ID: "test-session", ExpiresAt: time.Now().Add(time.Hour)}
	id, err := cfg.Runs.start(lockOwner(session), "anna", session.ExpiresAt, true)
	require.NoError(t, err)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /demo/runs/{runId}/events", func(w http.ResponseWriter, r *http.Request) {
		cfg.handleEvents(w, r, session)
	})
	srv := httptest.NewUnstartedServer(mux)
	srv.EnableHTTP2 = true
	srv.StartTLS()
	defer srv.Close()
	ctx, cancel := context.WithTimeout(t.Context(), 14*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"/demo/runs/"+id+"/events", nil)
	require.NoError(t, err)
	res, err := srv.Client().Do(req)
	require.NoError(t, err)
	defer res.Body.Close()
	require.Equal(t, 2, res.ProtoMajor)
	reader := bufio.NewReader(res.Body)
	require.Equal(t, "snapshot", readEventFrame(t, reader).event)
	// Longer than the per-write deadline, shorter than the heartbeat. A healthy
	// reader must still receive this event after the connection has been idle.
	emit := time.AfterFunc(11*time.Second, func() { cfg.Runs.append(id, stepEvent{GF: "exchange", Outcome: "ok"}) })
	defer emit.Stop()
	require.Equal(t, id+":1", readEventFrame(t, reader).id)
}
