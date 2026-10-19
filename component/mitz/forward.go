package mitz

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

// notifyBodyLimit bounds a notification, forwarded or not.
const notifyBodyLimit = 1 << 20

// forwardTimeout bounds the relay to the source system. A variable so a test
// can shorten it; nothing else reassigns it.
var forwardTimeout = 5 * time.Second

type notifyBodyKey struct{}

type notifyBody struct {
	data        []byte
	contentType string
}

// captureNotifyBody keeps the notification exactly as it arrived, before the
// generated binding reads it as JSON. Both paths out of that binding forward
// from here, so a body it cannot decode is relayed as faithfully as one it can.
func captureNotifyBody(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, notifyBodyLimit))
		var tooLarge *http.MaxBytesError
		switch {
		case errors.As(err, &tooLarge):
			http.Error(w, "notification exceeds 1 MiB", http.StatusRequestEntityTooLarge)
			return
		case err != nil:
			http.Error(w, "notification body could not be read", http.StatusBadRequest)
			return
		}
		r.Body = io.NopCloser(bytes.NewReader(data))
		captured := notifyBody{data: data, contentType: r.Header.Get("Content-Type")}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), notifyBodyKey{}, captured)))
	})
}

// forwardNotification relays the captured notification to the configured URL,
// unchanged. Nil means the target confirmed it with a 2xx. An error means
// delivery was not confirmed; after a timeout the target may still have
// processed it. Nothing here retries: MITZ redelivers a notification it got no
// acknowledgement for (PvE Aansluiting Uitwisselingssystemen 3.8.0,
// AUS-TR-e0150).
func (c *Component) forwardNotification(ctx context.Context) error {
	body, ok := ctx.Value(notifyBodyKey{}).(notifyBody)
	if !ok {
		return errors.New("the notification body was not captured")
	}
	ctx, cancel := context.WithTimeout(ctx, forwardTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.notifyForwardURL, bytes.NewReader(body.data))
	if err != nil {
		return err
	}
	if body.contentType != "" {
		req.Header.Set("Content-Type", body.contentType)
	}
	res, err := c.forwardClient.Do(req)
	if err != nil {
		return fmt.Errorf("forward notification: %w", err)
	}
	defer res.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(res.Body, 64<<10))
	if res.StatusCode < 200 || res.StatusCode > 299 {
		return fmt.Errorf("forward target answered %s", res.Status)
	}
	return nil
}
