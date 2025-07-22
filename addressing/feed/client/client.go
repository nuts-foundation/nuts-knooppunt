package feed

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/nuts-foundation/nuts-knooppunt/addressing/update/config"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

type FeedClient struct {
	httpClient *http.Client // HTTP client to make requests
}

func NewFeedClient(httpClient *http.Client) *FeedClient {
	return &FeedClient{httpClient: httpClient}
}

type Bundle = map[string]any

func (c *FeedClient) ProcessRequest(ctx context.Context, localDirectory config.Directory, transactionBundle *fhir.Bundle) error {
	// This function is responsible for sending a batch update to the server.
	// It will send the batch data to the server and handle the response.
	url := localDirectory.Url.String()
	body, err := json.Marshal(transactionBundle)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBuffer(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/fhir+json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to update: %s", resp.Status)
	}

	return nil
}
