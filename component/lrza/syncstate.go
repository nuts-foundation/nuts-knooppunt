package lrza

import (
	"context"
	"fmt"
	"net/http"
	"time"

	fhirclient "github.com/SanteonNL/go-fhir-client"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

// The last change sync time is the LRZA's time at the start of the last sync that applied changes: all
// LRZA changes before it are in the query directory, so the next sync reads the changes since then. It's
// stored in the query directory itself, as a Basic resource, so it survives a restart and stays in step
// with the data it describes: if the query directory is wiped, the time goes with it and the next sync is
// a full one.
const (
	syncStateCodeSystem   = "http://nuts-foundation.github.io/nuts-knooppunt/CodeSystem/lrza"
	syncStateCode         = "sync-state"
	syncStateExtensionURL = "http://nuts-foundation.github.io/nuts-knooppunt/StructureDefinition/lrza-last-change-sync-time"
)

// syncStateID is the id of the Basic resource holding the last change sync time. It's derived from the
// LRZA base URL like the ids of synced resources (see localID), so it's unique per source.
func (c *Component) syncStateID() (string, error) {
	_, id, err := localID(c.config.LRZABaseUrl, "Basic", syncStateCode)
	return id, err
}

// loadLastChangeSyncTime reads the last change sync time from the query directory: the _since value
// for the next incremental sync. It's the zero time, meaning a full sync, only when none is saved (a new or wiped
// query directory). A saved one that can't be used is an error rather than a full sync: a full sync
// doesn't see deletions and saves a new time, so deletions made since the saved time would be lost
// without anyone noticing. Deleting the Basic resource forces a full sync.
func (c *Component) loadLastChangeSyncTime(ctx context.Context) (time.Time, error) {
	id, err := c.syncStateID()
	if err != nil {
		return time.Time{}, err
	}
	var basic fhir.Basic
	var status int
	err = c.fhirQueryClient.ReadWithContext(ctx, "Basic/"+id, &basic, fhirclient.ResponseStatusCode(&status))
	if status == http.StatusNotFound || status == http.StatusGone {
		return time.Time{}, nil
	}
	if err != nil {
		return time.Time{}, fmt.Errorf("failed to read LRZA last change sync time from query directory: %w", err)
	}
	for _, ext := range basic.Extension {
		if ext.Url == syncStateExtensionURL && ext.ValueInstant != nil {
			syncTime, err := time.Parse(time.RFC3339Nano, *ext.ValueInstant)
			if err != nil {
				return time.Time{}, fmt.Errorf("invalid LRZA last change sync time in query directory (Basic/%s): %w", id, err)
			}
			return syncTime, nil
		}
	}
	return time.Time{}, fmt.Errorf("LRZA sync state in query directory has no last change sync time (Basic/%s)", id)
}

// saveLastChangeSyncTime writes the last change sync time to the query directory.
func (c *Component) saveLastChangeSyncTime(ctx context.Context, syncTime time.Time) error {
	id, err := c.syncStateID()
	if err != nil {
		return err
	}
	basic := fhir.Basic{
		Id: &id,
		Code: fhir.CodeableConcept{
			Coding: []fhir.Coding{{System: new(syncStateCodeSystem), Code: new(syncStateCode)}},
		},
		Extension: []fhir.Extension{{Url: syncStateExtensionURL, ValueInstant: new(syncTime.Format(time.RFC3339Nano))}},
	}
	if err := c.fhirQueryClient.UpdateWithContext(ctx, "Basic/"+id, basic, nil); err != nil {
		return fmt.Errorf("failed to write LRZA last change sync time to query directory: %w", err)
	}
	return nil
}
