package lrza

import (
	"context"
	"fmt"
	"net/http"

	fhirclient "github.com/SanteonNL/go-fhir-client"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

// The last sync time is stored in the query directory itself, as a Basic resource, so it survives a
// restart and stays in step with the data it describes: if the query directory is wiped, the time
// goes with it and the next sync is a full one.
const (
	syncStateCodeSystem   = "http://nuts-foundation.org/knooppunt/CodeSystem/lrza"
	syncStateCode         = "sync-state"
	syncStateExtensionURL = "http://nuts-foundation.org/knooppunt/StructureDefinition/lrza-last-sync-time"
)

// syncStateID is the id of the Basic resource holding the last sync time. It's derived from the
// LRZA base URL like the ids of synced resources (see localID), so it's unique per source.
func (c *Component) syncStateID() (string, error) {
	_, id, err := localID(c.config.LRZABaseUrl, "Basic", syncStateCode)
	return id, err
}

// loadSyncTime reads the last sync time from the query directory: the _since value for the next
// incremental sync. It's empty when none is saved (a new or wiped query directory), meaning a full
// sync.
func (c *Component) loadSyncTime(ctx context.Context) (string, error) {
	id, err := c.syncStateID()
	if err != nil {
		return "", err
	}
	var basic fhir.Basic
	var status int
	err = c.fhirQueryClient.ReadWithContext(ctx, "Basic/"+id, &basic, fhirclient.ResponseStatusCode(&status))
	if status == http.StatusNotFound || status == http.StatusGone {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("failed to read LRZA sync time from query directory: %w", err)
	}
	for _, ext := range basic.Extension {
		if ext.Url == syncStateExtensionURL && ext.ValueString != nil {
			return *ext.ValueString, nil
		}
	}
	return "", nil
}

// saveSyncTime writes the last sync time to the query directory. It's stored as a string, not an
// instant: it's passed back verbatim as _since, so it must not be normalized by the FHIR server.
func (c *Component) saveSyncTime(ctx context.Context, syncTime string) error {
	id, err := c.syncStateID()
	if err != nil {
		return err
	}
	basic := fhir.Basic{
		Id: &id,
		Meta: &fhir.Meta{
			Source: &c.config.LRZABaseUrl,
		},
		Code: fhir.CodeableConcept{
			Coding: []fhir.Coding{{System: new(syncStateCodeSystem), Code: new(syncStateCode)}},
		},
		Extension: []fhir.Extension{{Url: syncStateExtensionURL, ValueString: new(syncTime)}},
	}
	if err := c.fhirQueryClient.UpdateWithContext(ctx, "Basic/"+id, basic, nil); err != nil {
		return fmt.Errorf("failed to write LRZA sync time to query directory: %w", err)
	}
	return nil
}
