package testdata

import (
	"github.com/nuts-foundation/nuts-knooppunt/lib/to"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

func Care2CureHospital() fhir.Organization {
	return fhir.Organization{
		Id:   to.Ptr("ef860868-b886-4459-aa87-216955c05289"),
		Name: to.Ptr("Care2Cure Hospital"),
	}
}
