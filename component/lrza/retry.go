package lrza

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/nuts-foundation/nuts-knooppunt/lib/logging"
)

// maxRetryAttempts is the number of retries (on top of the initial attempt) for a transient failure
// talking to the LRZA source.
const maxRetryAttempts = 5

// retryBaseDelay and retryMaxDelay bound the exponential backoff used for 5xx/network failures and for
// 429 when the server doesn't advertise a Retry-After duration: 1s, 2s, 4s, 8s, 16s, capped at 30s.
const (
	retryBaseDelay = 1 * time.Second
	retryMaxDelay  = 30 * time.Second
)

// retryTransport retries requests to the LRZA source on the transient conditions the NL-GF Care
// Services spec expects a client to recover from by itself: HTTP 429 (rate limiting, honoring
// Retry-After) and HTTP 5xx or network errors (exponential backoff). Every other response - including
// 400 (malformed request), 401/403 (auth) and 404/422 - is returned to the caller on the first attempt
// unchanged, since the spec's guidance for those is to fix the request or credentials, not repeat it;
// retrying them can't succeed and would just mask the real cause.
//
// The rule is method-aware, because the registration write path shares this transport (see
// isIdempotent): a 429 means the server didn't process the request, so it is retried for every
// method, but a 5xx or network error may have come after the server applied it. Only idempotent
// methods (GET, PUT, DELETE) are retried then; a POST (a create or a transaction) is not, so a retry
// can't register a resource twice.
type retryTransport struct {
	next        http.RoundTripper
	maxAttempts int           // retries on top of the initial attempt
	baseDelay   time.Duration // exponential backoff starting point
	maxDelay    time.Duration // exponential backoff cap
}

func wrapRetryTransport(next http.RoundTripper) http.RoundTripper {
	return &retryTransport{
		next:        next,
		maxAttempts: maxRetryAttempts,
		baseDelay:   retryBaseDelay,
		maxDelay:    retryMaxDelay,
	}
}

func (t *retryTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	newBody, err := replayableBody(req)
	if err != nil {
		return nil, err
	}
	for attempt := 0; ; attempt++ {
		attemptReq := req.Clone(req.Context())
		if newBody != nil {
			attemptReq.Body = newBody()
		}
		resp, err := t.next.RoundTrip(attemptReq)
		retryable, delay := t.classifyResponse(attempt, req.Method, resp, err)
		if !retryable || attempt >= t.maxAttempts {
			return resp, err
		}

		// On a network error there's no response to read, so status and retryAfter stay "N/A".
		status := "N/A"
		retryAfter := "N/A"
		if resp != nil {
			status = strconv.Itoa(resp.StatusCode)
			if ra := resp.Header.Get("Retry-After"); ra != "" {
				retryAfter = ra
			}
			// resp.Body streams directly off the connection; closing it without reading to EOF first
			// leaves unread bytes on the wire, so the Transport can't safely hand that connection back
			// to the pool and opens a new one (new TCP/mTLS handshake) for the retry instead. Draining
			// it here lets the Transport reuse the existing connection.
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
		}
		slog.WarnContext(req.Context(), "Retrying LRZA request after transient failure",
			slog.String("url", req.URL.String()),
			slog.String("http status", status),
			slog.Int("attempt", attempt+1),
			slog.String("retryAfter", retryAfter),
			slog.Duration("delay", delay),
			logging.Error(err))
		if waitErr := sleep(req.Context(), delay); waitErr != nil {
			return nil, waitErr
		}
	}
}

// replayableBody returns a function yielding a fresh copy of the request body for each attempt, or
// nil if the request has no body. The FHIR client doesn't set GetBody, so without this a retried PUT
// or POST would be sent with the body the first attempt already consumed.
func replayableBody(req *http.Request) (func() io.ReadCloser, error) {
	if req.Body == nil || req.Body == http.NoBody {
		return nil, nil
	}
	if req.GetBody != nil {
		return func() io.ReadCloser {
			body, err := req.GetBody()
			if err != nil {
				return io.NopCloser(&errReader{err: err})
			}
			return body
		}, nil
	}
	data, err := io.ReadAll(req.Body)
	_ = req.Body.Close()
	if err != nil {
		return nil, fmt.Errorf("failed to read request body: %w", err)
	}
	return func() io.ReadCloser { return io.NopCloser(bytes.NewReader(data)) }, nil
}

type errReader struct{ err error }

func (r *errReader) Read(_ []byte) (int, error) { return 0, r.err }

// isIdempotent reports whether a request with this method can safely be repeated after a failure
// that may have happened after the server applied it.
func isIdempotent(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodPut, http.MethodDelete:
		return true
	default:
		return false
	}
}

// classifyResponse decides whether a response/error is worth retrying and, if so, how long to wait
// before the next attempt. See retryTransport for why 5xx and network errors depend on the method.
func (t *retryTransport) classifyResponse(attempt int, method string, resp *http.Response, err error) (retryable bool, delay time.Duration) {
	if err != nil {
		if !isIdempotent(method) {
			return false, 0
		}
		return true, t.backoffDelay(attempt)
	}
	switch {
	case resp.StatusCode == http.StatusTooManyRequests:
		if d, ok := parseRetryAfter(resp.Header.Get("Retry-After")); ok {
			return true, d
		}
		return true, t.backoffDelay(attempt)
	case resp.StatusCode >= 500 && isIdempotent(method):
		return true, t.backoffDelay(attempt)
	default:
		return false, 0
	}
}

func (t *retryTransport) backoffDelay(attempt int) time.Duration {
	delay := t.baseDelay << attempt // 1<<attempt doublings; attempt is always small (<= maxAttempts)
	if delay <= 0 || delay > t.maxDelay {
		return t.maxDelay
	}
	return delay
}

// parseRetryAfter parses the Retry-After header per RFC 9110: either a non-negative number of seconds
// or an HTTP-date. ok is false when the header is absent or malformed, so the caller falls back to
// exponential backoff.
func parseRetryAfter(value string) (delay time.Duration, ok bool) {
	if value == "" {
		return 0, false
	}
	if seconds, err := strconv.Atoi(value); err == nil {
		if seconds < 0 {
			return 0, false
		}
		return time.Duration(seconds) * time.Second, true
	}
	if t, err := http.ParseTime(value); err == nil {
		if d := time.Until(t); d > 0 {
			return d, true
		}
		return 0, true
	}
	return 0, false
}

// sleep waits for delay, returning early with the context's error if it's cancelled first.
func sleep(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
