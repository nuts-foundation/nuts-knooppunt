package client

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	feed "github.com/nuts-foundation/nuts-knooppunt/addressing/feed/client"
	"github.com/nuts-foundation/nuts-knooppunt/addressing/update/config"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

type DirectorySyncState struct {
	Directory config.Directory
	LastSync  *time.Time `json:"last_sync"` // The last time this directory was synced
}

// OrgDirectorySyncState is a map that holds the sync state for each organization
type OrgDirectorySyncState map[string]DirectorySyncState

type Client struct {
	// HTTP client to make requests to the update service.
	httpClient *http.Client
	SyncState  OrgDirectorySyncState // Holds the sync state for each directory
	Config     *config.Config        // Configuration for the client, including directories and identifiers
}

func NewClient(cfg *config.Config, httpClient *http.Client) *Client {
	return &Client{
		Config:     cfg,
		httpClient: httpClient,
		SyncState:  make(OrgDirectorySyncState),
	}
}

func (c *Client) SyncMasterDirectory(ctx context.Context) error {
	// This function is responsible for syncing the master directory.
	// It will fetch the latest updates from the master directory and update the local directory accordingly.
	masterSyncState, exists := c.SyncState[c.Config.MasterDirectory.Url.String()]
	if !exists {
		// If the master directory is not in the sync state, initialize it
		masterSyncState = DirectorySyncState{
			Directory: c.Config.MasterDirectory,
			LastSync:  nil, // No last sync time yet
		}
		c.SyncState[c.Config.MasterDirectory.Url.String()] = masterSyncState
	}
	updates, err := c.RequestUpdates(ctx, c.Config.MasterDirectory, c.SyncState[c.Config.MasterDirectory.Url.String()].LastSync)
	if err != nil {
		return fmt.Errorf("failed to request updates from master directory: %w", err)
	}

	consolidator := NewConsolidator()
	feedBundle, err := consolidator.FeedFromUpdateBundle(updates)
	if err != nil {
		return fmt.Errorf("failed to extract entries from update bundle: %w", err)
	}
	prettyEntries, err := json.MarshalIndent(feedBundle, "", "  ")
	fmt.Printf("Consolidated entries: %+v\n", string(prettyEntries))

	feedClient := feed.NewFeedClient(c.httpClient)

	err = feedClient.ProcessRequest(ctx, c.Config.LocalDirectory, feedBundle)
	if err != nil {
		return fmt.Errorf("failed to process updates: %w", err)
	}

	return nil
}

func (c *Client) RequestUpdates(ctx context.Context, directory config.Directory, since *time.Time) (*fhir.Bundle, error) {
	// if no time is given, we do a full history sync
	url := directory.Url.String() + "_history"
	if since != nil {
		url += "?_since=" + since.Format(time.RFC3339)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
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

	bundle := &fhir.Bundle{}
	if err := json.Unmarshal(rawJson, &bundle); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response body: %w", err)
	}

	fmt.Println("Received updates:", bundle)

	return bundle, nil
}
