//go:generate mockgen -destination=component_mock.go -package=pseudonimization -source=component.go
package pseudonimization

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"path"

	"github.com/nuts-foundation/nuts-knooppunt/lib/coding"
	"github.com/nuts-foundation/nuts-knooppunt/lib/from"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/caramel/to"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

type Pseudonymizer interface {
	IdentifierToToken(ctx context.Context, identifier fhir.Identifier, audience string) (*fhir.Identifier, error)
	TokenToBSN(identifier fhir.Identifier, audience string) (*fhir.Identifier, error)
}

func New(httpClient *http.Client, prsURL string) *Component {
	return &Component{
		httpClient: httpClient,
		prsURL:     prsURL,
	}
}

type Component struct {
	httpClient *http.Client
	prsURL     string // Base URL for PRS service
}

// IdentifierToToken converts a BSN identifier to a pseudonymous transport token using the PRS service.
// The process follows RFC 9497 OPRF protocol:
// 1. Create prsIdentifier from BSN
// 2. Derive key using HKDF
// 3. Blind the input using OPRF client
// 4. Send blinded input to PRS for evaluation
// 5. PRS returns the final pseudonymized identifier (deblinding happens at the consuming system/NVI)
func (c Component) IdentifierToToken(ctx context.Context, identifier fhir.Identifier, recipientURA string) (*fhir.Identifier, error) {
	if identifier.System == nil || *identifier.System != coding.BSNNamingSystem || identifier.Value == nil {
		return &identifier, nil
	}

	// Step 1: Create prsIdentifier
	prsID := prsIdentifier{
		LandCode: "NL",
		Type:     "BSN",
		Value:    *identifier.Value,
	}

	// Step 2 & 3: Blind the identifier (internally derives key and blinds)
	scope := "nationale-verwijsindex"
	blindedInputData, err := blindIdentifier(prsID, recipientURA, scope)
	if err != nil {
		return nil, fmt.Errorf("blinding identifier: %w", err)
	}

	// Step 4: Call PRS to get the pseudonymized identifier
	// PRS returns the final pseudonymized BSN (deblinding happens at the consuming system/NVI)
	pseudonymizedBSN, err := c.callPRSEvaluate(ctx, recipientURA, scope, blindedInputData)
	if err != nil {
		return nil, err
	}

	return &fhir.Identifier{
		System: to.Ptr(coding.BSNTransportTokenNamingSystem),
		Value:  &pseudonymizedBSN,
	}, nil
}

// TokenToBSN is not supported in the PRS implementation.
// PRS pseudonyms are one-way using OPRF and cannot be reversed to the original BSN.
// This function returns an error if a transport token is provided.
func (c Component) TokenToBSN(identifier fhir.Identifier, _ string) (*fhir.Identifier, error) {
	if identifier.System == nil || *identifier.System != coding.BSNTransportTokenNamingSystem || identifier.Value == nil {
		return &identifier, nil
	}
	return nil, fmt.Errorf("TokenToBSN is not supported: PRS pseudonyms cannot be reversed to BSN")
}

type prsIdentifier struct {
	LandCode string `json:"landCode"`
	Type     string `json:"type"`
	Value    string `json:"value"`
}

// PRS API request/response structures
type prsEvaluateRequest struct {
	RecipientOrganization string `json:"recipientOrganization"`
	RecipientScope        string `json:"recipientScope"`
	EncryptedPersonalID   []byte `json:"encryptedPersonalId"`
}

type prsEvaluateResponse struct {
	PseudonymizedIdentifier string `json:"pseudonymized_identifier"` // The final pseudonymized BSN
}

// callPRSEvaluate sends the blinded input to the PRS service and returns the pseudonymized identifier
func (c Component) callPRSEvaluate(ctx context.Context, recipientURA string, scope string, blindedInputData []byte) (string, error) {
	requestBody := prsEvaluateRequest{
		RecipientOrganization: "ura:" + recipientURA,
		RecipientScope:        scope,
		EncryptedPersonalID:   blindedInputData,
	}

	bodyBytes, err := json.Marshal(requestBody)
	if err != nil {
		return "", err
	}

	// Call PRS evaluate endpoint
	requestURL, err := url.Parse(c.prsURL)
	if err != nil {
		return "", err
	}
	requestURL.Path = path.Join(requestURL.Path, "oprf/eval")
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, requestURL.String(), bytes.NewReader(bodyBytes))
	if err != nil {
		return "", err
	}
	httpResponse, err := c.httpClient.Do(httpRequest)
	if err != nil {
		return "", fmt.Errorf("PRS request: %w", err)
	}
	defer httpResponse.Body.Close()

	response, err := from.JSONResponse[prsEvaluateResponse](httpResponse)
	if err != nil {
		return "", fmt.Errorf("PRS response: %w", err)
	}
	return response.PseudonymizedIdentifier, nil
}
