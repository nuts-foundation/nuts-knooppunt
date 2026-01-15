package vectors

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	fhirclient "github.com/SanteonNL/go-fhir-client"
	"github.com/nuts-foundation/nuts-knooppunt/test/testdata/vectors/care2cure"
	"github.com/nuts-foundation/nuts-knooppunt/test/testdata/vectors/hapi"
	"github.com/nuts-foundation/nuts-knooppunt/test/testdata/vectors/lrza"
	"github.com/nuts-foundation/nuts-knooppunt/test/testdata/vectors/nvi"
	"github.com/nuts-foundation/nuts-knooppunt/test/testdata/vectors/pip"
	"github.com/nuts-foundation/nuts-knooppunt/test/testdata/vectors/sunflower"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/caramel"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/caramel/to"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

type KnooppuntSystemDetails struct {
	MCSD KnooppuntMCSDDetails
}

type FHIRAPIDetails struct {
	FHIRBaseURL *url.URL
}

type KnooppuntMCSDDetails struct {
	AdminFHIRBaseURL *url.URL
	QueryFHIRBaseURL *url.URL
}

type Details struct {
	Knooppunt KnooppuntSystemDetails
	LRZa      FHIRAPIDetails
	NVI       FHIRAPIDetails
	PIP       FHIRAPIDetails
}

func Load(hapiBaseURL *url.URL) (*Details, error) {
	ctx := context.Background()
	knptMCSDAdminHAPITenant := hapi.Tenant{
		Name: "knpt-mcsd-admin",
		ID:   1,
	}
	knptMCSDQueryHAPITenant := hapi.Tenant{
		Name: "knpt-mcsd-query",
		ID:   2,
	}
	lrzaMCSDAdminHAPITenant := lrza.HAPITenant()
	care2CureAdminHAPITenant := care2cure.AdminHAPITenant()
	sunflowerAdminHAPITenant := sunflower.AdminHAPITenant()
	sunflowerPatientHAPITenant := sunflower.PatientsHAPITenant()
	nviTenant := nvi.HAPITenant()
	pipTenant := pip.HAPITenant()

	hapiDefaultFHIRClient := fhirclient.New(hapiBaseURL, http.DefaultClient, nil)

	// Delete all data first
	_ = hapiDefaultFHIRClient.CreateWithContext(ctx, fhir.Parameters{
		Parameter: []fhir.ParametersParameter{
			{
				Name:         "expungeEverything",
				ValueBoolean: to.Ptr(true),
			},
		},
	}, nil, fhirclient.AtPath("/$expunge"))

	// Create tenants
	for _, tenant := range []hapi.Tenant{
		knptMCSDQueryHAPITenant,
		knptMCSDAdminHAPITenant,
		lrzaMCSDAdminHAPITenant,
		care2CureAdminHAPITenant,
		sunflowerAdminHAPITenant,
		sunflowerPatientHAPITenant,
		nviTenant,
		pipTenant,
	} {
		if err := hapi.CreateTenant(ctx, tenant, hapiDefaultFHIRClient); err != nil {
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
	for _, resource := range sunflower.AdminResources() {
		if err := sunflowerMCSDAdminFHIRClient.UpdateWithContext(ctx, caramel.ResourceType(resource)+"/"+*resource.GetId(), resource, nil); err != nil {
			return nil, fmt.Errorf("create sunflower admin resource: %w", err)
		}
	}
	sunflowerMCSDPatientFHIRClient := sunflowerPatientHAPITenant.FHIRClient(hapiBaseURL)
	for _, resource := range sunflower.PatientsResources() {
		if err := sunflowerMCSDPatientFHIRClient.UpdateWithContext(ctx, caramel.ResourceType(resource)+"/"+*resource.GetId(), resource, nil); err != nil {
			return nil, fmt.Errorf("create sunflower patients resource: %w", err)
		}
	}

	//
	// Policy information point
	//
	pipFHIRClient := pipTenant.FHIRClient(hapiBaseURL)
	for _, resource := range pip.Resources(hapiBaseURL) {
		if err := pipFHIRClient.UpdateWithContext(ctx, caramel.ResourceType(resource)+"/"+*resource.GetId(), resource, nil); err != nil {
			return nil, fmt.Errorf("create pip patients resource: %w", err)
		}
	}

	return &Details{
		Knooppunt: KnooppuntSystemDetails{
			MCSD: KnooppuntMCSDDetails{
				AdminFHIRBaseURL: knptMCSDAdminHAPITenant.BaseURL(hapiBaseURL),
				QueryFHIRBaseURL: knptMCSDQueryHAPITenant.BaseURL(hapiBaseURL),
			},
		},
		LRZa: FHIRAPIDetails{
			FHIRBaseURL: lrzaMCSDAdminHAPITenant.BaseURL(hapiBaseURL),
		},
		NVI: FHIRAPIDetails{
			FHIRBaseURL: nviTenant.BaseURL(hapiBaseURL),
		},
		PIP: FHIRAPIDetails{
			FHIRBaseURL: pipTenant.BaseURL(hapiBaseURL),
		},
	}, nil
}
