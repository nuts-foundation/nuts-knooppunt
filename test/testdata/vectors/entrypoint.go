package vectors

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	fhirclient "github.com/SanteonNL/go-fhir-client"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

type KnooppuntSystemDetails struct {
	MCSD KnooppuntMCSDDetails
}

type LRZaSystemDetails struct {
	FHIRBaseURL *url.URL
}

type KnooppuntMCSDDetails struct {
	AdminFHIRBaseURL *url.URL
	QueryFHIRBaseURL *url.URL
}

type Details struct {
	Knooppunt KnooppuntSystemDetails
	LRZa      LRZaSystemDetails
}

func Load(hapiBaseURL *url.URL) (*Details, error) {
	ctx := context.Background()
	knptMCSDAdminHAPITenant := HAPITenant{
		Name: "knpt-mcsd-admin",
		ID:   1,
	}
	knptMCSDQueryHAPITenant := HAPITenant{
		Name: "knpt-mcsd-query",
		ID:   2,
	}
	lrzaMCSDAdminHAPITenant := HAPITenant{
		Name: "lrza-mcsd-admin",
		ID:   3,
	}
	care2CureAdminHAPITenant := HAPITenant{
		Name: "care2cure-admin",
		ID:   4,
	}

	hapiDefaultFHIRClient := fhirclient.New(hapiBaseURL, http.DefaultClient, nil)

	for _, tenant := range []HAPITenant{knptMCSDQueryHAPITenant, knptMCSDAdminHAPITenant, lrzaMCSDAdminHAPITenant, care2CureAdminHAPITenant} {
		if err := CreateHAPITenant(ctx, tenant, hapiDefaultFHIRClient); err != nil {
			return nil, fmt.Errorf("create hapi tenant: %w", err)
		}
	}

	lrzaMCSDAdminFHIRClient := lrzaMCSDAdminHAPITenant.FHIRClient(hapiBaseURL)
	// Create orgs
	for _, resource := range append([]fhir.Organization{Care2CureHospital()}, CareHomeSunflower()) {
		var response []byte
		if err := lrzaMCSDAdminFHIRClient.UpdateWithContext(ctx, "Organization/"+*resource.Id, resource, &response); err != nil {
			return nil, fmt.Errorf("create organization: %w", err)
		}
	}
	// Create root mCSD Directory endpoints
	for _, resource := range append(Care2CureHospitalRootEndpoints(), CareHomeSunflowerRootEndpoints()...) {
		var response []byte
		if err := lrzaMCSDAdminFHIRClient.UpdateWithContext(ctx, "Endpoint/"+*resource.Id, resource, &response); err != nil {
			return nil, fmt.Errorf("create endpoint: %w", err)
		}
	}
	// Create mCSD Admin Directory resources of Care2Cure Hospital
	care2CureMCSDAdminFHIRClient := care2CureAdminHAPITenant.FHIRClient(hapiBaseURL)
	for _, resource := range Care2CureHospitalAdminEndpoints() {
		var response []byte
		if err := care2CureMCSDAdminFHIRClient.UpdateWithContext(ctx, "Endpoint/"+*resource.Id, resource, &response); err != nil {
			return nil, fmt.Errorf("create endpoint: %w", err)
		}
	}
	return &Details{
		Knooppunt: KnooppuntSystemDetails{
			MCSD: KnooppuntMCSDDetails{
				AdminFHIRBaseURL: knptMCSDAdminHAPITenant.BaseURL(hapiBaseURL),
				QueryFHIRBaseURL: knptMCSDQueryHAPITenant.BaseURL(hapiBaseURL),
			},
		},
		LRZa: LRZaSystemDetails{
			FHIRBaseURL: lrzaMCSDAdminHAPITenant.BaseURL(hapiBaseURL),
		},
	}, nil
}
