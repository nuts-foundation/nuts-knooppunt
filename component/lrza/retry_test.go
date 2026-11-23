package lrza

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
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

	t.Run("returns a 429 without retrying when Retry-After is longer than maxDelay", func(t *testing.T) {
		stub := &stubRoundTripper{responses: []func() (*http.Response, error){
			func() (*http.Response, error) {
				return statusResponse(http.StatusTooManyRequests, http.Header{"Retry-After": []string{"3600"}}), nil
			},
		}}
		// The deadline only keeps the test from waiting an hour if the transport does wait.
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://source.example/fhir/Organization", nil)
		require.NoError(t, err)

		resp, err := newTestTransport(stub).RoundTrip(req)
		require.NoError(t, err)
		require.Equal(t, http.StatusTooManyRequests, resp.StatusCode)
		require.Equal(t, "3600", resp.Header.Get("Retry-After"), "the caller must be able to pass it on")
		require.Equal(t, 1, stub.calls)
	})

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

func TestRetryTransport_RetryAfterLimit(t *testing.T) {
	transport := &retryTransport{maxAttempts: maxRetryAttempts, baseDelay: retryBaseDelay, maxDelay: retryMaxDelay}
	tooManyRequests := func(retryAfter string) *http.Response {
		return statusResponse(http.StatusTooManyRequests, http.Header{"Retry-After": []string{retryAfter}})
	}

	t.Run("a Retry-After of maxDelay is waited for", func(t *testing.T) {
		retryable, delay := transport.classifyResponse(0, http.MethodGet, tooManyRequests("30"), nil)
		require.True(t, retryable)
		require.Equal(t, 30*time.Second, delay)
	})

	t.Run("a longer Retry-After is not", func(t *testing.T) {
		retryable, _ := transport.classifyResponse(0, http.MethodGet, tooManyRequests("31"), nil)
		require.False(t, retryable)
	})

	t.Run("nor a Retry-After date further away than maxDelay", func(t *testing.T) {
		date := time.Now().Add(time.Hour).UTC().Format(http.TimeFormat)
		retryable, _ := transport.classifyResponse(0, http.MethodGet, tooManyRequests(date), nil)
		require.False(t, retryable)
	})
}

func TestRetryTransport_BackoffDelay(t *testing.T) {
	transport := &retryTransport{maxAttempts: maxRetryAttempts, baseDelay: retryBaseDelay, maxDelay: retryMaxDelay}
	// Without jitter the delays would be 1s, 2s, 4s, 8s, 16s, then capped at 30s.
	for attempt, ceiling := range []time.Duration{1 * time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second, 16 * time.Second, 30 * time.Second, 30 * time.Second} {
		t.Run(fmt.Sprintf("attempt %d waits between %s and %s", attempt, ceiling/2, ceiling), func(t *testing.T) {
			seen := map[time.Duration]bool{}
			for range 100 {
				delay := transport.backoffDelay(attempt)
				require.GreaterOrEqual(t, delay, ceiling/2)
				require.LessOrEqual(t, delay, ceiling)
				seen[delay] = true
			}
			require.Greater(t, len(seen), 1, "the delay must vary, so clients that fail together don't retry together")
		})
	}
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

// TestRetryTransportMethods covers the method-aware rule: a 429 means the request wasn't processed,
// so it is retried for every method, but a 5xx or network error may come after the server applied
// it, so only idempotent methods are retried then - a POST could otherwise register a resource twice.
func TestRetryTransportMethods(t *testing.T) {
	newRequest := func(t *testing.T, method string) *http.Request {
		t.Helper()
		req, err := http.NewRequestWithContext(context.Background(), method, "http://source.example/fhir/Endpoint", io.NopCloser(strings.NewReader(`{"resourceType":"Endpoint"}`)))
		require.NoError(t, err)
		return req
	}

	for _, tc := range []struct {
		method    string
		first     func() (*http.Response, error)
		wantCalls int
	}{
		{http.MethodPost, func() (*http.Response, error) { return statusResponse(http.StatusServiceUnavailable, nil), nil }, 1},
		{http.MethodPost, func() (*http.Response, error) { return nil, errors.New("connection reset") }, 1},
		{http.MethodPost, func() (*http.Response, error) {
			return statusResponse(http.StatusTooManyRequests, http.Header{"Retry-After": []string{"0"}}), nil
		}, 2},
		{http.MethodPut, func() (*http.Response, error) { return statusResponse(http.StatusServiceUnavailable, nil), nil }, 2},
		{http.MethodDelete, func() (*http.Response, error) { return statusResponse(http.StatusBadGateway, nil), nil }, 2},
	} {
		first, _ := tc.first()
		name := tc.method + " after "
		if first == nil {
			name += "network error"
		} else {
			name += http.StatusText(first.StatusCode)
		}
		t.Run(name, func(t *testing.T) {
			var bodies []string
			stub := &recordingRoundTripper{responses: []func() (*http.Response, error){
				tc.first,
				func() (*http.Response, error) { return statusResponse(http.StatusOK, nil), nil },
			}, bodies: &bodies}

			transport := &retryTransport{next: stub, maxAttempts: maxRetryAttempts, baseDelay: time.Millisecond, maxDelay: 10 * time.Millisecond}
			resp, err := transport.RoundTrip(newRequest(t, tc.method))

			require.Equal(t, tc.wantCalls, stub.calls)
			if tc.wantCalls == 1 {
				if err == nil {
					require.NotEqual(t, http.StatusOK, resp.StatusCode)
				}
				return
			}
			require.NoError(t, err)
			require.Equal(t, http.StatusOK, resp.StatusCode)
			require.Equal(t, []string{`{"resourceType":"Endpoint"}`, `{"resourceType":"Endpoint"}`}, bodies, "a retry must resend the body")
		})
	}
}

// recordingRoundTripper is like stubRoundTripper, but also records each attempt's request body.
type recordingRoundTripper struct {
	responses []func() (*http.Response, error)
	calls     int
	bodies    *[]string
}

func (s *recordingRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.Body != nil {
		body, _ := io.ReadAll(req.Body)
		*s.bodies = append(*s.bodies, string(body))
	}
	resp, err := s.responses[s.calls]()
	s.calls++
	return resp, err
}
