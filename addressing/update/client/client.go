package client

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/nuts-foundation/nuts-knooppunt/addressing/update/config"
)

type Client struct {
	// HTTP client to make requests to the update service.
	httpClient *http.Client
}

type Updates struct {
	// Directory is the directory for which updates are requested.
}

func NewClient(http.Client) *Client {
	return &Client{httpClient: &http.Client{}}
}

func (c *Client) RequestUpdates(ctx context.Context, directory config.Directory, since time.Time) (*Updates, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, directory.Url.String()+"_history?_since="+since.Format(time.RFC3339), nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to fetch updates: %s", resp.Status)
	}

	rawJson, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	bundle := map[string]interface{}{}
	if err := json.Unmarshal(rawJson, &bundle); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response body: %w", err)
	}

	fmt.Println("Received updates:", bundle)

	// Perform http request to the
	return &Updates{}, nil
}
