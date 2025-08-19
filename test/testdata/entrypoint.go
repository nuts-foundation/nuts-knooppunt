package testdata

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
	PublicFHIRBaseURL *url.URL
	CacheFHIRBaseURL  *url.URL
}

type Details struct {
	Knooppunt KnooppuntSystemDetails
	LRZa      LRZaSystemDetails
}

func Load(hapiBaseURL *url.URL) (*Details, error) {
	ctx := context.Background()
	knptMCSDPublicHAPITenant := HAPITenant{
		Name: "knpt-mcsd-public",
		ID:   1,
	}
	knptMCSDCacheHAPITenant := HAPITenant{
		Name: "knpt-mcsd-cache",
		ID:   2,
	}
	lrzaMCSDPublicHAPITenant := HAPITenant{
		Name: "lrza-mcsd-public",
		ID:   3,
	}

	hapiDefaultFHIRClient := fhirclient.New(hapiBaseURL, http.DefaultClient, nil)

	for _, tenant := range []HAPITenant{knptMCSDCacheHAPITenant, knptMCSDPublicHAPITenant, lrzaMCSDPublicHAPITenant} {
		if err := CreateHAPITenant(ctx, tenant, hapiDefaultFHIRClient); err != nil {
			return nil, fmt.Errorf("create hapi tenant: %w", err)
		}
	}

	lrzaMCSDPublicFHIRClient := fhirclient.New(lrzaMCSDPublicHAPITenant.BaseURL(hapiBaseURL), http.DefaultClient, nil)
	for _, org := range []fhir.Organization{Care2CureHospital(), CareHomeSunflower()} {
		if err := lrzaMCSDPublicFHIRClient.CreateWithContext(ctx, org, &org); err != nil {
			return nil, fmt.Errorf("create organization %s: %w", *org.Name, err)
		}
	}
	return &Details{
		Knooppunt: KnooppuntSystemDetails{
			MCSD: KnooppuntMCSDDetails{
				PublicFHIRBaseURL: knptMCSDPublicHAPITenant.BaseURL(hapiBaseURL),
				CacheFHIRBaseURL:  knptMCSDCacheHAPITenant.BaseURL(hapiBaseURL),
			},
		},
		LRZa: LRZaSystemDetails{
			FHIRBaseURL: lrzaMCSDPublicHAPITenant.BaseURL(hapiBaseURL),
		},
	}, nil
}
