package mcsd

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	fhirclient "github.com/SanteonNL/go-fhir-client"
	fhirUtil "github.com/nuts-foundation/nuts-knooppunt/lib/fhir"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/caramel/to"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

// resourceIDResolver is a function type for resolving FHIR references from remote FHIR APIs to local resource IDs.
// It is required because the local copies of external resources have different IDs from the remote resources.
type resourceIDResolver interface {
	resolve(ctx context.Context, remoteFHIRResourceReference string) (*string, error)
}

// chainedResourceIDResolver tries multiple resolvers in order until one succeeds.
type chainedResourceIDResolver []resourceIDResolver

func (c chainedResourceIDResolver) resolve(ctx context.Context, remoteFHIRResourceReference string) (*string, error) {
	for _, resolver := range c {
		localID, err := resolver.resolve(ctx, remoteFHIRResourceReference)
		if err != nil {
			return nil, err
		}
		if localID != nil {
			return localID, nil
		}
	}
	return nil, nil
}

var _ resourceIDResolver = (*mapResourceIDResolver)(nil)

// mapResourceIDResolver resolves references using a provided map of remote references to local resource IDs.
// This is useful for transaction-bound references (e.g. temporary UUIDs in a FHIR transaction bundle).
type mapResourceIDResolver map[string]string

func (t mapResourceIDResolver) resolve(_ context.Context, remoteFHIRResourceReference string) (*string, error) {
	if localID, ok := t[remoteFHIRResourceReference]; ok {
		return &localID, nil
	}
	return nil, nil
}

var _ resourceIDResolver = (*metaSourceResourceIDResolver)(nil)

// metaSourceResourceIDResolver resolves references by querying the local FHIR server for resources with a specific meta.source value.
type metaSourceResourceIDResolver struct {
	sourceFHIRBaseURL *url.URL
	localFHIRClient   fhirclient.Client
}

func (m metaSourceResourceIDResolver) resolve(ctx context.Context, remoteFHIRResourceReference string) (*string, error) {
	resourceType := strings.Split(remoteFHIRResourceReference, "/")[0]

	searchParams := url.Values{
		"_source": []string{m.sourceFHIRBaseURL.JoinPath(remoteFHIRResourceReference).String()},
		"_count":  []string{"2"},
	}
	var searchSet fhir.Bundle
	if err := m.localFHIRClient.SearchWithContext(ctx, resourceType, searchParams, &searchSet); err != nil {
		return nil, fmt.Errorf("resource id resolution: failed to search for resource %s: %w", remoteFHIRResourceReference, err)
	}
	if len(searchSet.Entry) == 0 {
		return nil, nil
	}
	if len(searchSet.Entry) > 1 || to.Value(searchSet.Total) > 1 {
		return nil, fmt.Errorf("resource id resolution: multiple resources found for %s", remoteFHIRResourceReference)
	}

	resourceInfo, err := fhirUtil.ExtractResourceInfo(searchSet.Entry[0].Resource)
	if err != nil {
		return nil, fmt.Errorf("resource id resolution: failed to extract resource info for %s: %w", remoteFHIRResourceReference, err)
	}
	return &resourceInfo.ID, nil
}
