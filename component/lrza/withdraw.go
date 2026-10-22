package lrza

import (
	"fmt"
	"time"

	"github.com/nuts-foundation/nuts-knooppunt/lib/fhirapi"
)

// Withdrawal. The IG says resources SHOULD NOT be deleted from the LRZA, but withdrawn by a status
// change, so consumers syncing the directory see the resource go inactive rather than disappear. A
// DELETE through the proxy is therefore rewritten to an update of the current version.

// withdraw changes res, the current version of a resource, into its withdrawn version:
// Organization and HealthcareService get active=false, Location status=inactive, and Endpoint
// status=off with its period ending now.
func withdraw(res resource, now time.Time) error {
	switch res.resourceType() {
	case "Organization", "HealthcareService":
		res["active"] = false
	case "Location":
		res["status"] = "inactive"
	case "Endpoint":
		res["status"] = "off"
		period, _ := res["period"].(map[string]any)
		if period == nil {
			period = map[string]any{}
			res["period"] = period
		}
		period["end"] = now.UTC().Format(time.RFC3339)
	default:
		return fhirapi.BadRequestError(fmt.Sprintf("can't withdraw a %s", res.resourceType()), nil)
	}
	return nil
}
