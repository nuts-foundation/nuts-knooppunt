package client

import (
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

func (c *Client) RequestUpdates(directory config.Directory, since time.Time) (*Updates, error) {
	// Perform http request to the
	return &Updates{}, nil
}
