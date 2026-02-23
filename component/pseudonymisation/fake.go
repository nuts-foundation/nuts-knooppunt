package pseudonymisation

import (
	"context"
	"fmt"

	"github.com/nuts-foundation/nuts-knooppunt/lib/bsnutil"
	"github.com/nuts-foundation/nuts-knooppunt/lib/coding"
	"github.com/nuts-foundation/nuts-knooppunt/lib/to"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

var _ Pseudonymizer = (*FakePseudonymizer)(nil)

type FakePseudonymizer struct {
}

func (f FakePseudonymizer) IdentifierToToken(ctx context.Context, identifier fhir.Identifier, localOrganizationURA string, recipientURA string) (*fhir.Identifier, error) {
	if identifier.System == nil || *identifier.System != coding.BSNNamingSystem || identifier.Value == nil {
		return &identifier, nil
	}
	token, err := bsnutil.CreateTransportToken(*identifier.Value, recipientURA)
	if err != nil {
		return nil, fmt.Errorf("getting BSN transport token: %v", err)
	}
	return &fhir.Identifier{
		System: to.Ptr(coding.BSNTransportTokenNamingSystem),
		Value:  to.Ptr(token),
	}, nil
}
