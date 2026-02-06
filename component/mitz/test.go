package mitz

import (
	"testing"

	"github.com/nuts-foundation/nuts-knooppunt/test/mitzmock"
	"github.com/stretchr/testify/require"
)

func NewTestInstance(t *testing.T) *Component {
	closedQuestionService := mitzmock.NewClosedQuestionService(t)
	component, err := New(Config{
		MitzBase:      closedQuestionService.GetURL(),
		GatewaySystem: "test-gateway",
		SourceSystem:  "test-source",
	})
	require.NoError(t, err)
	return component
}
