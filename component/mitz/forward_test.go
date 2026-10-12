package mitz

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type forwardTarget struct {
	mu     sync.Mutex
	server *httptest.Server
	bodies [][]byte
	types  []string
	status int
	delay  time.Duration
	header http.Header
}

func newForwardTarget(t *testing.T) *forwardTarget {
	t.Helper()
	target := &forwardTarget{status: http.StatusNoContent, header: http.Header{}}
	target.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		target.mu.Lock()
		target.bodies, target.types = append(target.bodies, body), append(target.types, r.Header.Get("Content-Type"))
		status, delay := target.status, target.delay
		for key, values := range target.header {
			w.Header()[key] = values
		}
		target.mu.Unlock()
		time.Sleep(delay)
		w.WriteHeader(status)
	}))
	t.Cleanup(target.server.Close)
	return target
}

// notify posts to the registered route, as Mitz does, so the middleware, the
// generated binding and both of its paths are all on the path under test.
func notify(t *testing.T, forwardURL, contentType string, body []byte) int {
	t.Helper()
	component, err := New(Config{MitzBase: "http://example.com", NotifyForwardURL: forwardURL})
	require.NoError(t, err)
	publicMux := http.NewServeMux()
	component.RegisterHttpHandlers(publicMux, http.NewServeMux())
	req := httptest.NewRequest(http.MethodPost, "/mitz/notify", bytes.NewReader(body))
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	recorder := httptest.NewRecorder()
	publicMux.ServeHTTP(recorder, req)
	return recorder.Code
}

func TestForward_RelaysTheBodyByteForByteWithItsContentType(t *testing.T) {
	for _, tc := range []struct{ name, contentType, body string }{
		{"json", "application/fhir+json", `{"resourceType":"Bundle",  "type":"transaction"}`},
		{"xml", "application/fhir+xml", `<Bundle xmlns="http://hl7.org/fhir"><type value="transaction"/></Bundle>`},
		{"xml declared as json", "application/fhir+json", `<Bundle/>`},
		{"empty", "application/fhir+json", ``},
	} {
		t.Run(tc.name, func(t *testing.T) {
			target := newForwardTarget(t)
			status := notify(t, target.server.URL+"/intake", tc.contentType, []byte(tc.body))

			require.Equal(t, http.StatusNoContent, status)
			require.Len(t, target.bodies, 1)
			require.Equal(t, tc.body, string(target.bodies[0]), "not reserialized: the bytes received are the bytes sent")
			require.Equal(t, tc.contentType, target.types[0])
		})
	}
}

func TestForward_AnswersBadGatewayWhenDeliveryIsNotConfirmed(t *testing.T) {
	body := []byte(`{"resourceType":"Bundle"}`)

	t.Run("the target refuses the connection", func(t *testing.T) {
		target := newForwardTarget(t)
		url := target.server.URL
		target.server.Close()
		require.Equal(t, http.StatusBadGateway, notify(t, url, "application/fhir+json", body))
	})
	t.Run("the target answers an error", func(t *testing.T) {
		target := newForwardTarget(t)
		target.status = http.StatusInternalServerError
		require.Equal(t, http.StatusBadGateway, notify(t, target.server.URL, "application/fhir+json", body))
		require.Len(t, target.bodies, 1, "it was reached, and its answer is not a confirmation")
	})
	t.Run("the target redirects", func(t *testing.T) {
		elsewhere := newForwardTarget(t)
		target := newForwardTarget(t)
		target.status = http.StatusFound
		target.header.Set("Location", elsewhere.server.URL)
		require.Equal(t, http.StatusBadGateway, notify(t, target.server.URL, "application/fhir+json", body))
		require.Empty(t, elsewhere.bodies, "a redirect is not followed")
	})
	t.Run("the target answers too late", func(t *testing.T) {
		previous := forwardTimeout
		forwardTimeout = 50 * time.Millisecond
		defer func() { forwardTimeout = previous }()
		target := newForwardTarget(t)
		target.delay = 400 * time.Millisecond
		require.Equal(t, http.StatusBadGateway, notify(t, target.server.URL, "application/fhir+json", body))
	})
	t.Run("the same holds on the path a non-JSON body takes", func(t *testing.T) {
		target := newForwardTarget(t)
		target.status = http.StatusInternalServerError
		require.Equal(t, http.StatusBadGateway, notify(t, target.server.URL, "application/fhir+xml", []byte(`<Bundle/>`)))
	})
}

func TestForward_RefusesABodyOverTheLimit(t *testing.T) {
	target := newForwardTarget(t)
	big := []byte(`{"x":"` + strings.Repeat("a", notifyBodyLimit) + `"}`)
	require.Equal(t, http.StatusRequestEntityTooLarge, notify(t, target.server.URL, "application/fhir+json", big))
	require.Empty(t, target.bodies)
}

func TestForward_UnsetLeavesTheEndpointAsItWas(t *testing.T) {
	for _, tc := range []struct{ name, contentType, body string }{
		{"json", "application/fhir+json", `{"resourceType":"Bundle"}`},
		{"xml", "application/fhir+json", `<Bundle/>`},
		{"empty", "", ``},
		{"oversized", "application/fhir+json", `{"x":"` + strings.Repeat("a", notifyBodyLimit) + `"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, http.StatusNoContent, notify(t, "", tc.contentType, []byte(tc.body)))
		})
	}
}

func TestNew_RefusesAForwardURLThatIsNotAnAbsoluteHTTPURL(t *testing.T) {
	for _, bad := range []string{"gf-sandbox:8093/mitz/notify", "ftp://gf-sandbox/mitz/notify", "/mitz/notify"} {
		_, err := New(Config{MitzBase: "http://example.com", NotifyForwardURL: bad})
		require.Errorf(t, err, "%q", bad)
	}
}
