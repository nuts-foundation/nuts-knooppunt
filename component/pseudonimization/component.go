//go:generate mockgen -destination=component_mock.go -package=pseudonimization -source=component.go
package pseudonimization

import (
	"fmt"
	"net/http"

	"github.com/nuts-foundation/nuts-knooppunt/lib/bsnutil"
	"github.com/nuts-foundation/nuts-knooppunt/lib/coding"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/caramel/to"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

type Pseudonymizer interface {
	IdentifierToToken(identifier fhir.Identifier, audience string) (*fhir.Identifier, error)
	TokenToBSN(identifier fhir.Identifier, audience string) (*fhir.Identifier, error)
}

func New(httpClient *http.Client) *Component {
	return &Component{
		httpClient: httpClient,
	}
}

type Component struct {
	httpClient *http.Client
}

func (c Component) IdentifierToToken(identifier fhir.Identifier, audience string) (*fhir.Identifier, error) {
	if identifier.System == nil || *identifier.System != coding.BSNNamingSystem || identifier.Value == nil {
		return &identifier, nil
	}
	token, err := bsnutil.CreateTransportToken(*identifier.Value, audience)
	if err != nil {
		return nil, fmt.Errorf("getting BSN transport token: %v", err)
	}
	return &fhir.Identifier{
		System: to.Ptr(coding.BSNTransportTokenNamingSystem),
		Value:  to.Ptr(token),
	}, nil
}

func (c Component) TokenToBSN(identifier fhir.Identifier, audience string) (*fhir.Identifier, error) {
	if identifier.System == nil || *identifier.System != coding.BSNTransportTokenNamingSystem || identifier.Value == nil {
		return &identifier, nil
	}
	bsn, err := bsnutil.BSNFromTransportToken(*identifier.Value)
	if err != nil {
		return nil, fmt.Errorf("getting BSN from transport token: %v", err)
	}
	return &fhir.Identifier{
		System: to.Ptr(coding.BSNNamingSystem),
		Value:  to.Ptr(bsn),
	}, nil
}
