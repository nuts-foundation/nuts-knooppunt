package lrza

import (
	"context"
	"fmt"
	"net/http"
	"time"

	fhirclient "github.com/SanteonNL/go-fhir-client"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

// The synced-up-to time is the LRZA time up to which all LRZA changes are in the query directory: the
// _since value for the next sync. It's stored in the query directory as a Basic resource, so it survives
// a restart, and wiping the query directory also removes it (so the next sync is a full one).
const (
	syncStateCodeSystem   = "http://nuts-foundation.github.io/nuts-knooppunt/CodeSystem/lrza"
	syncStateCode         = "sync-state"
	syncStateExtensionURL = "http://nuts-foundation.github.io/nuts-knooppunt/StructureDefinition/lrza-synced-up-to"
)

// syncStateID is the id of the Basic resource, derived from the LRZA base URL (see localID).
func (c *Component) syncStateID() (string, error) {
	_, id, err := localID(c.config.LRZABaseUrl, "Basic", syncStateCode)
	return id, err
}

// loadSyncedUpTo reads the synced-up-to time. It's the zero time (a full sync) when none is saved. An
// unusable saved time is an error, not a full sync: a full sync misses deletions, then saves a new time,
// so deletions since the saved time would be lost silently.
func (c *Component) loadSyncedUpTo(ctx context.Context) (time.Time, error) {
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
		return time.Time{}, fmt.Errorf("failed to read LRZA synced-up-to time from query directory: %w", err)
	}
	for _, ext := range basic.Extension {
		if ext.Url == syncStateExtensionURL && ext.ValueInstant != nil {
			if syncedUpTo, err := time.Parse(time.RFC3339Nano, *ext.ValueInstant); err == nil {
				return syncedUpTo, nil
			}
		}
	}
	return time.Time{}, fmt.Errorf("invalid LRZA synced-up-to time in query directory (Basic/%s)", id)
}

// saveSyncedUpTo writes the synced-up-to time to the query directory.
func (c *Component) saveSyncedUpTo(ctx context.Context, syncedUpTo time.Time) error {
	id, err := c.syncStateID()
	if err != nil {
		return err
	}
	basic := fhir.Basic{
		Id: &id,
		Code: fhir.CodeableConcept{
			Coding: []fhir.Coding{{System: new(syncStateCodeSystem), Code: new(syncStateCode)}},
		},
		Extension: []fhir.Extension{{Url: syncStateExtensionURL, ValueInstant: new(syncedUpTo.Format(time.RFC3339Nano))}},
	}
	if err := c.fhirQueryClient.UpdateWithContext(ctx, "Basic/"+id, basic, nil); err != nil {
		return fmt.Errorf("failed to write LRZA synced-up-to time to query directory: %w", err)
	}
	return nil
}
