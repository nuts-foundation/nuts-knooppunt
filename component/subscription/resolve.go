package subscription

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/url"

	fhirclient "github.com/SanteonNL/go-fhir-client"
	"github.com/nuts-foundation/nuts-knooppunt/lib/coding"
	"github.com/nuts-foundation/nuts-knooppunt/lib/fhirutil"
	"github.com/nuts-foundation/nuts-knooppunt/lib/logging"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/caramel/to"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

// Endpoint resolution via GF Adressing (research question 9): given a partner's URA, find
// the Endpoint of the right payload type in the mCSD Query Directory. There is no
// existing helper for this in the knooppunt — it is new code, built from an
// Organization-by-identifier search plus its Endpoint references, filtered on
// payloadType (the same linkage model the mCSD sync maintains). The payload type codes
// (tta-notification / tta-subscription) do not exist in any published code system yet:
// that GF Adressing has no defined vocabulary for notification endpoints is itself a
// finding. Per-partner configuration is the fallback.

// resolveNotificationEndpoint finds where to deliver notifications for the partner
// (Subscription Server role, out-of-band creation).
func (c *Component) resolveNotificationEndpoint(ctx context.Context, partnerURA string) (string, error) {
	if partner, ok := c.config.Server.Partners[partnerURA]; ok && partner.NotificationEndpoint != "" {
		return partner.NotificationEndpoint, nil
	}
	endpoint, err := c.resolveMCSDEndpoint(ctx, c.config.Server.QueryDirectoryFHIRBaseURL, partnerURA, coding.MCSDPayloadTypeTTANotification)
	if err != nil {
		return "", fmt.Errorf("no notification endpoint for URA %s: %w", partnerURA, err)
	}
	return endpoint, nil
}

// resolveSubscriptionBase finds the partner's public FHIR base where POST /Subscription,
// $status and $events live (Subscription Client role, in-band creation and recovery).
func (c *Component) resolveSubscriptionBase(ctx context.Context, partnerURA string) (string, error) {
	if partner, ok := c.config.Client.Partners[partnerURA]; ok && partner.FHIRBaseURL != "" {
		return partner.FHIRBaseURL, nil
	}
	endpoint, err := c.resolveMCSDEndpoint(ctx, c.config.Client.QueryDirectoryFHIRBaseURL, partnerURA, coding.MCSDPayloadTypeTTASubscription)
	if err != nil {
		return "", fmt.Errorf("no subscription endpoint for URA %s: %w", partnerURA, err)
	}
	return endpoint, nil
}

// resolveMCSDEndpoint searches the Query Directory for the partner Organization by URA
// identifier and walks its Endpoint references, returning the address of the first
// active Endpoint whose payloadType includes the given code.
func (c *Component) resolveMCSDEndpoint(ctx context.Context, queryDirectoryBase, partnerURA, payloadTypeCode string) (string, error) {
	if queryDirectoryBase == "" {
		return "", fmt.Errorf("no mCSD query directory configured and no partner configuration")
	}
	baseURL, err := url.Parse(queryDirectoryBase)
	if err != nil {
		return "", fmt.Errorf("invalid query directory URL: %w", err)
	}
	client := fhirclient.New(baseURL, c.httpClient, fhirutil.ClientConfig())

	var organizations fhir.Bundle
	params := url.Values{}
	params.Set("identifier", coding.URANamingSystem+"|"+partnerURA)
	if err := client.SearchWithContext(ctx, "Organization", params, &organizations); err != nil {
		return "", fmt.Errorf("organization search failed: %w", err)
	}

	wanted := fhir.Coding{System: to.Ptr(coding.MCSDPayloadTypeSystem), Code: to.Ptr(payloadTypeCode)}
	for _, entry := range organizations.Entry {
		var organization fhir.Organization
		if entry.Resource == nil || json.Unmarshal(entry.Resource, &organization) != nil {
			continue
		}
		for _, endpointRef := range organization.Endpoint {
			if endpointRef.Reference == nil {
				continue
			}
			var endpoint fhir.Endpoint
			if err := client.ReadWithContext(ctx, *endpointRef.Reference, &endpoint); err != nil {
				slog.DebugContext(ctx, "Failed to read mCSD Endpoint", slog.String("reference", *endpointRef.Reference), logging.Error(err))
				continue
			}
			if endpoint.Status == fhir.EndpointStatusActive && coding.CodablesIncludesCode(endpoint.PayloadType, wanted) {
				return endpoint.Address, nil
			}
		}
	}
	return "", fmt.Errorf("no active Endpoint with payloadType %s found in query directory", payloadTypeCode)
}
