package nvi

import (
	"github.com/nuts-foundation/nuts-knooppunt/test/testdata/vectors/hapi"
)

func HAPITenant() hapi.Tenant {
	return hapi.Tenant{
		Name: "nvi",
		ID:   6,
	}
}
