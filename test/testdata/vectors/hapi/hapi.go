package hapi

import (
	"context"
	"net/http"
	"net/url"
	"strings"

	fhirclient "github.com/SanteonNL/go-fhir-client"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

type Tenant struct {
	Name string
	ID   int
}

func (h Tenant) BaseURL(hapiServerURL *url.URL) *url.URL {
	return hapiServerURL.JoinPath(h.Name)
}

func (h Tenant) FHIRClient(hapiServerURL *url.URL) fhirclient.Client {
	return fhirclient.New(h.BaseURL(hapiServerURL), http.DefaultClient, nil)
}

func CreateTenant(ctx context.Context, details Tenant, fhirClient fhirclient.Client) error {
	var tenant fhir.Parameters
	tenant.Parameter = []fhir.ParametersParameter{
		{
			Name:         "id",
			ValueInteger: &details.ID,
		},
		{
			Name:        "name",
			ValueString: &details.Name,
		},
	}
	err := fhirClient.CreateWithContext(ctx, tenant, &tenant, fhirclient.AtPath("/$partition-management-create-partition"))
	if err != nil && shouldIgnorePartitionError(err.Error()) {
		// assume it's OK (partition already exists)
		return nil
	}
	return err
}

// shouldIgnorePartitionError determines if a partition creation error should be ignored
// because it indicates the partition already exists
func shouldIgnorePartitionError(errorStr string) bool {
	// Handle various error formats that indicate the partition already exists
	return strings.Contains(errorStr, "HAPI-1309") || strings.Contains(errorStr, "HAPI-0389")
}
