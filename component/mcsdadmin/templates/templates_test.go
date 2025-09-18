package templates

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/caramel/to"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

func TestTemplates_connect_props(t *testing.T) {
	orgs := []fhir.Organization{
		{
			Name: to.Ptr("Example Organisation One"),
			Endpoint: []fhir.Reference{
				{
					Id: to.Ptr("IdOne"),
				},
				{
					Id: to.Ptr("IdTwo"),
				},
			},
		},
		{
			Name: to.Ptr("Example Organisation Two"),
			Endpoint: []fhir.Reference{
				{
					Id: to.Ptr("IdOne"),
				},
			},
		},
		{
			Name: to.Ptr("Example Organisation Three"),
			Endpoint: []fhir.Reference{
				{
					Id: to.Ptr("IdThree"),
				},
			},
		},
		{
			Name: to.Ptr("Example Organisation Three"),
			Endpoint: []fhir.Reference{
				{
					Id: to.Ptr("IdFour"),
				},
				{
					Id: to.Ptr("IdOne"),
				},
			},
		},
	}

	props := MakeEpConnectProps(orgs)
	matrix := props.RowData
	assert.True(t, matrix[0][1])
	assert.False(t, matrix[0][2])
	assert.True(t, matrix[1][0])
	assert.False(t, matrix[1][1])
	assert.False(t, matrix[2][0])
	assert.True(t, matrix[2][2])
	assert.True(t, matrix[3][0])
	assert.False(t, matrix[3][1])
}
