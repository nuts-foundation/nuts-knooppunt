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
					Reference: to.Ptr("Endpoint/1111"),
				},
				{
					Reference: to.Ptr("Endpoint/2222"),
				},
			},
		},
		{
			Name: to.Ptr("Example Organisation Two"),
			Endpoint: []fhir.Reference{
				{
					Reference: to.Ptr("Endpoint/1111"),
				},
			},
		},
		{
			Name: to.Ptr("Example Organisation Three"),
			Endpoint: []fhir.Reference{
				{
					Reference: to.Ptr("Endpoint/3333"),
				},
			},
		},
		{
			Name: to.Ptr("Example Organisation Three"),
			Endpoint: []fhir.Reference{
				{
					Reference: to.Ptr("Endpoint/4444"),
				},
				{
					Reference: to.Ptr("Endpoint/1111"),
				},
			},
		},
	}

	eps := []fhir.Endpoint{
		{
			Id: to.Ptr("1111"),
		},
		{
			Id: to.Ptr("2222"),
		},
		{
			Id: to.Ptr("3333"),
		},
	}

	props := MakeEpConnectProps(orgs, eps)
	rows := props.Rows
	assert.True(t, rows[0].Cells[1].Enabled)
	assert.False(t, rows[0].Cells[2].Enabled)
	assert.True(t, rows[1].Cells[0].Enabled)
	assert.False(t, rows[1].Cells[1].Enabled)
	assert.False(t, rows[2].Cells[0].Enabled)
	assert.True(t, rows[2].Cells[2].Enabled)
	assert.True(t, rows[3].Cells[0].Enabled)
	assert.False(t, rows[3].Cells[1].Enabled)
}
