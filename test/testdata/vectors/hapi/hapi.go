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
	// Operator marks a tenant whose writes are checked by a mock interceptor (the mock LRZA,
	// mock-components/lrza): its FHIRClient acts as the operator, who skips those checks.
	Operator bool
}

func (h Tenant) BaseURL(hapiServerURL *url.URL) *url.URL {
	return hapiServerURL.JoinPath(h.Name)
}

func (h Tenant) FHIRClient(hapiServerURL *url.URL) fhirclient.Client {
	httpClient := http.DefaultClient
	if h.Operator {
		httpClient = OperatorHTTPClient()
	}
	return fhirclient.New(h.BaseURL(hapiServerURL), httpClient, nil)
}

// OperatorHeader marks a request to a mock HAPI partition as the operator's: seeding the directory, or
// a test setting it up or inspecting it, rather than a client the mock's rules apply to.
const OperatorHeader = "X-Mock-Operator"

// OperatorHTTPClient is an HTTP client that sends every request as the operator.
func OperatorHTTPClient() *http.Client {
	return &http.Client{Transport: operatorTransport{next: http.DefaultTransport}}
}

type operatorTransport struct {
	next http.RoundTripper
}

func (t operatorTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context())
	req.Header.Set(OperatorHeader, "true")
	return t.next.RoundTrip(req)
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
