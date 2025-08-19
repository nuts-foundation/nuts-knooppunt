package testdata

import (
	"github.com/nuts-foundation/nuts-knooppunt/lib/to"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

func CareHomeSunflower() fhir.Organization {
	return fhir.Organization{
		Id:   to.Ptr("e5909595-767e-41c1-9b00-a23ddf33e5d1"),
		Name: to.Ptr("Sunflower Care Home"),
	}
}
