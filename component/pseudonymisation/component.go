//go:generate mockgen -destination=component_mock.go -package=pseudonymisation -source=component.go
package pseudonymisation

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"path"

	"github.com/nuts-foundation/nuts-knooppunt/component/authn"
	"github.com/nuts-foundation/nuts-knooppunt/lib/coding"
	"github.com/nuts-foundation/nuts-knooppunt/lib/from"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/caramel/to"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

type Config struct {
	PRSBaseURL string `koanf:"prsurl"`
}

type Pseudonymizer interface {
	IdentifierToToken(ctx context.Context, identifier fhir.Identifier, localOrganizationURA string, recipientURA string, scope string) (*fhir.Identifier, error)
}

func New(cfg Config, httpClientFn authn.HTTPClientProvider) *Component {
	return &Component{
		httpClientFn: httpClientFn,
		config:       cfg,
	}
}

type Component struct {
	httpClientFn authn.HTTPClientProvider
	config       Config
}

// IdentifierToToken converts a BSN identifier to a pseudonymous transport token using the PRS service.
// The process follows RFC 9497 OPRF protocol:
// 1. Create PRS identifier from BSN
// 2. Derive key using HKDF
// 3. Blind the input using OPRF client
// 4. Send blinded input to PRS for evaluation
// 5. PRS returns the final pseudonymized identifier (deblinding happens at the consuming system/NVI)
func (c Component) IdentifierToToken(ctx context.Context, identifier fhir.Identifier, localOrganizationURA string, recipientURA string, scope string) (*fhir.Identifier, error) {
	if c.config.PRSBaseURL == "" {
		// TODO: Remove Fake Pseudonymizer fallback once PRS is properly integrated
		slog.WarnContext(ctx, "PRS base URL is not configured, using fake pseudonymizer for IdentifierToToken")
		return FakePseudonymizer{}.IdentifierToToken(ctx, identifier, localOrganizationURA, recipientURA, scope)
	}

	if identifier.System == nil || *identifier.System != coding.BSNNamingSystem || identifier.Value == nil {
		return &identifier, nil
	}

	// Step 1: Create PRS identifier
	prsID := prsIdentifier{
		LandCode: "NL",
		Type:     "BSN",
		Value:    *identifier.Value,
	}

	// Step 2 & 3: Blind the identifier (internally derives key and blinds)
	blindedInputData, err := blindIdentifier(prsID, recipientURA, scope)
	if err != nil {
		return nil, fmt.Errorf("blinding identifier: %w", err)
	}

	// Step 4: Call PRS to get the pseudonymized identifier
	// PRS returns the final pseudonymized BSN (deblinding happens at the consuming system/NVI)
	pseudonymizedBSN, err := c.callPRSEvaluate(ctx, localOrganizationURA, recipientURA, scope, blindedInputData)
	if err != nil {
		return nil, err
	}

	return &fhir.Identifier{
		System: to.Ptr(coding.BSNTransportTokenNamingSystem),
		Value:  &pseudonymizedBSN,
	}, nil
}

type prsIdentifier struct {
	LandCode string `json:"landCode"`
	Type     string `json:"type"`
	Value    string `json:"value"`
}

type prsEvaluateRequest struct {
	RecipientOrganization string `json:"recipientOrganization"`
	RecipientScope        string `json:"recipientScope"`
	EncryptedPersonalID   []byte `json:"encryptedPersonalId"`
}

type prsEvaluateResponse struct {
	JWE string `json:"jwe"`
}

// callPRSEvaluate sends the blinded input to the PRS service and returns the pseudonymized identifier
func (c Component) callPRSEvaluate(ctx context.Context, localOrganizationURA string, recipientURA string, scope string, blindedInputData []byte) (string, error) {
	httpClient, err := c.httpClientFn(ctx, []string{"prs:read"}, localOrganizationURA, c.config.PRSBaseURL)
	if err != nil {
		return "", fmt.Errorf("creating PRS HTTP client: %w", err)
	}

	requestBody := prsEvaluateRequest{
		RecipientOrganization: "ura:" + recipientURA,
		RecipientScope:        scope,
		EncryptedPersonalID:   blindedInputData,
	}

	bodyBytes, err := json.Marshal(requestBody)
	if err != nil {
		return "", err
	}

	requestURL, err := url.Parse(c.config.PRSBaseURL)
	if err != nil {
		return "", err
	}
	requestURL.Path = path.Join(requestURL.Path, "oprf/eval")
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, requestURL.String(), bytes.NewReader(bodyBytes))
	if err != nil {
		return "", err
	}
	httpRequest.Header.Set("Content-Type", "application/json")
	httpResponse, err := httpClient.Do(httpRequest)
	if err != nil {
		return "", fmt.Errorf("PRS request: %w", err)
	}
	defer httpResponse.Body.Close()

	response, err := from.JSONResponse[prsEvaluateResponse](httpResponse)
	if err != nil {
		return "", fmt.Errorf("PRS response: %w", err)
	}
	return response.JWE, nil
}
