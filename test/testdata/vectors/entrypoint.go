package vectors

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	fhirclient "github.com/SanteonNL/go-fhir-client"
	"github.com/nuts-foundation/nuts-knooppunt/test/testdata/vectors/care2cure"
	"github.com/nuts-foundation/nuts-knooppunt/test/testdata/vectors/lrza"
	"github.com/nuts-foundation/nuts-knooppunt/test/testdata/vectors/sunflower"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/caramel"
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
	sunflowerAdminHAPITenant := HAPITenant{
		Name: "sunflower-admin",
		ID:   5,
	}

	hapiDefaultFHIRClient := fhirclient.New(hapiBaseURL, http.DefaultClient, nil)

	for _, tenant := range []HAPITenant{knptMCSDQueryHAPITenant, knptMCSDAdminHAPITenant, lrzaMCSDAdminHAPITenant, care2CureAdminHAPITenant, sunflowerAdminHAPITenant} {
		if err := CreateHAPITenant(ctx, tenant, hapiDefaultFHIRClient); err != nil {
			return nil, fmt.Errorf("create HAPI tenant: %w", err)
		}
	}

	//
	// Knooppunt mCSD Admin
	//
	lrzaMCSDAdminFHIRClient := lrzaMCSDAdminHAPITenant.FHIRClient(hapiBaseURL)
	for _, resource := range lrza.Resources(hapiBaseURL) {
		if err := lrzaMCSDAdminFHIRClient.UpdateWithContext(ctx, caramel.ResourceType(resource)+"/"+*resource.GetId(), resource, nil); err != nil {
			return nil, fmt.Errorf("create root resource: %w", err)
		}
	}

	//
	// Care2Cure Hospital
	//
	care2CureMCSDAdminFHIRClient := care2CureAdminHAPITenant.FHIRClient(hapiBaseURL)
	for _, resource := range care2cure.Resources() {
		if err := care2CureMCSDAdminFHIRClient.UpdateWithContext(ctx, caramel.ResourceType(resource)+"/"+*resource.GetId(), resource, nil); err != nil {
			return nil, fmt.Errorf("create care2cure resource: %w", err)
		}
	}

	//
	// Sunflower Care Home
	//
	sunflowerMCSDAdminFHIRClient := sunflowerAdminHAPITenant.FHIRClient(hapiBaseURL)
	for _, resource := range sunflower.Resources() {
		if err := sunflowerMCSDAdminFHIRClient.UpdateWithContext(ctx, caramel.ResourceType(resource)+"/"+*resource.GetId(), resource, nil); err != nil {
			return nil, fmt.Errorf("create sunflower resource: %w", err)
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
