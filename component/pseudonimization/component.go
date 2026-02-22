//go:generate mockgen -destination=component_mock.go -package=pseudonimization -source=component.go
package pseudonimization

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/nuts-foundation/nuts-knooppunt/lib/bsnutil"
	"github.com/nuts-foundation/nuts-knooppunt/lib/coding"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/caramel/to"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
	"golang.org/x/crypto/hkdf"
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

type prsIdentifier struct {
	LandCode string `json:"landCode"`
	Type     string `json:"type"`
	Value    string `json:"value"`
}

// CreatePseudonym derives a pseudonym from a personal identifier using HKDF-SHA256.
// This is the Go equivalent of:
//
//	info = f"{recipient_organization}|{recipient_scope}|v1".encode("utf-8")
//	hkdf = HKDF(algorithm=hashes.SHA256(), length=32, salt=None, info=info)
//	pid = json.dumps(personal_identifier)
//	pseudonym = hkdf.derive(pid.encode("utf-8"))
func (c Component) CreatePseudonym(personalIdentifier prsIdentifier, recipientOrganization, recipientScope string) ([]byte, error) {
	// JSON encode the personal identifier (this is the Input Key Material)
	pid, err := json.Marshal(personalIdentifier)
	if err != nil {
		return nil, fmt.Errorf("marshaling personal identifier: %w", err)
	}

	// Create the info string: "{recipient_organization}|{recipient_scope}|v1"
	info := fmt.Sprintf("%s|%s|v1", recipientOrganization, recipientScope)

	// Create HKDF reader with:
	// - hash: SHA256
	// - secret/IKM: the JSON-encoded personal identifier
	// - salt: nil (no salt)
	// - info: the recipient organization, scope, and version
	hkdfReader := hkdf.New(sha256.New, pid, nil, []byte(info))

	// Derive 32 bytes (256 bits) for the pseudonym
	pseudonym := make([]byte, 32)
	if _, err := io.ReadFull(hkdfReader, pseudonym); err != nil {
		return nil, fmt.Errorf("deriving pseudonym: %w", err)
	}

	return pseudonym, nil
}
