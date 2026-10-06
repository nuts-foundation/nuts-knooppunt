package lrza

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// stubRoundTripper returns one response (or error) per call, in order, and counts how many times it
// was invoked.
type stubRoundTripper struct {
	responses []func() (*http.Response, error)
	calls     int
}

func (s *stubRoundTripper) RoundTrip(_ *http.Request) (*http.Response, error) {
	resp, err := s.responses[s.calls]()
	s.calls++
	return resp, err
}

func newTestTransport(stub *stubRoundTripper) *retryTransport {
	return &retryTransport{
		next:        stub,
		maxAttempts: maxRetryAttempts,
		baseDelay:   time.Millisecond,
		maxDelay:    10 * time.Millisecond,
	}
}

func statusResponse(status int, headers http.Header) *http.Response {
	if headers == nil {
		headers = http.Header{}
	}
	return &http.Response{StatusCode: status, Header: headers, Body: http.NoBody}
}

func newTestRequest(t *testing.T) *http.Request {
	t.Helper()
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://source.example/fhir/Organization", nil)
	require.NoError(t, err)
	return req
}

func TestRetryTransport(t *testing.T) {
	t.Run("retries 429 honoring Retry-After then succeeds", func(t *testing.T) {
		stub := &stubRoundTripper{responses: []func() (*http.Response, error){
			func() (*http.Response, error) {
				return statusResponse(http.StatusTooManyRequests, http.Header{"Retry-After": []string{"0"}}), nil
			},
			func() (*http.Response, error) {
				return statusResponse(http.StatusOK, nil), nil
			},
		}}
		resp, err := newTestTransport(stub).RoundTrip(newTestRequest(t))
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.StatusCode)
		require.Equal(t, 2, stub.calls)
	})

	t.Run("retries 5xx with exponential backoff then succeeds", func(t *testing.T) {
		stub := &stubRoundTripper{responses: []func() (*http.Response, error){
			func() (*http.Response, error) { return statusResponse(http.StatusServiceUnavailable, nil), nil },
			func() (*http.Response, error) { return statusResponse(http.StatusServiceUnavailable, nil), nil },
			func() (*http.Response, error) { return statusResponse(http.StatusOK, nil), nil },
		}}
		resp, err := newTestTransport(stub).RoundTrip(newTestRequest(t))
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.StatusCode)
		require.Equal(t, 3, stub.calls)
	})

	t.Run("retries network errors then succeeds", func(t *testing.T) {
		stub := &stubRoundTripper{responses: []func() (*http.Response, error){
			func() (*http.Response, error) { return nil, errors.New("connection reset") },
			func() (*http.Response, error) { return statusResponse(http.StatusOK, nil), nil },
		}}
		resp, err := newTestTransport(stub).RoundTrip(newTestRequest(t))
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.StatusCode)
		require.Equal(t, 2, stub.calls)
	})

	t.Run("gives up after maxAttempts", func(t *testing.T) {
		stub := &stubRoundTripper{responses: []func() (*http.Response, error){
			func() (*http.Response, error) { return statusResponse(http.StatusServiceUnavailable, nil), nil },
			func() (*http.Response, error) { return statusResponse(http.StatusServiceUnavailable, nil), nil },
			func() (*http.Response, error) { return statusResponse(http.StatusServiceUnavailable, nil), nil },
		}}
		transport := newTestTransport(stub)
		transport.maxAttempts = 2
		resp, err := transport.RoundTrip(newTestRequest(t))
		require.NoError(t, err)
		require.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)
		require.Equal(t, 3, stub.calls) // initial attempt + 2 retries, then give up
	})

	for _, status := range []int{http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound, http.StatusUnprocessableEntity} {
		t.Run("does not retry "+http.StatusText(status), func(t *testing.T) {
			stub := &stubRoundTripper{responses: []func() (*http.Response, error){
				func() (*http.Response, error) { return statusResponse(status, nil), nil },
			}}
			resp, err := newTestTransport(stub).RoundTrip(newTestRequest(t))
			require.NoError(t, err)
			require.Equal(t, status, resp.StatusCode)
			require.Equal(t, 1, stub.calls)
		})
	}

	t.Run("stops retrying when the context is cancelled during backoff", func(t *testing.T) {
		stub := &stubRoundTripper{responses: []func() (*http.Response, error){
			func() (*http.Response, error) { return statusResponse(http.StatusServiceUnavailable, nil), nil },
			func() (*http.Response, error) { return statusResponse(http.StatusOK, nil), nil },
		}}
		transport := newTestTransport(stub)
		transport.baseDelay = time.Hour // never fires before cancellation

		ctx, cancel := context.WithCancel(context.Background())
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://source.example/fhir/Organization", nil)
		require.NoError(t, err)
		cancel()

		_, err = transport.RoundTrip(req)
		require.ErrorIs(t, err, context.Canceled)
		require.Equal(t, 1, stub.calls)
	})
}

func TestParseRetryAfter(t *testing.T) {
	t.Run("seconds form", func(t *testing.T) {
		d, ok := parseRetryAfter("5")
		require.True(t, ok)
		require.Equal(t, 5*time.Second, d)
	})

	t.Run("negative seconds is rejected", func(t *testing.T) {
		_, ok := parseRetryAfter("-5")
		require.False(t, ok)
	})

	t.Run("HTTP-date form", func(t *testing.T) {
		future := time.Now().Add(1 * time.Hour)
		d, ok := parseRetryAfter(future.UTC().Format(http.TimeFormat))
		require.True(t, ok)
		require.InDelta(t, time.Hour, d, float64(5*time.Second))
	})

	t.Run("empty is absent", func(t *testing.T) {
		_, ok := parseRetryAfter("")
		require.False(t, ok)
	})

	t.Run("garbage is absent", func(t *testing.T) {
		_, ok := parseRetryAfter("not-a-date")
		require.False(t, ok)
	})
}

// TestNewSourceHTTPClientRetriesTransientFailures is a light integration check that
// wrapRetryTransport is actually wired into the LRZA source client, using a real HTTP server.
func TestNewSourceHTTPClientRetriesTransientFailures(t *testing.T) {
	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls < 2 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client, err := newSourceHTTPClient(Config{LRZABaseUrl: server.URL})
	require.NoError(t, err)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, server.URL, nil)
	require.NoError(t, err)
	resp, err := client.Do(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, 2, calls)
}
