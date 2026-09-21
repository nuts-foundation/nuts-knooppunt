package subscription

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/nuts-foundation/nuts-knooppunt/lib/fhirsubscription"
	"github.com/nuts-foundation/nuts-knooppunt/lib/logging"
)

// deliverResult is the outcome of a delivery attempt sequence.
type deliverResult struct {
	// StatusCode of the final attempt; 0 when the final attempt failed at network level.
	StatusCode int
	// Attempts actually made.
	Attempts int
	// Err is set when no attempt returned 2xx.
	Err error
}

func (r deliverResult) Outcome() string {
	if r.Err == nil {
		return fmt.Sprintf("%d", r.StatusCode)
	}
	return fmt.Sprintf("failed after %d attempts: %v", r.Attempts, r.Err)
}

// deliver POSTs a notification Bundle to endpoint with exponential-backoff retry.
// Per the spec's retry obligation, transient errors (5xx, network errors) are retried;
// 4xx responses are treated as definitive rejections and not retried.
func (c *Component) deliver(ctx context.Context, endpoint string, bundle interface{}) deliverResult {
	body, err := json.Marshal(bundle)
	if err != nil {
		return deliverResult{Err: fmt.Errorf("marshal notification: %w", err)}
	}

	baseDelay := time.Duration(c.config.Server.RetryBaseDelayMillis) * time.Millisecond
	maxAttempts := c.config.Server.RetryMaxAttempts
	if maxAttempts < 1 {
		maxAttempts = 1
	}

	var lastErr error
	var lastStatus int
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		status, err := c.post(ctx, endpoint, body)
		lastStatus = status
		if err == nil && status >= 200 && status < 300 {
			return deliverResult{StatusCode: status, Attempts: attempt}
		}
		if err != nil {
			lastErr = err
		} else {
			lastErr = fmt.Errorf("HTTP %d", status)
			if status >= 400 && status < 500 {
				// Definitive rejection (e.g. a client refusing a handshake): retrying
				// won't change the answer.
				return deliverResult{StatusCode: status, Attempts: attempt, Err: lastErr}
			}
		}
		if attempt < maxAttempts {
			delay := baseDelay << (attempt - 1)
			slog.DebugContext(ctx, "Notification delivery failed, backing off",
				slog.String("endpoint", endpoint),
				slog.Int("attempt", attempt),
				slog.Duration("delay", delay),
				logging.Error(lastErr))
			select {
			case <-time.After(delay):
			case <-ctx.Done():
				return deliverResult{StatusCode: lastStatus, Attempts: attempt, Err: ctx.Err()}
			}
		}
	}
	return deliverResult{StatusCode: lastStatus, Attempts: maxAttempts, Err: lastErr}
}

func (c *Component) post(ctx context.Context, endpoint string, body []byte) (int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", fhirsubscription.FHIRJSONContentType)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	return resp.StatusCode, nil
}
