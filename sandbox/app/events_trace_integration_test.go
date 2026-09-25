package main

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nuts-foundation/nuts-knooppunt/component/tracing"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

func TestEventHTTP_InternalPRSSpanJoinsOwningAction(t *testing.T) {
	previousProvider, previousPropagator := otel.GetTracerProvider(), otel.GetTextMapPropagator()
	t.Cleanup(func() {
		otel.SetTracerProvider(previousProvider)
		otel.SetTextMapPropagator(previousPropagator)
	})
	for _, clearRun := range []bool{false, true} {
		name := "live run"
		if clearRun {
			name = "run released during PRS call"
		}
		t.Run(name, func(t *testing.T) {
			entered, release := make(chan struct{}), make(chan struct{})
			var releaseOnce sync.Once
			unblock := func() { releaseOnce.Do(func() { close(release) }) }
			prs := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, "/oprf/eval", r.URL.Path)
				close(entered)
				<-release
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, `{"jwe":"secret-PRS-payload"}`)
			}))
			defer prs.Close()
			defer unblock()
			bridge, err := newEventTraceBridge(prs.URL, "nuts-knooppunt", "test-receiver-token")
			require.NoError(t, err)
			ingest := httptest.NewServer(bridge.handler())
			defer ingest.Close()
			exporter, err := otlptracehttp.New(t.Context(),
				otlptracehttp.WithEndpoint(strings.TrimPrefix(ingest.URL, "http://")),
				otlptracehttp.WithInsecure(),
				otlptracehttp.WithHeaders(map[string]string{"Authorization": "Bearer test-receiver-token"}))
			require.NoError(t, err)
			provider := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter),
				sdktrace.WithResource(resource.NewSchemaless(attribute.String("service.name", "nuts-knooppunt"))))
			defer func() { require.NoError(t, provider.Shutdown(context.Background())) }()
			otel.SetTracerProvider(provider)
			otel.SetTextMapPropagator(propagation.TraceContext{})
			upstream := httptest.NewServer(otelhttp.NewHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				req, err := http.NewRequestWithContext(r.Context(), http.MethodPost, prs.URL+"/oprf/eval", strings.NewReader(`{"encryptedPersonalId":"secret-input"}`))
				require.NoError(t, err)
				response, err := (&http.Client{Transport: tracing.WrapTransport(nil)}).Do(req)
				if err != nil {
					http.Error(w, "PRS unavailable", http.StatusBadGateway)
					return
				}
				_, _ = io.Copy(io.Discard, response.Body)
				_ = response.Body.Close()
				w.Header().Set("Content-Type", "application/fhir+json")
				_, _ = io.WriteString(w, `{"resourceType":"Bundle","type":"searchset","entry":[]}`)
			}), "NVI"))
			defer upstream.Close()
			cfg, err := NewConfigFromEnv(envOf(map[string]string{"KNOOPPUNT_INTERNAL_URL": upstream.URL}))
			require.NoError(t, err)
			cfg.eventTraces, cfg.Runs = bridge, newRunStore(cfg.Locks)
			demo, client := demoServer(t, cfg)
			openPatient(t, client, demo, "anna")
			owner := lockOwner(&authSession{ID: sessionCookieValue(t, client, demo.URL)})
			runID := cfg.Runs.current(owner, "anna")
			finished := make(chan error, 1)
			go func() {
				response, err := client.Get(demo.URL + "/demo/ehr/patients/anna")
				if err == nil {
					_, err = io.Copy(io.Discard, response.Body)
					_ = response.Body.Close()
				}
				finished <- err
			}()
			select {
			case <-entered:
			case <-time.After(5 * time.Second):
				t.Fatal("patient request did not reach PRS")
			}
			if clearRun {
				cfg.Runs.clearPatient("anna")
			}
			unblock()
			require.NoError(t, <-finished)
			snapshot, err := cfg.Runs.snapshot(runID, owner, 0)
			if clearRun {
				require.ErrorIs(t, err, errRunUnavailable)
				require.Empty(t, cfg.Runs.current(owner, "anna"))
				return
			}
			require.NoError(t, err)
			events := decodedSteps(t, snapshot)
			require.Len(t, events, 2)
			var parent, child stepEvent
			for _, event := range events {
				if event.GF == "pseudonym" {
					child = event
				} else {
					parent = event
				}
			}
			require.Equal(t, parent.CallID, child.ParentCallID)
			require.NotEmpty(t, child.ParentCallID)
			require.Equal(t, parent.ActionID, child.ActionID)
			require.Equal(t, "record", child.Action)
			require.Equal(t, "pseudonymization", child.Purpose)
			require.Equal(t, "ok", child.Outcome)
			require.Equal(t, 200, child.Response.Status)
			for _, raw := range snapshot.Events {
				require.NotContains(t, string(raw), "secret-")
				require.NotContains(t, string(raw), "test-receiver-token")
			}
		})
	}
}
